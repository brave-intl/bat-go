package payment

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/clients/cbr"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/cryptography"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/brave-intl/bat-go/utils/requestutils"
	"github.com/brave-intl/bat-go/utils/responses"
	"github.com/go-chi/chi"
	"github.com/go-chi/cors"
	uuid "github.com/satori/go.uuid"
	"github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/webhook"
)

func corsMiddleware(allowedMethods []string) func(next http.Handler) http.Handler {
	debug, err := strconv.ParseBool(os.Getenv("DEBUG"))
	if err != nil {
		debug = false
	}
	return cors.Handler(cors.Options{
		Debug:            debug,
		AllowedOrigins:   strings.Split(os.Getenv("ALLOWED_ORIGINS"), ","),
		AllowedMethods:   allowedMethods,
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{""},
		AllowCredentials: false,
		MaxAge:           300, // Maximum value not ignored by any of major browsers
	})
}

// Router for order endpoints
func Router(service *Service) chi.Router {
	r := chi.NewRouter()

	if os.Getenv("ENV") == "local" {
		r.Method("OPTIONS", "/", middleware.InstrumentHandler("CreateOrderOptions", corsMiddleware([]string{"POST"})(nil)))
		r.Method("POST", "/", middleware.InstrumentHandler("CreateOrder", corsMiddleware([]string{"POST"})(CreateOrder(service))))
	} else {
		r.Method("POST", "/", middleware.InstrumentHandler("CreateOrder", CreateOrder(service)))
	}

	r.Method("OPTIONS", "/{orderID}", middleware.InstrumentHandler("GetOrderOptions", corsMiddleware([]string{"GET"})(nil)))
	r.Method("GET", "/{orderID}", middleware.InstrumentHandler("GetOrder", corsMiddleware([]string{"GET"})(GetOrder(service))))
	r.Method("DELETE", "/{orderID}", middleware.InstrumentHandler("CancelOrder", corsMiddleware([]string{"DELETE"})(middleware.SimpleTokenAuthorizedOnly(CancelOrder(service)))))

	r.Method("GET", "/{orderID}/transactions", middleware.InstrumentHandler("GetTransactions", GetTransactions(service)))
	r.Method("POST", "/{orderID}/transactions/uphold", middleware.InstrumentHandler("CreateUpholdTransaction", CreateUpholdTransaction(service)))
	r.Method("POST", "/{orderID}/transactions/gemini", middleware.InstrumentHandler("CreateGeminiTransaction", CreateGeminiTransaction(service)))
	r.Method("POST", "/{orderID}/transactions/anonymousCard", middleware.InstrumentHandler("CreateAnonCardTransaction", CreateAnonCardTransaction(service)))

	r.Route("/{orderID}/credentials", func(cr chi.Router) {
		cr.Use(corsMiddleware([]string{"GET", "POST"}))
		cr.Method("POST", "/", middleware.InstrumentHandler("CreateOrderCreds", CreateOrderCreds(service)))
		cr.Method("GET", "/", middleware.InstrumentHandler("GetOrderCreds", GetOrderCreds(service)))
		// TODO authorization should be merchant specific, however currently this is only used internally
		cr.Method("DELETE", "/", middleware.InstrumentHandler("DeleteOrderCreds", middleware.SimpleTokenAuthorizedOnly(DeleteOrderCreds(service))))

		cr.Method("GET", "/{itemID}", middleware.InstrumentHandler("GetOrderCredsByID", GetOrderCredsByID(service)))
	})

	return r
}

// CredentialRouter handles calls relating to credentials
func CredentialRouter(service *Service) chi.Router {
	r := chi.NewRouter()
	r.Method("POST", "/subscription/verifications", middleware.InstrumentHandler("VerifyCredential", middleware.SimpleTokenAuthorizedOnly(VerifyCredential(service))))
	return r
}

