package promotion

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/utils/validators"
	"github.com/go-chi/chi"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// Router for promotion endpoints
func Router(service *Service) chi.Router {
	r := chi.NewRouter()
	if os.Getenv("ENV") == "production" {
		r.Method("POST", "/", middleware.SimpleTokenAuthorizedOnly(CreatePromotion(service)))
	} else {
		r.Method("POST", "/", CreatePromotion(service))
	}

	r.Method("GET", "/{claimType}/grants/summary", middleware.InstrumentHandler("GetClaimSummary", GetClaimSummary(service)))
	r.Method("GET", "/", middleware.InstrumentHandler("GetAvailablePromotions", GetAvailablePromotions(service)))
	r.Method("POST", "/{promotionId}", middleware.HTTPSignedOnly(service)(middleware.InstrumentHandler("ClaimPromotion", ClaimPromotion(service))))
	r.Method("GET", "/{promotionId}/claims/{claimId}", middleware.InstrumentHandler("GetClaim", GetClaim(service)))
	return r
}

// SuggestionsRouter for suggestions endpoints
func SuggestionsRouter(service *Service) chi.Router {
	r := chi.NewRouter()
	r.Method("POST", "/", middleware.InstrumentHandler("MakeSuggestion", MakeSuggestion(service)))
	return r
}

// LookupPublicKey based on the HTTP signing keyID, which in our case is the walletID
func (service *Service) LookupPublicKey(ctx context.Context, keyID string) (*httpsignature.Verifier, error) {
	walletID, err := uuid.FromString(keyID)
	if err != nil {
		return nil, errors.Wrap(err, "KeyID format is invalid")
	}

	wallet, err := service.GetOrCreateWallet(ctx, walletID)
	if err != nil {
		return nil, errors.Wrap(err, "Error getting wallet")
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
		paymentID := r.URL.Query().Get("paymentId")

		if len(paymentID) == 0 || !govalidator.IsUUIDv4(paymentID) {
			return &handlers.AppError{
				Message: "Error validating request query parameter",
				Code:    http.StatusBadRequest,
				Data: map[string]interface{}{
					"validationErrors": map[string]string{
						"paymentId": "paymentId must be a uuidv4",
					},
				},
			}
		}

		platform := r.URL.Query().Get("platform")
		if !validators.IsPlatform(platform) {
			return handlers.ValidationError("request query parameter", map[string]string{
				"platform": fmt.Sprintf("platform '%s' is not supported", platform),
			})
		}

		legacy := false
		legacyParam := r.URL.Query().Get("legacy")
		if legacyParam == "true" {
			legacy = true
		}

		id, err := uuid.FromString(paymentID)
		if err != nil {
			panic(err) // Should not be possible
		}

		promotions, err := service.GetAvailablePromotions(r.Context(), id, platform, legacy)
		if err != nil {
			return handlers.WrapError(err, "Error getting available promotions", http.StatusInternalServerError)
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(&PromotionsResponse{promotions}); err != nil {
			panic(err)
		}
		return nil
	})
}

// ClaimRequest includes the ID of the wallet attempting to claim and blinded credentials which to be signed
type ClaimRequest struct {
	PaymentID    uuid.UUID `json:"paymentId"`
	BlindedCreds []string  `json:"blindedCreds" valid:"base64"`
}

// ClaimResponse includes a ClaimID which can later be used to check the status of the claim
type ClaimResponse struct {
	ClaimID uuid.UUID `json:"claim_id"`
}

// ClaimPromotion is the handler for claiming a particular promotion by a wallet
func ClaimPromotion(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		limit := int64(1024 * 1024 * 10) // 10MiB
		body, err := ioutil.ReadAll(io.LimitReader(r.Body, limit))
		if err != nil {
			return handlers.WrapError(err, "Error reading body", http.StatusInternalServerError)
		}

		var req ClaimRequest
		err = json.Unmarshal(body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error unmarshalling body", http.StatusBadRequest)
		}
		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		keyID, err := middleware.GetKeyID(r.Context())
		if err != nil {
			return handlers.WrapError(err, "Error looking up http signature info", http.StatusBadRequest)
		}
		if req.PaymentID.String() != keyID {
			return &handlers.AppError{
				Message: "Error validating request",
				Code:    http.StatusBadRequest,
				Data: map[string]interface{}{
					"validationErrors": map[string]string{
						"paymentId": "paymentId must match signature",
					},
				},
			}
		}

		promotionID := chi.URLParam(r, "promotionId")
		if promotionID == "" || !govalidator.IsUUIDv4(promotionID) {
			return &handlers.AppError{
				Message: "Error validating request url parameter",
				Code:    http.StatusBadRequest,
				Data: map[string]interface{}{
					"validationErrors": map[string]string{
						"promotionId": "promotionId must be a uuidv4",
					},
				},
			}
		}

		pID, err := uuid.FromString(promotionID)
		if err != nil {
			panic(err) // Should not be possible
		}

		claimID, err := service.ClaimPromotionForWallet(r.Context(), pID, req.PaymentID, req.BlindedCreds)
		if err != nil {
			return handlers.WrapError(err, "Error claiming promotion", http.StatusBadRequest)
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
	SignedCreds JSONStringArray `json:"signedCreds"`
	BatchProof  string          `json:"batchProof"`
	PublicKey   string          `json:"publicKey"`
}

// GetClaim is the handler for checking on a particular claim's status
func GetClaim(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		claimID := chi.URLParam(r, "claimId")
		if claimID == "" || !govalidator.IsUUIDv4(claimID) {
			return &handlers.AppError{
				Message: "Error validating request url parameter",
				Code:    http.StatusBadRequest,
				Data: map[string]interface{}{
					"validationErrors": map[string]string{
						"claimId": "claimId must be a uuidv4",
					},
				},
			}
		}

		id, err := uuid.FromString(claimID)
		if err != nil {
			panic(err) // Should not be possible
		}

		claim, err := service.datastore.GetClaimCreds(id)
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
		paymentIDQuery := r.URL.Query().Get("paymentID")
		paymentID, err := uuid.FromString(paymentIDQuery)

		if err != nil {
			return handlers.ValidationError("query parameter", map[string]string{
				"paymentID": "must be a uuidv4",
			})
		}

		summary, err := service.datastore.GetClaimSummary(paymentID, claimType)
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
		limit := int64(1024 * 1024 * 10) // 10MiB
		body, err := ioutil.ReadAll(io.LimitReader(r.Body, limit))
		if err != nil {
			// FIXME
			return handlers.WrapError(err, "Error reading body", http.StatusInternalServerError)
		}

		var req SuggestionRequest
		err = json.Unmarshal(body, &req)
		if err != nil {
			// FIXME
			return handlers.WrapError(err, "Error unmarshalling body", http.StatusBadRequest)
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
		limit := int64(1024 * 1024 * 10) // 10MiB
		body, err := ioutil.ReadAll(io.LimitReader(r.Body, limit))
		if err != nil {
			return handlers.WrapError(err, "Error reading body", http.StatusInternalServerError)
		}

		var req CreatePromotionRequest
		err = json.Unmarshal(body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error unmarshalling body", http.StatusBadRequest)
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

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(&CreatePromotionResponse{Promotion: *promotion}); err != nil {
			panic(err)
		}
		return nil

	})
}
