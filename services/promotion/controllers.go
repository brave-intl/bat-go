package promotion

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/libs/clients"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/inputs"
	"github.com/brave-intl/bat-go/libs/jsonutils"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/middleware"
	"github.com/brave-intl/bat-go/libs/requestutils"
	"github.com/brave-intl/bat-go/libs/responses"
	"github.com/brave-intl/bat-go/libs/useragent"
	"github.com/brave-intl/bat-go/libs/validators"
	"github.com/go-chi/chi"
	"github.com/prometheus/client_golang/prometheus"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// RouterV2 for promotion endpoints
func RouterV2(service *Service, vbatExpires time.Time) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.NewUpgradeRequiredByMiddleware(vbatExpires))
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
func Router(service *Service, vbatExpires time.Time) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.NewUpgradeRequiredByMiddleware(vbatExpires))
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
	r.Method("GET", "/drain/{drainId}", middleware.InstrumentHandler("GetDrainPoll", GetDrainPoll(service)))
	r.Method("POST", "/report-bap", middleware.HTTPSignedOnly(service)(middleware.InstrumentHandler("PostReportBAPEvent", PostReportBAPEvent(service))))
	r.Method("GET", "/custodian-drain-status/{paymentId}", middleware.SimpleTokenAuthorizedOnly(middleware.InstrumentHandler("GetCustodianDrainInfo", GetCustodianDrainInfo(service))))
	r.Method("PATCH", "/drain-jobs/wallets/{walletId}/erred", middleware.SimpleTokenAuthorizedOnly(middleware.InstrumentHandler("PatchDrainJobErred", PatchDrainJobErred(service))))
	return r
}

// SuggestionsV2Router for suggestions endpoints
func SuggestionsV2Router(service *Service, vbatExpires time.Time) (chi.Router, error) {
	r := chi.NewRouter()
	r.Use(middleware.NewUpgradeRequiredByMiddleware(vbatExpires))
	var (
		enableLinkingDraining bool
		err                   error
	)
	// make sure that we only enable the DrainJob if we have linking/draining enabled
	if os.Getenv("ENABLE_LINKING_DRAINING") != "" {
		enableLinkingDraining, err = strconv.ParseBool(os.Getenv("ENABLE_LINKING_DRAINING"))
		if err != nil {
			return nil, fmt.Errorf("invalid enable_linking_draining flag: %w", err)
		}
	}

	if enableLinkingDraining {
		r.Method("POST", "/claim", middleware.HTTPSignedOnly(service)(middleware.InstrumentHandler("DrainSuggestionV2", DrainSuggestionV2(service))))
	}
	return r, nil
}

// SuggestionsRouter for suggestions endpoints
func SuggestionsRouter(service *Service, vbatExpires time.Time) (chi.Router, error) {
	r := chi.NewRouter()
	r.Use(middleware.NewUpgradeRequiredByMiddleware(vbatExpires))
	r.Method("POST", "/", middleware.InstrumentHandler("MakeSuggestion", MakeSuggestion(service)))

	var (
		enableLinkingDraining bool
		err                   error
	)
	// make sure that we only enable the DrainJob if we have linking/draining enabled
	if os.Getenv("ENABLE_LINKING_DRAINING") != "" {
		enableLinkingDraining, err = strconv.ParseBool(os.Getenv("ENABLE_LINKING_DRAINING"))
		if err != nil {
			return nil, fmt.Errorf("invalid enable_linking_draining flag: %w", err)
		}
	}

	if enableLinkingDraining {
		r.Method("POST", "/claim", middleware.HTTPSignedOnly(service)(middleware.InstrumentHandler("DrainSuggestion", DrainSuggestion(service))))
	}
	return r, nil
}

// WalletEventRouter for reporting bat loss events
func WalletEventRouter(service *Service, vbatExpires time.Time) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.NewUpgradeRequiredByMiddleware(vbatExpires))
	r.Method("POST", "/{walletId}/events/batloss/{reportId}", middleware.HTTPSignedOnly(service)(middleware.InstrumentHandler("PostReportWalletEvent", PostReportWalletEvent(service))))
	return r
}

