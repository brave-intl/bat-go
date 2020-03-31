package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"os"

	"github.com/brave-intl/bat-go/grant"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/brave-intl/bat-go/utils/wallet"
	"github.com/go-chi/chi"
)

// GrantsRouter is the router for grant endpoints
func GrantsRouter(service *grant.Service) chi.Router {
	r := chi.NewRouter()
	if os.Getenv("ENV") != "local" {
		r.Use(middleware.SimpleTokenAuthorizedOnly)
	}
	// Hacky compatibility layer between for legacy grants and new datastore
	r.Method("GET", "/active", middleware.InstrumentHandler("GetActive", GetActive(service)))
	return r
}

// ActiveGrantsResponse includes information about currently active grants for a wallet
type ActiveGrantsResponse struct {
	Grants []grant.Grant `json:"grants"`
}

// GetActive is the handler for returning info about active grants
func GetActive(service *grant.Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var wallet wallet.Info
		var walletID = new(inputs.ID)

		err := inputs.DecodeAndValidateString(context.Background(), walletID, r.URL.Query().Get("paymentId"))
		if err != nil {
			return handlers.ValidationError(
				"Error validating request url parameter",
				map[string]interface{}{
					"paymentId": err.Error(),
				},
			)
		}

		logging.AddWalletIDToContext(r.Context(), walletID.UUID())
		wallet.ID = walletID.String()

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
