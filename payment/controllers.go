package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/brave-intl/bat-go/utils/requestutils"
	"github.com/go-chi/chi"
	uuid "github.com/satori/go.uuid"
)

// Router for order endpoints
func Router(service *Service) chi.Router {
	r := chi.NewRouter()
	r.Method("POST", "/", middleware.InstrumentHandler("CreateOrder", CreateOrder(service)))
	r.Method("GET", "/{orderID}", middleware.InstrumentHandler("GetOrder", GetOrder(service)))

	r.Method("GET", "/{orderID}/transactions", middleware.InstrumentHandler("GetTransactions", GetTransactions(service)))
	r.Method("POST", "/{orderID}/transactions/uphold", middleware.InstrumentHandler("CreateUpholdTransaction", CreateUpholdTransaction(service)))

	r.Method("POST", "/{orderID}/transactions/anonymousCard", middleware.InstrumentHandler("CreateAnonCardTransaction", CreateAnonCardTransaction(service)))

	r.Method("POST", "/{orderID}/credentials", middleware.InstrumentHandler("CreateOrderCreds", CreateOrderCreds(service)))
	r.Method("GET", "/{orderID}/credentials", middleware.InstrumentHandler("GetOrderCreds", GetOrderCreds(service)))
	r.Method("GET", "/{orderID}/credentials/{itemID}", middleware.InstrumentHandler("GetOrderCredsByID", GetOrderCredsByID(service)))

	return r
}

// VoteRouter for voting endpoint
func VoteRouter(service *Service) chi.Router {
	r := chi.NewRouter()
	r.Method("POST", "/", middleware.InstrumentHandler("MakeVote", MakeVote(service)))
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
		if len(req.Items) == 0 {
			return handlers.ValidationError(
				"Error validating request body",
				map[string]interface{}{
					"items": "array must contain at least one item",
				},
			)
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

// GetOrder is the handler for getting an order
func GetOrder(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		orderID := chi.URLParam(r, "orderID")
		if orderID == "" || !govalidator.IsUUIDv4(orderID) {
			return handlers.ValidationError(
				"Error validating request url parameter",
				map[string]interface{}{
					"orderID": "orderID must be a uuidv4",
				},
			)
		}

		id := uuid.Must(uuid.FromString(orderID))

		order, err := service.datastore.GetOrder(id)
		if err != nil {
			return handlers.WrapError(err, "Error retrieving the order", http.StatusInternalServerError)
		}

		w.Header().Set("Content-Type", "application/json")
		if order == nil {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		if err := json.NewEncoder(w).Encode(order); err != nil {
			return handlers.WrapError(err, "Error encoding the orders JSON", http.StatusInternalServerError)
		}
		return nil
	})
}

// GetTransactions is the handler for listing the transactions for an order
func GetTransactions(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		orderID := chi.URLParam(r, "orderID")
		if orderID == "" || !govalidator.IsUUIDv4(orderID) {
			return handlers.ValidationError(
				"Error validating request url parameter",
				map[string]interface{}{
					"orderID": "orderID must be a uuidv4",
				},
			)
		}

		id := uuid.Must(uuid.FromString(orderID))

		order, err := service.datastore.GetTransactions(id)
		if err != nil {
			return handlers.WrapError(err, "Error retrieving the transactions for the order", http.StatusInternalServerError)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(order); err != nil {
			return handlers.WrapError(err, "Error encoding the transactions JSON", http.StatusInternalServerError)
		}
		return nil
	})
}

// CreateTransactionRequest includes information needed to create a transaction
type CreateTransactionRequest struct {
	ExternalTransactionID string `json:"externalTransactionID" valid:"uuidv4"`
}