// LookupVerifier based on the HTTP signing keyID, which in our case is the walletID
func (service *Service) LookupVerifier(ctx context.Context, keyID string) (context.Context, *httpsignature.Verifier, error) {
	walletID, err := uuid.FromString(keyID)
	if err != nil {
		return nil, nil, errorutils.Wrap(err, "KeyID format is invalid")
	}

	wallet, err := service.wallet.GetWallet(ctx, walletID)
	if err != nil {
		return nil, nil, errorutils.Wrap(err, "error getting wallet")
	}

	if wallet == nil {
		return nil, nil, nil
	}

	var publicKey httpsignature.Ed25519PubKey
	if len(wallet.PublicKey) > 0 {
		var err error
		publicKey, err = hex.DecodeString(wallet.PublicKey)
		if err != nil {
			return nil, nil, err
		}
	}
	tmp := httpsignature.Verifier(publicKey)
	return ctx, &tmp, nil
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

			logging.AddWalletIDToContext(r.Context(), *walletID.UUID())
			filter = "walletID"
		}

		platform := r.URL.Query().Get("platform")
		if len(platform) > 0 && !validators.IsPlatform(platform) {
			return handlers.ValidationError("request query parameter", map[string]string{
				"platform": fmt.Sprintf("platform '%s' is not supported", platform),
			})
		}

		migrate := false
		migrateParam := r.URL.Query().Get("migrate")
		if migrateParam == "true" {
			migrate = true
		}

		promotions, err := service.GetAvailablePromotions(r.Context(), walletID.UUID(), platform, migrate)
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
		err := requestutils.ReadJSON(r.Context(), r.Body, &req)
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

		claimID, err := service.ClaimPromotionForWallet(r.Context(), *promotionID.UUID(), req.WalletID, req.BlindedCreds)

		if err != nil {
			var (
				target *errorutils.ErrorBundle
				status = http.StatusBadRequest
			)

			if errors.Is(err, errClaimedDifferentBlindCreds) {
				status = http.StatusConflict
			}

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

// DrainPollResponse - structure for a drain poll response
type DrainPollResponse struct {
	ID     *uuid.UUID `json:"drainId"`
	Status string     `json:"status"`
}

// GetDrainPoll is the handler for checking on a particular claim's status
func GetDrainPoll(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var drainID = new(inputs.ID)
		if err := inputs.DecodeAndValidateString(context.Background(), drainID, chi.URLParam(r, "drainId")); err != nil {
			return handlers.ValidationError(
				"Error validating request url parameter",
				map[string]interface{}{
					"drainId": err.Error(),
				},
			)
		}

		var resp = &DrainPollResponse{}

		drainPoll, err := service.Datastore.GetDrainPoll(drainID.UUID())
		if err != nil {
			return handlers.WrapError(err, "Error getting drain poll by id", http.StatusBadRequest)
		}

		if drainPoll == nil {
			return &handlers.AppError{
				Message: "Drain Job does not exist",
				Code:    http.StatusNotFound,
				Data:    map[string]interface{}{},
			}
		}

		resp.ID = drainPoll.ID
		resp.Status = drainPoll.Status

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
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

		claim, err := service.Datastore.GetClaimCreds(*claimID.UUID())
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

		wallet, err := service.wallet.ReadableDatastore().GetWallet(r.Context(), walletID)
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
		err := requestutils.ReadJSON(r.Context(), r.Body, &req)
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

// DrainSuggestionV2Request includes the ID of the verified wallet attempting to drain suggestions
// and returns the drain_poll uuid so the client can poll for status updates on this draining
type DrainSuggestionV2Request struct {
	WalletID    uuid.UUID           `json:"paymentId" valid:"-"`
	Credentials []CredentialBinding `json:"credentials"`
}

// DrainSuggestionV2Response - the response structure of the token draining endpoint v2
type DrainSuggestionV2Response struct {
	DrainID *uuid.UUID `json:"drainId"`
}

// DrainSuggestionV2 is the handler for draining ad suggestions for a verified wallet
func DrainSuggestionV2(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var (
			req  DrainSuggestionV2Request
			resp = DrainSuggestionV2Response{}
		)

		ctx := r.Context()
		// no logger, setup
		// get logger from context
		logger := logging.Logger(ctx, "wallet.DrainSuggestionV2")

		err := requestutils.ReadJSON(r.Context(), r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		sublogger := logger.With().
			Str("wallet_id", req.WalletID.String()).
			Logger()

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			sublogger.Error().Err(err).Msg("failed to validate request")
			return handlers.WrapValidationError(err)
		}

		logging.AddWalletIDToContext(r.Context(), req.WalletID)

		keyID, err := middleware.GetKeyID(r.Context())
		sublogger = sublogger.With().Str("key_id", keyID).Logger()
		if err != nil {
			sublogger.Error().Err(err).Msg("failed to get http signature key id")
			return handlers.WrapError(err, "Error looking up http signature info", http.StatusBadRequest)
		}
		if req.WalletID.String() != keyID {
			sublogger.Error().Err(err).Msg("httpsignature key id != wallet id")
			return handlers.ValidationError("request",
				map[string]string{"paymentId": "paymentId must match signature"})
		}

		drainID, err := service.Drain(ctx, req.Credentials, req.WalletID)
		if err != nil {
			switch err.(type) {
			case govalidator.Error:
				sublogger.Error().Err(err).Msg("validation error")
				return handlers.WrapValidationError(err)
			case govalidator.Errors:
				sublogger.Error().Err(err).Msg("validation error")
				return handlers.WrapValidationError(err)
			default:
				// FIXME not all remaining errors should be mapped to 400
				sublogger.Error().Err(err).Msg("error draining")
				return handlers.WrapError(err, "Error draining", http.StatusBadRequest)
			}
		}
		resp.DrainID = drainID

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			panic(err)
		}
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

		ctx := r.Context()
		// no logger, setup
		// get logger from context
		logger := logging.Logger(ctx, "wallet.DrainSuggestion")

		err := requestutils.ReadJSON(r.Context(), r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		sublogger := logger.With().Str("wallet_id", req.WalletID.String()).Logger()

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			sublogger.Error().Err(err).Msg("validating request body")
			return handlers.WrapValidationError(err)
		}

		logging.AddWalletIDToContext(r.Context(), req.WalletID)

		keyID, err := middleware.GetKeyID(r.Context())
		if err != nil {
			sublogger.Error().Err(err).Msg("error getting keyid from http signature")
			return handlers.WrapError(err, "Error looking up http signature info", http.StatusBadRequest)
		}
		if req.WalletID.String() != keyID {
			sublogger.Error().Err(err).Msg("keyid doesnt match wallet in url")
			return handlers.ValidationError("request",
				map[string]string{"paymentId": "paymentId must match signature"})
		}

		_, err = service.Drain(r.Context(), req.Credentials, req.WalletID)
		if err != nil {
			sublogger.Error().Err(err).Msg("failed to drain")
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
		err := requestutils.ReadJSON(r.Context(), r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		promotion, err := service.Datastore.CreatePromotion(req.Type, req.NumGrants, req.Value, req.Platform)
		if err != nil {
			return handlers.WrapError(err, "Error creating promotion", http.StatusBadRequest)
		}

		if req.Active {
			err = service.Datastore.ActivatePromotion(promotion)
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

// Validate - implement validatable
func (ccr *ClobberedClaimsRequest) Validate(ctx context.Context) error {
	// govalidator "required" does not always work on arrays, just make sure there
	// are more than 0 items
	if ccr.ClaimIDs == nil || len(ccr.ClaimIDs) < 1 {
		return errors.New("request should have more than zero items")
	}
	return nil
}

// PostReportClobberedClaims is the handler for reporting claims that were clobbered by client bug
func PostReportClobberedClaims(service *Service, version int) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var req ClobberedClaimsRequest
		err := requestutils.ReadJSON(r.Context(), r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}
		err = req.Validate(r.Context())
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		if len(req.ClaimIDs) == 0 {
			return handlers.WrapValidationError(errors.New("ClaimIDs: required, cannot be empty"))
		}

		// govalidator does not always catch empty array on required
		if len(req.ClaimIDs) == 0 {
			return handlers.WrapValidationError(errors.New("ClaimIDs: required, cannot be empty"))
		}

		err = service.Datastore.InsertClobberedClaims(r.Context(), req.ClaimIDs, version)
		if err != nil {
			return handlers.WrapError(err, "Error making control issuer", http.StatusInternalServerError)
		}

		w.WriteHeader(http.StatusOK)
		return nil
	})
}

// BatLossPayload holds the data needed to report that bat has been lost by client bug
type BatLossPayload struct {
	Amount decimal.Decimal `json:"amount" valid:"required"`
}

// PostReportWalletEvent is the handler for reporting bat was lost by client bug
func PostReportWalletEvent(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var req BatLossPayload
		err := requestutils.ReadJSON(r.Context(), r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		walletID, err := uuid.FromString(chi.URLParam(r, "walletId"))
		if err != nil {
			return handlers.ValidationError("query parameter", map[string]string{
				"paymentId": "must be a uuidv4",
			})
		}
		reportIDParam := chi.URLParam(r, "reportId")
		reportID, err := strconv.Atoi(reportIDParam)
		if err != nil {
			return handlers.ValidationError("report id is not an int", map[string]string{
				"reportId": "report id (" + reportIDParam + ") must be an integer",
			})
		}
		platform := useragent.ParsePlatform(r.UserAgent())

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		created, err := service.Datastore.InsertBATLossEvent(
			r.Context(),
			walletID,
			reportID,
			req.Amount,
			platform,
		)
		if err != nil {
			if errors.Is(err, errorutils.ErrConflictBATLossEvent) {
				return handlers.WrapError(err, "Error inserting bat loss event", http.StatusConflict)
			}
			return handlers.WrapError(err, "Error inserting bat loss event", http.StatusInternalServerError)
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		return handlers.RenderContent(r.Context(), nil, w, status)
	})
}

// BapReportPayload holds the data needed to report that bat has been lost by client bug
type BapReportPayload struct {
	Amount decimal.Decimal `json:"amount" valid:"required"`
}

// BapReportResp holds the data needed to report that bat has been lost by client bug
type BapReportResp struct {
	ReportBapID *uuid.UUID `json:"reportBapId" valid:"required"`
}

// PostReportBAPEvent is the handler for reporting bat was lost by client bug
func PostReportBAPEvent(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var req BapReportPayload
		err := requestutils.ReadJSON(r.Context(), r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		// get wallet id from http signature id
		id, err := middleware.GetKeyID(r.Context())
		if err != nil {
			return handlers.ValidationError("no id in http signature", map[string]string{
				"id": "missing",
			})
		}

		walletID, err := uuid.FromString(id)
		if err != nil {
			return handlers.ValidationError("query parameter", map[string]string{
				"paymentId": "must be a uuidv4",
			})
		}

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		// do the magic here
		bapReportID, err := service.Datastore.InsertBAPReportEvent(
			r.Context(),
			walletID,
			req.Amount,
		)

		if err != nil {
			if errors.Is(err, errorutils.ErrConflictBAPReportEvent) {
				return handlers.WrapError(err, "Error inserting bap report, paymentId already reported", http.StatusConflict)
			}
			return handlers.WrapError(err, "Error inserting bap report", http.StatusInternalServerError)
		}
		return handlers.RenderContent(r.Context(), BapReportResp{ReportBapID: bapReportID}, w, http.StatusOK)
	})
}

// CustodianDrainInfoResponse - the response to a custodian drain info request
type CustodianDrainInfoResponse struct {
	responses.Meta
	Drains []CustodianDrain `json:"drains,omitempty"`
}

// GetCustodianDrainInfo is the handler which provides information about a particular paymentId's drains
func GetCustodianDrainInfo(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {

		var paymentID = new(inputs.ID)
		if err := inputs.DecodeAndValidateString(context.Background(), paymentID, chi.URLParam(r, "paymentId")); err != nil {
			return handlers.ValidationError(
				"Error validating request url parameter",
				map[string]interface{}{
					"paymentId": err.Error(),
				},
			)
		}

		var resp = &CustodianDrainInfoResponse{}

		drainInfo, err := service.Datastore.GetCustodianDrainInfo(paymentID.UUID())
		if err != nil {
			return handlers.WrapError(err, "Error getting custodian drain info payment id", http.StatusBadRequest)
		}

		if drainInfo == nil {
			return &handlers.AppError{
				Message: "Drain Info does not exist",
				Code:    http.StatusNotFound,
				Data:    map[string]interface{}{},
			}
		}

		resp.Drains = drainInfo
		resp.Meta.Status = "success"

		return handlers.RenderContent(r.Context(), resp, w, http.StatusOK)
	})
}