// MerchantRouter handles calls made for the merchant
func MerchantRouter(service *Service) chi.Router {
	r := chi.NewRouter()
	if os.Getenv("ENV") != "local" {
		r.Use(middleware.SimpleTokenAuthorizedOnly)
	}

	// Once instrument handler is refactored https://github.com/brave-intl/bat-go/issues/291
	// We can use this service context instead of having
	r.Use(middleware.NewServiceCtx(service))

	// RESTy routes for "merchant" resource
	r.Route("/", func(r chi.Router) {
		r.Route("/{merchantID}", func(mr chi.Router) {
			mr.Route("/keys", func(kr chi.Router) {
				kr.Method("GET", "/", middleware.InstrumentHandler("GetKeys", GetKeys(service)))
				kr.Method("POST", "/", middleware.InstrumentHandler("CreateKey", CreateKey(service)))
				kr.Method("DELETE", "/{id}", middleware.InstrumentHandler("DeleteKey", DeleteKey(service)))
			})
			mr.Route("/transactions", func(kr chi.Router) {
				kr.Method("GET", "/", middleware.InstrumentHandler("MerchantTransactions", MerchantTransactions(service)))
			})
		})
	})

	return r
}

// DeleteKeyRequest includes information needed to delete a key
type DeleteKeyRequest struct {
	DelaySeconds int `json:"delaySeconds" valid:"-"`
}

// CreateKeyRequest includes information needed to create a key
type CreateKeyRequest struct {
	Name string `json:"name" valid:"required"`
}

// CreateKey is the handler for creating keys for a merchant
func CreateKey(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		reqMerchant := chi.URLParam(r, "merchantID")

		var req CreateKeyRequest
		err := requestutils.ReadJSON(r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		encrypted, nonce, err := GenerateSecret()
		if err != nil {
			return handlers.WrapError(err, "Could not generate a secret key ", http.StatusInternalServerError)
		}

		key, err := service.Datastore.CreateKey(reqMerchant, req.Name, encrypted, nonce)
		if err != nil {
			return handlers.WrapError(err, "Error create api keys", http.StatusInternalServerError)
		}

		return handlers.RenderContent(r.Context(), key, w, http.StatusOK)
	})
}

// DeleteKey deletes a key
func DeleteKey(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var id = new(inputs.ID)
		if err := inputs.DecodeAndValidateString(context.Background(), id, chi.URLParam(r, "id")); err != nil {
			return handlers.ValidationError(
				"Error validating request url parameter",
				map[string]interface{}{
					"id": err.Error(),
				},
			)
		}

		var req DeleteKeyRequest
		err := requestutils.ReadJSON(r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		key, err := service.Datastore.DeleteKey(*id.UUID(), req.DelaySeconds)
		if err != nil {
			return handlers.WrapError(err, "Error updating keys for the merchant", http.StatusInternalServerError)
		}
		status := http.StatusOK
		if key == nil {
			status = http.StatusNotFound
		}

		return handlers.RenderContent(r.Context(), key, w, status)
	})
}

// GetKeys returns all keys for a specified merchant
func GetKeys(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		reqID := chi.URLParam(r, "merchantID")
		expired := r.URL.Query().Get("expired")
		showExpired := expired == "true"

		var keys *[]Key
		keys, err := service.Datastore.GetKeys(reqID, showExpired)
		if err != nil {
			return handlers.WrapError(err, "Error Getting Keys for Merchant", http.StatusInternalServerError)
		}

		return handlers.RenderContent(r.Context(), keys, w, http.StatusOK)
	})
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
	Email string             `json:"email" valid:"-"`
}

// CreateOrder is the handler for creating a new order
func CreateOrder(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {

		ctx := r.Context()
		sublogger := logging.Logger(ctx, "payments").With().Str("func", "CreateOrderHandler").Logger()

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
		// validation of sku tokens happens in createorderitemfrommacaroon
		order, err := service.CreateOrderFromRequest(ctx, req)

		if err != nil {
			if errors.Is(err, ErrInvalidSKU) {
				sublogger.Error().Err(err).Msg("invalid sku")
				return handlers.ValidationError(ErrInvalidSKU.Error(), nil)
			}
			sublogger.Error().Err(err).Msg("error creating the order")
			return handlers.WrapError(err, "Error creating the order in the database", http.StatusInternalServerError)
		}

		return handlers.RenderContent(r.Context(), order, w, http.StatusCreated)
	})
}

