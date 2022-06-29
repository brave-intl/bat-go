package skus

import (
	"context"
	"errors"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/logging"
	"net/http"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/brave-intl/bat-go/utils/requestutils"
	"github.com/go-chi/chi"
	uuid "github.com/satori/go.uuid"
)

// RouterV2 for order endpoints v2
func RouterV2(service *Service, instrumentHandler middleware.InstrumentHandlerDef) chi.Router {
	r := chi.NewRouter()

	r.Route("/{orderID}/credentials", func(cr chi.Router) {

		cr.Use(corsMiddleware([]string{"GET", "POST"}))

		cr.Method("POST", "/", instrumentHandler(
			"CreateOrderCredsV2", CreateOrderCredsV2(service)))

		cr.Method("GET", "/", middleware.InstrumentHandler(
			"GetOrderCredsV2", GetOrderCredsV2(service)))

		cr.Method("GET", "/{itemID}", middleware.InstrumentHandler(
			"GetOrderCredsByIDV2", GetOrderCredsByIDV2(service)))
	})

	return r
}

// CreateOrderCredsV2Request includes the item ID and blinded credentials which to be signed
type CreateOrderCredsV2Request struct {
	ItemID       uuid.UUID `json:"itemId" valid:"-"`
	BlindedCreds []string  `json:"blindedCreds" valid:"base64"`
}

// CreateOrderCredsV2 is the handler for creating order credentials
func CreateOrderCredsV2(service *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {

		var req CreateOrderCredsV2Request
		err := requestutils.ReadJSON(r.Context(), r.Body, &req)
		if err != nil {
			logging.FromContext(r.Context()).Err(err).Msg("create order request v2")
			return handlers.WrapError(err, "error in request body", http.StatusBadRequest)
		}

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		var orderID = new(inputs.ID)
		if err := inputs.DecodeAndValidateString(context.Background(), orderID, chi.URLParam(r, "orderID")); err != nil {
			return handlers.ValidationError(
				"error validating request url parameter",
				map[string]interface{}{
					"orderID": err.Error(),
				},
			)
		}

		orderCreds, err := service.Datastore.GetOrderTimeLimitedV2CredsByItemID(*orderID.UUID(), req.ItemID, false)
		if err != nil {
			logging.FromContext(r.Context()).
				Err(err).Msg("create order request v2")

			return handlers.WrapError(err, "error retrieving order credentials",
				http.StatusInternalServerError)
		}

		if orderCreds != nil {
			return handlers.WrapError(err, "error order credentials already exist for order",
				http.StatusBadRequest)
		}

		err = service.CreateOrderCredentials(r.Context(), *orderID.UUID(), req.ItemID, req.BlindedCreds)
		if err != nil {
			logging.FromContext(r.Context()).Err(err).Msg("create order credentials v2")
			switch {
			case errors.Is(err, ErrOrderUnpaid):
				return handlers.WrapError(err, "error creating order credentials order not paid", http.StatusBadRequest)
			case errors.As(err, &errorutils.ErrNotFound):
				return handlers.WrapError(err, "error creating order credentials: order not found", http.StatusBadRequest)
			}
			return handlers.WrapError(err, "error creating order credentials", http.StatusInternalServerError)
		}

		return handlers.RenderContent(r.Context(), nil, w, http.StatusCreated)
	}
}

// GetOrderCredsV2 is the handler for fetching order credentials
func GetOrderCredsV2(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {

		var orderID = new(inputs.ID)
		if err := inputs.DecodeAndValidateString(context.Background(), orderID, chi.URLParam(r, "orderID")); err != nil {
			return handlers.ValidationError(
				"Error validating request url parameter",
				map[string]interface{}{
					"orderID": err.Error(),
				},
			)
		}

		// get credentials, either single-use/time-limited
		creds, status, err := service.GetCredentials(r.Context(), *orderID.UUID())
		if err != nil {
			return handlers.WrapError(err, "Error getting credentials", status)
		}
		return handlers.RenderContent(r.Context(), creds, w, status)
	})
}

// GetOrderCredsByIDV2 is the handler for fetching order credentials by an item id
func GetOrderCredsByIDV2(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {

		// get the IDs from the URL
		var (
			orderID           = new(inputs.ID)
			itemID            = new(inputs.ID)
			validationPayload = map[string]interface{}{}
			err               error
		)

		// decode and validate orderID url param
		if err = inputs.DecodeAndValidateString(
			context.Background(), orderID, chi.URLParam(r, "orderID")); err != nil {
			validationPayload["orderID"] = err.Error()
		}

		// decode and validate itemID url param
		if err = inputs.DecodeAndValidateString(
			context.Background(), itemID, chi.URLParam(r, "itemID")); err != nil {
			validationPayload["itemID"] = err.Error()
		}

		// did we get any validation errors?
		if len(validationPayload) > 0 {
			return handlers.ValidationError(
				"Error validating request url parameter",
				validationPayload)
		}

		creds, err := service.Datastore.GetOrderTimeLimitedV2CredsByItemID(*orderID.UUID(), *itemID.UUID(), false)
		if err != nil {
			return handlers.WrapError(err, "Error getting claim", http.StatusBadRequest)
		}

		if creds == nil {
			return &handlers.AppError{
				Message: "Could not find credentials",
				Code:    http.StatusNotFound,
				Data:    map[string]interface{}{},
			}
		}

		status := http.StatusOK
		if len(creds.Credentials) < 0 || creds.Credentials[0].SignedCreds == nil {
			status = http.StatusAccepted
		}

		return handlers.RenderContent(r.Context(), creds, w, status)
	})
}
