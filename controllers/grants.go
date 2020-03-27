package controllers

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/grant"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/brave-intl/bat-go/utils/requestutils"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/go-chi/chi"
	chiware "github.com/go-chi/chi/middleware"
	uuid "github.com/satori/go.uuid"
)

// GrantsRouter is the router for grant endpoints
func GrantsRouter(service *grant.Service) chi.Router {
	r := chi.NewRouter()
	if os.Getenv("ENV") != "local" {
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
	r.Method("GET", "/active", middleware.InstrumentHandler("GetActive", GetActive(service)))
	r.Method("POST", "/drain", middleware.InstrumentHandler("DrainGrants", DrainGrants(service)))
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

// ActiveGrantsResponse includes information about currently active grants for a wallet
type ActiveGrantsResponse struct {
	Grants []grant.Grant `json:"grants"`
}

// GetActive is the handler for returning info about active grants
func GetActive(service *grant.Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		walletID := r.URL.Query().Get("paymentId")

		if len(walletID) == 0 || !govalidator.IsUUIDv4(walletID) {
			return handlers.ValidationError("Error validating request query parameter",
				map[string]string{
					"paymentId": "paymentId must be a uuidv4",
				})
		}
		logging.AddWalletIDToContext(r.Context(), uuid.Must(uuid.FromString(walletID)))

		var wallet wallet.Info
		wallet.ID = walletID
		grants, err := service.GetGrantsOrderedByExpiry(wallet, "")
		if err != nil {
			return handlers.WrapError(err, "Error looking up active grants", http.StatusBadRequest)
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(&ActiveGrantsResponse{Grants: grants}); err != nil {
			panic(err)
		}
		return nil
	})
}

// RedeemGrants is the handler for redeeming one or more grants
func RedeemGrants(service *grant.Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var req grant.RedeemGrantsRequest
		err := requestutils.ReadJSON(r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		logging.AddWalletIDToContext(r.Context(), uuid.Must(uuid.FromString(req.WalletInfo.ID)))

		redeemInfo, err := service.Redeem(r.Context(), &req)
		if err != nil {
			// FIXME not all errors are 4xx
			return handlers.WrapError(err, "Error redeeming grant", http.StatusBadRequest)
		}

		if redeemInfo == nil {
			w.WriteHeader(http.StatusNoContent)
			return nil
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(redeemInfo); err != nil {
			panic(err)
		}
		return nil
	})
}

// DrainGrants is the handler for draining all grants in a wallet into a linked uphold account
func DrainGrants(service *grant.Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var req grant.DrainGrantsRequest
		err := requestutils.ReadJSON(r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		drainInfo, err := service.Drain(r.Context(), &req)
		if err != nil {
			// FIXME not all errors are 4xx
			return handlers.WrapError(err, "Error redeeming grant", http.StatusBadRequest)
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(drainInfo); err != nil {
			panic(err)
		}
		return nil
	})
}