// CancelOrder is the handler for cancelling an order
func CancelOrder(service *Service) handlers.AppHandler {
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

		err := service.CancelOrder(*orderID.UUID())
		if err != nil {
			return handlers.WrapError(err, "Error retrieving the order", http.StatusInternalServerError)
		}

		return handlers.RenderContent(r.Context(), nil, w, http.StatusOK)
	})
}

// GetOrder is the handler for getting an order
func GetOrder(service *Service) handlers.AppHandler {
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

		order, err := service.GetOrder(*orderID.UUID())
		if err != nil {
			return handlers.WrapError(err, "Error retrieving the order", http.StatusInternalServerError)
		}

		status := http.StatusOK
		if order == nil {
			status = http.StatusNotFound
		}

		return handlers.RenderContent(r.Context(), order, w, status)
	})
}

// GetTransactions is the handler for listing the transactions for an order
func GetTransactions(service *Service) handlers.AppHandler {
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

		transactions, err := service.Datastore.GetTransactions(*orderID.UUID())
		if err != nil {
			return handlers.WrapError(err, "Error retrieving the transactions", http.StatusInternalServerError)
		}

		return handlers.RenderContent(r.Context(), transactions, w, http.StatusOK)
	})
}

// CreateTransactionRequest includes information needed to create a transaction
type CreateTransactionRequest struct {
	ExternalTransactionID uuid.UUID `json:"externalTransactionID" valid:"requiredUUID"`
}

// CreateGeminiTransaction creates a transaction against an order
func CreateGeminiTransaction(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var req CreateTransactionRequest
		err := requestutils.ReadJSON(r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		var orderID = new(inputs.ID)
		if err := inputs.DecodeAndValidateString(context.Background(), orderID, chi.URLParam(r, "orderID")); err != nil {
			return handlers.ValidationError(
				"Error validating request url parameter",
				map[string]interface{}{
					"orderID": err.Error(),
				},
			)
		}

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		// Ensure the external transaction ID hasn't already been added to any orders.
		transaction, err := service.Datastore.GetTransaction(req.ExternalTransactionID.String())
		if err != nil {
			return handlers.WrapError(err, "externalTransactinID has already been submitted to an order", http.StatusConflict)
		}

		if transaction != nil {
			err = fmt.Errorf("external Transaction ID: %s has already been added to the order", req.ExternalTransactionID.String())
			return handlers.WrapError(err, "Error creating the transaction", http.StatusBadRequest)
		}

		transaction, err = service.CreateTransactionFromRequest(r.Context(), req, *orderID.UUID(), getGeminiCustodialTx)
		if err != nil {
			return handlers.WrapError(err, "Error creating the transaction", http.StatusBadRequest)
		}

		return handlers.RenderContent(r.Context(), transaction, w, http.StatusCreated)
	})
}

