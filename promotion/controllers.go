package promotion

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/clients"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/brave-intl/bat-go/utils/jsonutils"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/brave-intl/bat-go/utils/requestutils"
	"github.com/brave-intl/bat-go/utils/validators"
	"github.com/go-chi/chi"
	"github.com/prometheus/client_golang/prometheus"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// RouterV2 for promotion endpoints
func RouterV2(service *Service) chi.Router {
	r := chi.NewRouter()
	if os.Getenv("ENV") != "local" {
		r.Method("POST", "/", middleware.SimpleTokenAuthorizedOnly(CreatePromotion(service)))
	} else {
		r.Method("POST", "/", CreatePromotion(service))
	}

	// version 2 clobbered claims
	r.Method("POST", "/reportclobberedclaims", middleware.InstrumentHandler("ReportClobberedClaims", PostReportClobberedClaims(service, 2)))

	return r
}

// Router for promotion endpoints
func Router(service *Service) chi.Router {
	r := chi.NewRouter()
	if os.Getenv("ENV") != "local" {
		r.Method("POST", "/", middleware.SimpleTokenAuthorizedOnly(CreatePromotion(service)))
	} else {
		r.Method("POST", "/", CreatePromotion(service))
	}

	r.Method("GET", "/{claimType}/grants/summary", middleware.InstrumentHandler("GetClaimSummary", GetClaimSummary(service)))
	r.Method("GET", "/", middleware.InstrumentHandler("GetAvailablePromotions", GetAvailablePromotions(service)))
	// version 1 clobbered claims
	r.Method("POST", "/reportclobberedclaims", middleware.InstrumentHandler("ReportClobberedClaims", PostReportClobberedClaims(service, 1)))
	r.Method("POST", "/{promotionId}", middleware.HTTPSignedOnly(service)(middleware.InstrumentHandler("ClaimPromotion", ClaimPromotion(service))))
	r.Method("GET", "/{promotionId}/claims/{claimId}", middleware.InstrumentHandler("GetClaim", GetClaim(service)))
	return r
}

// SuggestionsRouter for suggestions endpoints
func SuggestionsRouter(service *Service) chi.Router {
	r := chi.NewRouter()
	r.Method("POST", "/", middleware.InstrumentHandler("MakeSuggestion", MakeSuggestion(service)))
	r.Method("POST", "/claim", middleware.HTTPSignedOnly(service)(middleware.InstrumentHandler("DrainSuggestion", DrainSuggestion(service))))
	return r
}

// LookupPublicKey based on the HTTP signing keyID, which in our case is the walletID
func (service *Service) LookupPublicKey(ctx context.Context, keyID string) (*httpsignature.Verifier, error) {
	walletID, err := uuid.FromString(keyID)
	if err != nil {
		return nil, errorutils.Wrap(err, "KeyID format is invalid")
	}

	wallet, err := service.wallet.GetOrCreateWallet(ctx, walletID)
	if err != nil {
		return nil, errorutils.Wrap(err, "error getting wallet")
	}

	if wallet == nil {
		return nil, nil
	}

	var publicKey httpsignature.Ed25519PubKey
	if len(wallet.PublicKey) > 0 {
		var err error
		publicKey, err = hex.DecodeString(wallet.PublicKey)
		if err != nil {
			return nil, err
		}
	}
	tmp := httpsignature.Verifier(publicKey)
	return &tmp, nil
}

// PromotionsResponse is a list of known promotions to be consumed by the browser
type PromotionsResponse struct {
	Promotions []Promotion `json:"promotions"`
}

// GetAvailablePromotions is the handler for getting available promotions
func GetAvailablePromotions(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var (
			filter   string
			walletID = new(inputs.ID)
		)
		walletIDText := r.URL.Query().Get("paymentId")

		if len(walletIDText) > 0 {
			if err := inputs.DecodeAndValidateString(context.Background(), walletID, walletIDText); err != nil {
				return handlers.ValidationError(
					"Error validating request url parameter",
					map[string]interface{}{
						"paymentId": err.Error(),
					},
				)
			}

			logging.AddWalletIDToContext(r.Context(), walletID.UUID())
			filter = "walletID"
		}

		platform := r.URL.Query().Get("platform")
		if len(platform) > 0 && !validators.IsPlatform(platform) {
			return handlers.ValidationError("request query parameter", map[string]string{
				"platform": fmt.Sprintf("platform '%s' is not supported", platform),
			})
		}

		legacy := false
		legacyParam := r.URL.Query().Get("legacy")
		if legacyParam == "true" {
			legacy = true
		}

		migrate := false
		migrateParam := r.URL.Query().Get("migrate")
		if migrateParam == "true" {
			migrate = true
		}

		tmp := walletID.UUID()
		promotions, err := service.GetAvailablePromotions(r.Context(), &tmp, platform, legacy, migrate)
		if err != nil {
			return handlers.WrapError(err, "Error getting available promotions", http.StatusInternalServerError)
		}
		if promotions == nil {
			return handlers.WrapError(err, "Error finding wallet", http.StatusNotFound)
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(&PromotionsResponse{*promotions}); err != nil {
			panic(err)
		}
		if len(filter) == 0 {
			filter = "none"
		}
		promotionGetCount.With(prometheus.Labels{
			"filter":  filter,
			"legacy":  fmt.Sprint(legacy),
			"migrate": fmt.Sprint(migrate),
		}).Inc()
		for _, promotion := range *promotions {
			promotionExposureCount.With(prometheus.Labels{
				"id": promotion.ID.String(),
			}).Inc()
		}
		return nil
	})
}