// DrainJobRequest holds data for drain job requests
type DrainJobRequest struct {
	Erred bool `json:"erred"`
}

// PatchDrainJobErred is the handler for toggling a drain job as retriable
func PatchDrainJobErred(service *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {

		walletID, err := uuid.FromString(chi.URLParam(r, "walletId"))
		if err != nil {
			return handlers.ValidationError("validation error", map[string]string{
				"walletId": "must be a valid uuid v4",
			})
		}

		var drainJobRequest DrainJobRequest
		err = requestutils.ReadJSON(r.Context(), r.Body, &drainJobRequest)
		if err != nil {
			return handlers.WrapError(errors.New("could not decode request body"), "patch drain job",
				http.StatusBadRequest)
		}

		if drainJobRequest.Erred {
			return handlers.ValidationError("validation error", map[string]string{
				"erred": "invalid value true only false is supported",
			})
		}

		err = service.Datastore.UpdateDrainJobAsRetriable(r.Context(), walletID)
		if err != nil {
			logging.FromContext(r.Context()).Err(err).Msg("patch drain job")
			switch {
			case errors.Is(err, errorutils.ErrNotFound):
				return handlers.WrapError(fmt.Errorf("no updateable drain job found for walletId %s", walletID),
					"patch drain job", http.StatusNotFound)
			default:
				return handlers.WrapError(fmt.Errorf("error updating drain job for walletdId %s", walletID),
					"patch drain job", http.StatusInternalServerError)
			}
		}

		w.WriteHeader(http.StatusNoContent)
		return nil
	}
}