// CreateUpholdTransaction creates a transaction against an order
func CreateUpholdTransaction(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var req CreateTransactionRequest
		err := requestutils.ReadJSON(r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		var orderID = new(inputs.ID)
		if err := inputs.DecodeAndValidateString(context.Background(), orderID, chi.URLParam(r, "orderID")); err != nil {
			return handlers.ValidationError(
				"Error validating request url parameter",
				map[string]interface{}{
					"orderID": err.Error(),
				},
			)
		}

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		// Ensure the external transaction ID hasn't already been added to any orders.
		transaction, err := service.Datastore.GetTransaction(req.ExternalTransactionID.String())
		if err != nil {
			return handlers.WrapError(err, "externalTransactinID has already been submitted to an order", http.StatusConflict)
		}

		if transaction != nil {
			err = fmt.Errorf("external Transaction ID: %s has already been added to the order", req.ExternalTransactionID.String())
			return handlers.WrapError(err, "Error creating the transaction", http.StatusBadRequest)
		}

		transaction, err = service.CreateTransactionFromRequest(r.Context(), req, *orderID.UUID(), getUpholdCustodialTx)
		if err != nil {
			return handlers.WrapError(err, "Error creating the transaction", http.StatusBadRequest)
		}

		return handlers.RenderContent(r.Context(), transaction, w, http.StatusCreated)
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
		ctx := r.Context()
		sublogger := logging.Logger(ctx, "payments").With().
			Str("func", "CreateAnonCardTransaction").
			Logger()
		var req CreateAnonCardTransactionRequest
		err := requestutils.ReadJSON(r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		var orderID = new(inputs.ID)
		if err := inputs.DecodeAndValidateString(context.Background(), orderID, chi.URLParam(r, "orderID")); err != nil {
			return handlers.ValidationError(
				"Error validating request url parameter",
				map[string]interface{}{
					"orderID": err.Error(),
				},
			)
		}

		transaction, err := service.CreateAnonCardTransaction(r.Context(), req.WalletID, req.Transaction, *orderID.UUID())
		if err != nil {
			sublogger.Error().Err(err).Msg("failed to create anon card transaction")
			return handlers.WrapError(err, "Error creating anon card transaction", http.StatusInternalServerError)
		}

		return handlers.RenderContent(r.Context(), transaction, w, http.StatusCreated)
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

		var orderID = new(inputs.ID)
		if err := inputs.DecodeAndValidateString(context.Background(), orderID, chi.URLParam(r, "orderID")); err != nil {
			return handlers.ValidationError(
				"Error validating request url parameter",
				map[string]interface{}{
					"orderID": err.Error(),
				},
			)
		}

		orderCreds, err := service.Datastore.GetOrderCredsByItemID(*orderID.UUID(), req.ItemID, false)
		if err != nil {
			return handlers.WrapError(err, "Error validating no credentials exist for order", http.StatusBadRequest)
		}
		if orderCreds != nil {
			return handlers.WrapError(err, "There are existing order credentials created for this order", http.StatusConflict)
		}

		err = service.CreateOrderCreds(r.Context(), *orderID.UUID(), req.ItemID, req.BlindedCreds)
		if err != nil {
			return handlers.WrapError(err, "Error creating order creds", http.StatusBadRequest)
		}

		return handlers.RenderContent(r.Context(), nil, w, http.StatusOK)
	})
}

// GetOrderCreds is the handler for fetching order credentials
func GetOrderCreds(service *Service) handlers.AppHandler {
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

		if creds == nil {
			return &handlers.AppError{
				Message: "Credentials do not exist",
				Code:    http.StatusNotFound,
				Data:    map[string]interface{}{},
			}
		}
		return handlers.RenderContent(r.Context(), creds, w, status)
	})
}

// DeleteOrderCreds is the handler for deleting order credentials
func DeleteOrderCreds(service *Service) handlers.AppHandler {
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
		// is signed param
		isSigned := r.URL.Query().Get("isSigned") == "true"

		err := service.Datastore.DeleteOrderCreds(*orderID.UUID(), isSigned)
		if err != nil {
			return handlers.WrapError(err, "Error deleting credentials", http.StatusBadRequest)
		}

		return handlers.RenderContent(r.Context(), "Order credentials successfully deleted", w, http.StatusOK)
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

		creds, err := service.Datastore.GetOrderCredsByItemID(*orderID.UUID(), *itemID.UUID(), false)
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
		if creds.SignedCreds == nil {
			status = http.StatusAccepted
		}

		return handlers.RenderContent(r.Context(), creds, w, status)
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

		logger, err := appctx.GetLogger(r.Context())
		if err != nil {
			_, logger = logging.SetupLogger(r.Context())
		}

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		err = service.Vote(r.Context(), req.Credentials, req.Vote)
		if err != nil {
			switch err.(type) {
			case govalidator.Error:
				logger.Warn().Err(err).Msg("failed vote validation")
				return handlers.WrapValidationError(err)
			case govalidator.Errors:
				logger.Warn().Err(err).Msg("failed multiple vote validation")
				return handlers.WrapValidationError(err)
			default:
				// check for custom vote invalidations
				if errors.Is(err, ErrInvalidSKUToken) {
					verr := handlers.ValidationError("failed to validate sku token", nil)
					data := []string{}
					if errors.Is(err, ErrInvalidSKUTokenSKU) {
						data = append(data, "invalid sku value")
					}
					if errors.Is(err, ErrInvalidSKUTokenBadMerchant) {
						data = append(data, "invalid merchant value")
					}
					verr.Data = data
					logger.Warn().Err(err).Msg("failed sku validations")
					return verr
				}
				logger.Warn().Err(err).Msg("failed to perform vote")
				return handlers.WrapError(err, "Error making vote", http.StatusBadRequest)
			}
		}

		w.WriteHeader(http.StatusOK)
		return nil
	})
}