// ClaimRequest includes the ID of the wallet attempting to claim and blinded credentials which to be signed
type ClaimRequest struct {
	WalletID     uuid.UUID `json:"paymentId" valid:"-"`
	BlindedCreds []string  `json:"blindedCreds" valid:"base64"`
}

// ClaimResponse includes a ClaimID which can later be used to check the status of the claim
type ClaimResponse struct {
	ClaimID uuid.UUID `json:"claimId"`
}

// ClaimPromotion is the handler for claiming a particular promotion by a wallet
func ClaimPromotion(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var req ClaimRequest
		err := requestutils.ReadJSON(r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		logging.AddWalletIDToContext(r.Context(), req.WalletID)

		keyID, err := middleware.GetKeyID(r.Context())
		if err != nil {
			return handlers.WrapError(err, "Error looking up http signature info", http.StatusBadRequest)
		}
		if req.WalletID.String() != keyID {
			return handlers.ValidationError("Error validating request", map[string]string{
				"paymentId": "paymentId must match signature",
			})
		}

		var promotionID = new(inputs.ID)
		if err := inputs.DecodeAndValidateString(context.Background(), promotionID, chi.URLParam(r, "promotionId")); err != nil {
			return handlers.ValidationError(
				"Error validating request url parameter",
				map[string]interface{}{
					"promotionId": err.Error(),
				},
			)
		}

		claimID, err := service.ClaimPromotionForWallet(r.Context(), promotionID.UUID(), req.WalletID, req.BlindedCreds)

		if err != nil {
			var target *errorutils.ErrorBundle
			status := http.StatusBadRequest
			if errors.As(err, &target) {
				err = target
				response, ok := target.Data().(clients.HTTPState)
				if ok {
					if response.Status != 0 {
						status = response.Status
					}
					err = fmt.Errorf(target.Error())
				}
			}
			return handlers.WrapError(err, "Error claiming promotion", status)
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(&ClaimResponse{*claimID}); err != nil {
			panic(err)
		}
		return nil
	})
}

// GetClaimResponse includes signed credentials and a batch proof showing they were signed by the public key
type GetClaimResponse struct {
	SignedCreds jsonutils.JSONStringArray `json:"signedCreds"`
	BatchProof  string                    `json:"batchProof"`
	PublicKey   string                    `json:"publicKey"`
}

// GetClaim is the handler for checking on a particular claim's status
func GetClaim(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var claimID = new(inputs.ID)
		if err := inputs.DecodeAndValidateString(context.Background(), claimID, chi.URLParam(r, "claimId")); err != nil {
			return handlers.ValidationError(
				"Error validating request url parameter",
				map[string]interface{}{
					"claimId": err.Error(),
				},
			)
		}

		claim, err := service.datastore.GetClaimCreds(claimID.UUID())
		if err != nil {
			return handlers.WrapError(err, "Error getting claim", http.StatusBadRequest)
		}

		if claim == nil {
			return &handlers.AppError{
				Message: "Claim does not exist",
				Code:    http.StatusNotFound,
				Data:    map[string]interface{}{},
			}
		}

		if claim.SignedCreds == nil {
			return &handlers.AppError{
				Message: "Claim has been accepted but is not ready",
				Code:    http.StatusAccepted,
				Data:    map[string]interface{}{},
			}
		}

		resp := &GetClaimResponse{
			SignedCreds: *claim.SignedCreds,
			BatchProof:  *claim.BatchProof,
			PublicKey:   *claim.PublicKey,
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			panic(err)
		}
		return nil

	})
}

// GetClaimSummary returns an summary of grants claimed by a given wallet
func GetClaimSummary(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		claimType := chi.URLParam(r, "claimType")
		walletIDQuery := r.URL.Query().Get("paymentID")
		if len(walletIDQuery) == 0 {
			walletIDQuery = r.URL.Query().Get("paymentId")
		}
		walletID, err := uuid.FromString(walletIDQuery)

		if err != nil {
			return handlers.ValidationError("query parameter", map[string]string{
				"paymentId": "must be a uuidv4",
			})
		}

		logging.AddWalletIDToContext(r.Context(), walletID)

		wallet, err := service.ReadableDatastore().GetWallet(walletID)
		if err != nil {
			return handlers.WrapError(err, "Error finding wallet", http.StatusInternalServerError)
		}

		if wallet == nil {
			err := fmt.Errorf("wallet not found id: '%s'", walletID.String())
			return handlers.WrapError(err, "Error finding wallet", http.StatusNotFound)
		}

		summary, err := service.ReadableDatastore().GetClaimSummary(walletID, claimType)
		if err != nil {
			return handlers.WrapError(err, "Error aggregating wallet claims", http.StatusInternalServerError)
		}

		if summary == nil {
			w.WriteHeader(http.StatusNoContent)
			return nil
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(summary); err != nil {
			panic(err)
		}
		return nil
	})
}