// CreateUpholdTransaction creates a transaction against an order
func CreateUpholdTransaction(service *Service) handlers.AppHandler {
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
					"orderID": "orderID must be a uuidv4",
				},
			)
		}
		validOrderID := uuid.Must(uuid.FromString(orderID))

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		// Ensure the external transaction ID hasn't already been added to any orders.
		transaction, err := service.datastore.GetTransaction(req.ExternalTransactionID)
		if err != nil {
			return handlers.WrapError(err, "externalTransactinID has already been submitted to an order", http.StatusConflict)
		}

		if transaction != nil {
			err = fmt.Errorf("External Transaction ID: %s has already been added to the order", req.ExternalTransactionID)
			return handlers.WrapError(err, "Error creating the transaction", http.StatusBadRequest)
		}

		transaction, err = service.CreateTransactionFromRequest(req, validOrderID)
		if err != nil {
			return handlers.WrapError(err, "Error creating the transaction", http.StatusBadRequest)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(transaction); err != nil {
			return handlers.WrapError(err, "Error encoding the transaction JSON", http.StatusInternalServerError)
		}
		return nil
	})
}

// CreateAnonCardTransactionRequest includes information needed to create a anon card transaction
type CreateAnonCardTransactionRequest struct {
	WalletID    uuid.UUID `json:"paymentId"`
	Transaction string    `json:"transaction"`
}

// CreateAnonCardTransaction creates a transaction against an order
func CreateAnonCardTransaction(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var req CreateAnonCardTransactionRequest
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
					"orderID": "orderID must be a uuidv4",
				},
			}
		}
		validOrderID := uuid.Must(uuid.FromString(orderID))

		transaction, err := service.CreateAnonCardTransaction(r.Context(), req.WalletID, req.Transaction, validOrderID)
		if err != nil {
			return handlers.WrapError(err, "Error creating anon card transaction", http.StatusInternalServerError)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(transaction); err != nil {
			return handlers.WrapError(err, "Error encoding the transaction JSON", http.StatusInternalServerError)
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

		orderID := chi.URLParam(r, "orderID")
		if orderID == "" || !govalidator.IsUUIDv4(orderID) {
			return &handlers.AppError{
				Message: "Error validating request url parameter",
				Code:    http.StatusBadRequest,
				Data: map[string]interface{}{
					"orderID": "orderID must be a uuidv4",
				},
			}
		}
		validOrderID := uuid.Must(uuid.FromString(orderID))

		orderCreds, err := service.datastore.GetOrderCreds(validOrderID)
		if err != nil {
			return handlers.WrapError(err, "Error validating no credentials exist for order", http.StatusBadRequest)
		}
		if orderCreds != nil {
			return handlers.WrapError(err, "There are existing order credentials created for this order", http.StatusConflict)
		}

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
			return handlers.ValidationError("Error validating request url parameter",
				map[string]interface{}{
					"orderID": "orderID must be a uuidv4",
				})
		}

		id := uuid.Must(uuid.FromString(orderID))

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

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(creds); err != nil {
			panic(err)
		}
		return nil
	})
}

// GetOrderCredsByID is the handler for fetching order credentials by an item id
func GetOrderCredsByID(service *Service) handlers.AppHandler {
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

		creds, err := service.datastore.GetOrderCredsByItemID(orderID.UUID(), itemID.UUID())
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

		return handlers.RenderContent(r.Context(), creds, w, http.StatusOK)
	})
}

// VoteRequest includes a suggestion payload and credentials to be redeemed
type VoteRequest struct {
	Vote        string              `json:"vote" valid:"base64"`
	Credentials []CredentialBinding `json:"credentials"`
}

// MakeVote is the handler for making a vote using credentials
func MakeVote(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var req VoteRequest
		err := requestutils.ReadJSON(r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		err = service.Vote(r.Context(), req.Credentials, req.Vote)
		if err != nil {
			switch err.(type) {
			case govalidator.Error:
				return handlers.WrapValidationError(err)
			case govalidator.Errors:
				return handlers.WrapValidationError(err)
			default:
				// FIXME
				return handlers.WrapError(err, "Error making vote", http.StatusBadRequest)
			}
		}

		w.WriteHeader(http.StatusOK)
		return nil
	})
}