// MerchantTransactions is the handler for getting paginated merchant transactions
func MerchantTransactions(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		// inputs
		// /merchants/{merchantID}/transactions?page=1&items=50&order=id
		var (
			merchantID, mIDErr      = inputs.NewMerchantID(r.Context(), chi.URLParam(r, "merchantID"))
			ctx, pagination, pIDErr = inputs.NewPagination(r.Context(), r.URL.String(), new(Transaction))
		)

		// Check Validation Errors
		if mIDErr != nil {
			return handlers.WrapValidationError(mIDErr)
		}
		if pIDErr != nil {
			return handlers.WrapValidationError(pIDErr)
		}

		// Get Paginated Results
		transactions, total, err := service.Datastore.GetPagedMerchantTransactions(
			ctx, merchantID.UUID(), pagination)
		if err != nil {
			return handlers.WrapError(err, "error getting transactions", http.StatusInternalServerError)
		}

		// Build Response
		response := &responses.PaginationResponse{
			Page:    pagination.Page,
			Items:   pagination.Items,
			MaxPage: total/pagination.Items - 1, // 0 indexed
			Ordered: pagination.RawOrder,
			Data:    transactions,
		}

		// render response
		if err := response.Render(ctx, w, http.StatusOK); err != nil {
			return handlers.WrapError(err, "error rendering response", http.StatusInternalServerError)
		}

		return nil
	})
}

// VerifyCredentialRequest includes an opaque subscription credential blob
type VerifyCredentialRequest struct {
	Type         string  `json:"type" valid:"in(single-use|time-limited)"`
	Version      float64 `json:"version" valid:"-"`
	SKU          string  `json:"sku" valid:"-"`
	MerchantID   string  `json:"merchantId" valid:"-"`
	Presentation string  `json:"presentation" valid:"base64"`
}

// VerifyCredential is the handler for verifying subscription credentials
func VerifyCredential(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var req VerifyCredentialRequest

		err := requestutils.ReadJSON(r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapError(err, "Error in request validation", http.StatusBadRequest)
		}

		if req.Type == "single-use" {
			var bytes []byte
			bytes, err = base64.StdEncoding.DecodeString(req.Presentation)
			if err != nil {
				return handlers.WrapError(err, "Error in decoding presentation", http.StatusBadRequest)
			}

			var decodedCredential cbr.CredentialRedemption
			err = json.Unmarshal(bytes, &decodedCredential)
			if err != nil {
				return handlers.WrapError(err, "Error in presentation formatting", http.StatusBadRequest)
			}

			// Ensure that the credential being redeemed (opaque to merchant) matches the outer credential details
			issuerID, err := encodeIssuerID(req.MerchantID, req.SKU)
			if err != nil {
				return handlers.WrapError(err, "Error in outer merchantId or sku", http.StatusBadRequest)
			}
			if issuerID != decodedCredential.Issuer {
				return handlers.WrapError(nil, "Error, outer merchant and sku don't match issuer", http.StatusBadRequest)
			}

			err = service.cbClient.RedeemCredential(r.Context(), decodedCredential.Issuer, decodedCredential.TokenPreimage, decodedCredential.Signature, decodedCredential.Issuer)
			if err != nil {
				return handlers.WrapError(err, "Error verifying credentials", http.StatusInternalServerError)
			}

			return handlers.RenderContent(r.Context(), "Credentials successfully verified", w, http.StatusOK)
		} else if req.Type == "time-limited" {
			// Presentation includes a token and token metadata test test
			type Presentation struct {
				ItemID    string `json:"itemId"`
				IssuedAt  string `json:"issuedAt"`
				ExpiresAt string `json:"expiresAt"`
				Token     string `json:"token"`
			}

			var bytes []byte
			bytes, err = base64.StdEncoding.DecodeString(req.Presentation)
			if err != nil {
				return handlers.WrapError(err, "Error in decoding presentation", http.StatusBadRequest)
			}

			var presentation Presentation
			err = json.Unmarshal(bytes, &presentation)
			if err != nil {
				return handlers.WrapError(err, "Error in presentation formatting", http.StatusBadRequest)
			}
			timeLimitedSecret := cryptography.NewTimeLimitedSecret([]byte(os.Getenv("BRAVE_MERCHANT_KEY")))

			issuedAt, err := time.Parse("2006-01-02", presentation.IssuedAt)
			if err != nil {
				return handlers.WrapError(err, "Error parsing issuedAt", http.StatusBadRequest)
			}
			expiresAt, err := time.Parse("2006-01-02", presentation.ExpiresAt)
			if err != nil {
				return handlers.WrapError(err, "Error parsing expiresAt", http.StatusBadRequest)
			}

			verified, err := timeLimitedSecret.Verify([]byte(presentation.ItemID), issuedAt, expiresAt, presentation.Token)
			if err != nil {
				return handlers.WrapError(err, "Error in token verification", http.StatusBadRequest)
			}

			if verified {
				// check against expiration time
				if time.Now().After(expiresAt) {
					return handlers.RenderContent(r.Context(), "Credentials expired", w, http.StatusBadRequest)
				}
				return handlers.RenderContent(r.Context(), "Credentials successfully verified", w, http.StatusOK)
			}

			return handlers.RenderContent(r.Context(), "Credentials could not be verified", w, http.StatusForbidden)

		}

		return handlers.WrapError(nil, "Unknown credential type", http.StatusBadRequest)
	})
}