// SuggestionRequest includes a suggestion payload and credentials to be redeemed
type SuggestionRequest struct {
	Suggestion  string              `json:"suggestion" valid:"base64"`
	Credentials []CredentialBinding `json:"credentials"`
}

// MakeSuggestion is the handler for making a suggestion using credentials
func MakeSuggestion(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var req SuggestionRequest
		err := requestutils.ReadJSON(r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		err = service.Suggest(r.Context(), req.Credentials, req.Suggestion)
		if err != nil {
			switch err.(type) {
			case govalidator.Error:
				return handlers.WrapValidationError(err)
			case govalidator.Errors:
				return handlers.WrapValidationError(err)
			default:
				// FIXME
				return handlers.WrapError(err, "Error making suggestion", http.StatusBadRequest)
			}
		}

		w.WriteHeader(http.StatusOK)
		return nil
	})
}

// DrainSuggestionRequest includes the ID of the verified wallet attempting to drain suggestions
type DrainSuggestionRequest struct {
	WalletID    uuid.UUID           `json:"paymentId" valid:"-"`
	Credentials []CredentialBinding `json:"credentials"`
}

// DrainSuggestion is the handler for draining ad suggestions for a verified wallet
func DrainSuggestion(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var req DrainSuggestionRequest
		err := requestutils.ReadJSON(r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		logging.AddWalletIDToContext(r.Context(), req.WalletID)

		keyID, err := middleware.GetKeyID(r.Context())
		if err != nil {
			return handlers.WrapError(err, "Error looking up http signature info", http.StatusBadRequest)
		}
		if req.WalletID.String() != keyID {
			return handlers.ValidationError("request",
				map[string]string{"paymentId": "paymentId must match signature"})
		}

		err = service.Drain(r.Context(), req.Credentials, req.WalletID)
		if err != nil {
			switch err.(type) {
			case govalidator.Error:
				return handlers.WrapValidationError(err)
			case govalidator.Errors:
				return handlers.WrapValidationError(err)
			default:
				// FIXME not all remaining errors should be mapped to 400
				return handlers.WrapError(err, "Error draining", http.StatusBadRequest)
			}
		}

		w.WriteHeader(http.StatusOK)
		return nil
	})
}

// CreatePromotionRequest includes information needed to create a promotion
type CreatePromotionRequest struct {
	Type      string          `json:"type" valid:"in(ads|ugp)"`
	NumGrants int             `json:"numGrants" valid:"required"`
	Value     decimal.Decimal `json:"value" valid:"required"`
	Platform  string          `json:"platform" valid:"platform,optional"`
	Active    bool            `json:"active" valid:"-"`
}

// CreatePromotionResponse includes information about the created promotion
type CreatePromotionResponse struct {
	Promotion
}

// CreatePromotion is the handler for creating a promotion
func CreatePromotion(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var req CreatePromotionRequest
		err := requestutils.ReadJSON(r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		promotion, err := service.datastore.CreatePromotion(req.Type, req.NumGrants, req.Value, req.Platform)
		if err != nil {
			return handlers.WrapError(err, "Error creating promotion", http.StatusBadRequest)
		}

		if req.Active {
			err = service.datastore.ActivatePromotion(promotion)
			if err != nil {
				return handlers.WrapError(err, "Error marking promotion active", http.StatusBadRequest)
			}
		}

		_, err = service.CreateIssuer(r.Context(), promotion.ID, "control")
		if err != nil {
			return handlers.WrapError(err, "Error making control issuer", http.StatusInternalServerError)
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(&CreatePromotionResponse{Promotion: *promotion}); err != nil {
			panic(err)
		}
		return nil
	})
}

// ClobberedClaimsRequest holds the data needed to report claims that were clobbered by client bug
type ClobberedClaimsRequest struct {
	ClaimIDs []uuid.UUID `json:"claimIds" valid:"required"`
}

// PostReportClobberedClaims is the handler for reporting claims that were clobbered by client bug
func PostReportClobberedClaims(service *Service, version int) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var req ClobberedClaimsRequest
		err := requestutils.ReadJSON(r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		err = service.datastore.InsertClobberedClaims(r.Context(), req.ClaimIDs, version)
		if err != nil {
			return handlers.WrapError(err, "Error making control issuer", http.StatusInternalServerError)
		}

		w.WriteHeader(http.StatusOK)
		return nil
	})
}
