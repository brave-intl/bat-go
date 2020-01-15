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
	SKU     string `json:"sku"`
	Quanity int    `json:"quanity"`
}

// CreateOrderRequest includes information needed to create an order
type CreateOrderRequest struct {
	Items []OrderItemRequest `json:"items"`
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
		if err != nil {
			panic(err) // Should not be possible
		}

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
		if err != nil {
			panic(err) // Should not be possible
		}

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