// WebhookRouter - handles calls from various payment method webhooks informing payments of completion
func WebhookRouter(service *Service) chi.Router {
	r := chi.NewRouter()
	r.Method("POST", "/stripe", middleware.InstrumentHandler("HandleStripeWebhook", HandleStripeWebhook(service)))
	return r
}

// HandleStripeWebhook is the handler for stripe checkout session webhooks
func HandleStripeWebhook(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()
		// get logger
		sublogger := logging.Logger(ctx, "payments").With().
			Str("func", "HandleStripeWebhook").
			Logger()

		// get webhook secret from ctx
		endpointSecret, err := appctx.GetStringFromContext(ctx, appctx.StripeWebhookSecretCTXKey)
		if err != nil {
			sublogger.Error().Err(err).Msg("failed to get stripe_webhook_secret from context")
			return handlers.WrapError(
				err, "error getting stripe_webhook_secret from context",
				http.StatusInternalServerError)
		}

		b, err := requestutils.Read(r.Body)
		if err != nil {
			sublogger.Error().Err(err).Msg("failed to read request body")
			return handlers.WrapError(err, "error reading request body", http.StatusServiceUnavailable)
		}

		event, err := webhook.ConstructEvent(
			b, r.Header.Get("Stripe-Signature"), endpointSecret)
		if err != nil {
			sublogger.Error().Err(err).Msg("failed to verify stripe signature")
			return handlers.WrapError(err, "error verifying webhook signature", http.StatusBadRequest)
		}

		// log the event
		sublogger.Debug().Str("event_type", event.Type).Str("data", string(event.Data.Raw)).Msg("webhook event captured")

		// Handle invoice events
		if event.Type == StripeInvoiceUpdated {
			// Retrieve invoice from update events
			var invoice stripe.Invoice
			err := json.Unmarshal(event.Data.Raw, &invoice)
			if err != nil {
				sublogger.Error().Err(err).Msg("error parsing webhook json")
				return handlers.WrapError(err, "error parsing webhook JSON", http.StatusBadRequest)
			}
			sublogger.Debug().
				Str("event_type", event.Type).
				Str("invoice", fmt.Sprintf("%+v", invoice)).Msg("webhook invoice")

			subscription, err := service.scClient.Subscriptions.Get(invoice.Subscription.ID, nil)
			if err != nil {
				sublogger.Error().Err(err).Msg("error getting subscription")
				return handlers.WrapError(err, "error retrieving subscription", http.StatusInternalServerError)
			}

			sublogger.Debug().
				Str("subscription", fmt.Sprintf("%+v", subscription)).Msg("corresponding subscription")

			orderID, err := uuid.FromString(subscription.Metadata["orderID"])
			if err != nil {
				sublogger.Error().Err(err).Msg("error getting order id from subscription metadata")
				return handlers.WrapError(err, "error retrieving orderID", http.StatusInternalServerError)
			}

			sublogger.Debug().
				Str("orderID", orderID.String()).Msg("order id")

			// If the invoice is paid set order status to paid, otherwise
			if invoice.Paid {
				// is this an existing subscription??
				ok, subID, err := service.Datastore.IsStripeSub(orderID)
				if err != nil {
					sublogger.Error().Err(err).Msg("failed to tell if this is a stripe subscription")
					return handlers.WrapError(err, "error looking up payment provider", http.StatusInternalServerError)
				}
				if ok && subID != "" {
					// okay, this is a subscription renewal, not first time,
					err = service.Datastore.RenewOrder(ctx, orderID)
					if err != nil {
						sublogger.Error().Err(err).Msg("failed to renew the order")
						return handlers.WrapError(err, "error renewing order", http.StatusInternalServerError)
					}
					// end flow for renew order
					return handlers.RenderContent(r.Context(), "subscription renewed", w, http.StatusOK)
				}

				// not a renewal, first time
				err = service.Datastore.UpdateOrder(orderID, OrderStatusPaid)
				if err != nil {
					sublogger.Error().Err(err).Msg("failed to update order status")
					return handlers.WrapError(err, "error updating order status", http.StatusInternalServerError)
				}
				err = service.Datastore.UpdateOrderMetadata(orderID, "stripeSubscriptionId", subscription.ID)
				if err != nil {
					sublogger.Error().Err(err).Msg("failed to update order metadata")
					return handlers.WrapError(err, "error updating order metadata", http.StatusInternalServerError)
				}
				sublogger.Debug().Str("orderID", orderID.String()).Msg("order is now paid")
				return handlers.RenderContent(r.Context(), "payment successful", w, http.StatusOK)
			}

			sublogger.Debug().
				Str("orderID", orderID.String()).Msg("order not paid, set pending")
			err = service.Datastore.UpdateOrder(orderID, "pending")
			if err != nil {
				sublogger.Error().Err(err).Msg("failed to update order status")
				return handlers.WrapError(err, "error updating order status", http.StatusInternalServerError)
			}
			sublogger.Debug().
				Str("sub_id", subscription.ID).Msg("set subscription id in order metadata")
			err = service.Datastore.UpdateOrderMetadata(orderID, "stripeSubscriptionId", subscription.ID)
			if err != nil {
				sublogger.Error().Err(err).Msg("failed to update order metadata")
				return handlers.WrapError(err, "error updating order metadata", http.StatusInternalServerError)
			}
			sublogger.Debug().
				Str("sub_id", subscription.ID).Msg("set ok response")
			return handlers.RenderContent(r.Context(), "payment failed", w, http.StatusOK)
		}

		// Handle subscription cancellations
		if event.Type == StripeCustomerSubscriptionDeleted {
			var subscription stripe.Subscription
			err := json.Unmarshal(event.Data.Raw, &subscription)
			if err != nil {
				return handlers.WrapError(err, "error parsing webhook JSON", http.StatusBadRequest)
			}
			orderID, err := uuid.FromString(subscription.Metadata["orderID"])
			if err != nil {
				return handlers.WrapError(err, "error retrieving orderID", http.StatusInternalServerError)
			}
			err = service.Datastore.UpdateOrder(orderID, OrderStatusCanceled)
			if err != nil {
				return handlers.WrapError(err, "error updating order status", http.StatusInternalServerError)
			}
			return handlers.RenderContent(r.Context(), "subscription canceled", w, http.StatusOK)
		}

		return handlers.RenderContent(r.Context(), "event received", w, http.StatusOK)
	})
}
