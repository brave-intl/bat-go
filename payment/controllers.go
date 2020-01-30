package payment

import (
	"encoding/json"
	"net/http"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/requestutils"
	"github.com/go-chi/chi"
	uuid "github.com/satori/go.uuid"
)

// Router for order endpoints
func Router(service *Service) chi.Router {
	r := chi.NewRouter()
	r.Method("POST", "/", middleware.InstrumentHandler("CreateOrder", CreateOrder(service)))
	r.Method("GET", "/{id}", middleware.InstrumentHandler("GetOrder", GetOrder(service)))

	r.Method("POST", "/{orderID}/transactions", middleware.InstrumentHandler("CreateTransaction", CreateTransaction(service)))

	return r
}

// OrderItemRequest is the body for creating new items
type OrderItemRequest struct {
	SKU      string `json:"sku" valid:"-"`
	Quantity int    `json:"quantity" valid:"int"`
}

// CreateOrderRequest includes information needed to create an order
type CreateOrderRequest struct {
	Items []OrderItemRequest `json:"items" valid:"-"`
}

// CreateOrder is the handler for creating a new order
func CreateOrder(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var req CreateOrderRequest
		err := requestutils.ReadJSON(r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		order, err := service.CreateOrderFromRequest(req)

		if err != nil {
			return handlers.WrapError(err, "Error creating the order in the database", http.StatusInternalServerError)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(order); err != nil {
			return handlers.WrapError(err, "Error encoding the orders JSON", http.StatusInternalServerError)
		}
		return nil
	})
}

// GetOrder is the handler for creating a new order
func GetOrder(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		orderID := chi.URLParam(r, "id")
		if orderID == "" || !govalidator.IsUUIDv4(orderID) {
			return handlers.ValidationError(
				"Error validating request url parameter",
				map[string]interface{}{
					"validationErrors": map[string]string{
						"orderID": "orderID must be a uuidv4",
					},
				},
			)
		}

		id, err := uuid.FromString(orderID)
		uuid.Must(id, err)

		order, err := service.datastore.GetOrder(id)
		if err != nil {
			return handlers.WrapError(err, "Error retrieving the order", http.StatusInternalServerError)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(order); err != nil {
			return handlers.WrapError(err, "Error encoding the orders JSON", http.StatusInternalServerError)
		}
		return nil
	})
}

// CreateTransactionRequest includes information needed to create a transaction
type CreateTransactionRequest struct {
	ExternalTransactionID string `json:"externalTransactionID"`
}

// CreateTransaction creates a transaction against an order
func CreateTransaction(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var req CreateTransactionRequest
		err := requestutils.ReadJSON(r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		orderID := chi.URLParam(r, "orderID")
		if orderID == "" || !govalidator.IsUUIDv4(orderID) {
			return handlers.ValidationError(
				"Error validating request url parameter",
				map[string]interface{}{
					"validationErrors": map[string]string{
						"orderID": "orderID must be a uuidv4",
					},
				},
			)
		}
		validOrderID, err := uuid.FromString(orderID)
		uuid.Must(validOrderID, err)

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		transaction, err := service.CreateTransactionFromRequest(req, validOrderID)
		if err != nil {
			return handlers.WrapError(err, "Error creating the transaction", http.StatusInternalServerError)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(transaction); err != nil {
			return handlers.WrapError(err, "Error encoding the transaction JSON", http.StatusInternalServerError)
		}
		return nil
	})
}

// FIXME should this be consollidated with the above?

// CreateAnonCardTransactionRequest includes information needed to create a anon card transaction
type CreateAnonCardTransactionRequest struct {
	WalletID    uuid.UUID `json:"paymentId"`
	Transaction string    `json:"transaction"`
	Kind        string    `json:"kind"`
}

// CreateAnonCardTransaction creates a transaction against an order
func CreateAnonCardTransaction(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var req CreateAnonCardTransactionRequest
		err := requestutils.ReadJSON(r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		orderID := chi.URLParam(r, "id")
		if orderID == "" || !govalidator.IsUUIDv4(orderID) {
			return &handlers.AppError{
				Message: "Error validating request url parameter",
				Code:    http.StatusBadRequest,
				Data: map[string]interface{}{
					"validationErrors": map[string]string{
						"orderID": "orderID must be a uuidv4",
					},
				},
			}
		}
		validOrderID, err := uuid.FromString(orderID)
		uuid.Must(validOrderID, err)

		txInfo, err := service.wallet.SubmitAnonCardTransaction(r.Context(), req.WalletID, req.Transaction)
		if err != nil {
			return handlers.WrapError(err, "Error submitting anon card transaction", http.StatusBadRequest)
		}

		transaction, err := service.datastore.CreateTransaction(validOrderID, txInfo.ID, txInfo.Status, txInfo.DestCurrency, "anonymous-card", txInfo.DestAmount)
		if err != nil {
			return handlers.WrapError(err, "Error recording anon card transaction", http.StatusInternalServerError)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(transaction); err != nil {
			panic(err)
		}
		return nil

	})
}

// CreateOrderCredsRequest includes the item ID and blinded credentials which to be signed
type CreateOrderCredsRequest struct {
	ItemID       uuid.UUID `json:"itemId" valid:"-"`
	BlindedCreds []string  `json:"blindedCreds" valid:"base64"`
}

// CreateOrderCreds is the handler for creating order credentials
func CreateOrderCreds(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var req CreateOrderCredsRequest
		err := requestutils.ReadJSON(r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		orderID := chi.URLParam(r, "id")
		if orderID == "" || !govalidator.IsUUIDv4(orderID) {
			return &handlers.AppError{
				Message: "Error validating request url parameter",
				Code:    http.StatusBadRequest,
				Data: map[string]interface{}{
					"validationErrors": map[string]string{
						"orderID": "orderID must be a uuidv4",
					},
				},
			}
		}
		validOrderID, err := uuid.FromString(orderID)
		uuid.Must(validOrderID, err)

		err = service.CreateOrderCreds(r.Context(), validOrderID, req.ItemID, req.BlindedCreds)
		if err != nil {
			return handlers.WrapError(err, "Error creating order creds", http.StatusBadRequest)
		}

		return nil
	})
}

// GetOrderCreds is the handler for fetching order credentials
func GetOrderCreds(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		orderID := chi.URLParam(r, "orderID")
		if orderID == "" || !govalidator.IsUUIDv4(orderID) {
			return &handlers.AppError{
				Message: "Error validating request url parameter",
				Code:    http.StatusBadRequest,
				Data: map[string]interface{}{
					"validationErrors": map[string]string{
						"orderID": "orderID must be a uuidv4",
					},
				},
			}
		}

		id, err := uuid.FromString(orderID)
		uuid.Must(id, err)

		creds, err := service.datastore.GetOrderCreds(id)
		if err != nil {
			return handlers.WrapError(err, "Error getting claim", http.StatusBadRequest)
		}

		if creds == nil {
			return &handlers.AppError{
				Message: "Order does not exist",
				Code:    http.StatusNotFound,
				Data:    map[string]interface{}{},
			}
		}

		w.WriteHeader(http.StatusAccepted)
		if err := json.NewEncoder(w).Encode(creds); err != nil {
			panic(err)
		}
		return nil
	})
}
