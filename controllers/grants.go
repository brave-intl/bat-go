package controllers

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/grant"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/closers"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/go-chi/chi"
	chiware "github.com/go-chi/chi/middleware"
	uuid "github.com/satori/go.uuid"
)

// GrantsRouter is the router for grant endpoints
func GrantsRouter(service *grant.Service) chi.Router {
	r := chi.NewRouter()
	if os.Getenv("ENV") == "production" {
		r.Use(middleware.SimpleTokenAuthorizedOnly)
	}
	if len(os.Getenv("THROTTLE_GRANT_REQUESTS")) > 0 {
		throttle, err := strconv.ParseInt(os.Getenv("THROTTLE_GRANT_REQUESTS"), 10, http.StatusBadRequest)
		if err != nil {
			panic("THROTTLE_GRANT_REQUESTS was provided but not a valid number")
		}
		r.Method("POST", "/", chiware.Throttle(int(throttle))(middleware.InstrumentHandler("RedeemGrants", RedeemGrants(service))))
	} else {
		r.Method("POST", "/", middleware.InstrumentHandler("RedeemGrants", RedeemGrants(service)))
	}
	// Hacky compatibility layer between for legacy grants and new datastore
	r.Method("POST", "/claim", middleware.InstrumentHandler("ClaimGrant", Claim(service)))
	r.Method("PUT", "/{grantId}", middleware.InstrumentHandler("ClaimGrant", ClaimGrantWithGrantID(service)))
	r.Method("GET", "/", middleware.InstrumentHandler("Status", handlers.AppHandler(Status)))
	return r
}

// Status is the handler for checking redemption status
func Status(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	if !grant.RedemptionDisabled() {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	return nil
}

// Claim is the handler for claiming grants
func Claim(service *grant.Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		defer closers.Panic(r.Body)

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return handlers.WrapError(err, "Error reading body", http.StatusBadRequest)
		}

		var req grant.ClaimRequest
		err = json.Unmarshal(body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error unmarshalling body", http.StatusBadRequest)
		}
		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		claim, err := service.ClaimPromotion(r.Context(), req.WalletInfo, req.PromotionID)
		if err != nil {
			// FIXME not all errors are 4xx
			return handlers.WrapError(err, "Error claiming grant", http.StatusBadRequest)
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(&grant.ClaimResponse{ApproximateValue: claim.ApproximateValue}); err != nil {
			panic(err)
		}
		return nil
	})
}

// ClaimGrantWithGrantID is the handler for claiming grants using only a grant id
func ClaimGrantWithGrantID(service *grant.Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		defer closers.Panic(r.Body)

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return handlers.WrapError(err, "Error reading body", http.StatusBadRequest)
		}

		var req grant.ClaimGrantWithGrantIDRequest
		err = json.Unmarshal(body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error unmarshalling body", http.StatusBadRequest)
		}
		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		if grantID := chi.URLParam(r, "grantId"); grantID != "" {
			if !govalidator.IsUUIDv4(grantID) {
				return &handlers.AppError{
					Message: "Error validating request url parameter",
					Code:    http.StatusBadRequest,
					Data: map[string]interface{}{
						"validationErrors": map[string]string{
							"grantId": "grantId must be a uuidv4",
						},
					},
				}
			}

			var grant grant.Grant
			grant.GrantID, err = uuid.FromString(grantID)
			if err != nil {
				return handlers.WrapError(err, "Error claiming grant", http.StatusInternalServerError)
			}

			err = service.Claim(r.Context(), req.WalletInfo, grant)
			if err != nil {
				// FIXME not all errors are 4xx
				return handlers.WrapError(err, "Error claiming grant", http.StatusBadRequest)
			}
		}

		w.WriteHeader(http.StatusOK)
		return nil
	})
}

// RedeemGrants is the handler for redeeming one or more grants
func RedeemGrants(service *grant.Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		defer closers.Panic(r.Body)

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return handlers.WrapError(err, "Error reading body", http.StatusBadRequest)
		}

		var req grant.RedeemGrantsRequest
		err = json.Unmarshal(body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error unmarshalling body", http.StatusBadRequest)
		}
		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		txInfo, err := service.Redeem(r.Context(), &req)
		if err != nil {
			// FIXME not all errors are 4xx
			return handlers.WrapError(err, "Error redeeming grant", http.StatusBadRequest)
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(txInfo); err != nil {
			panic(err)
		}
		return nil
	})
}
