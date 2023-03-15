package skus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/asaskevich/govalidator"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/datastore"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/inputs"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/middleware"
	"github.com/brave-intl/bat-go/libs/requestutils"
	"github.com/brave-intl/bat-go/libs/responses"
	"github.com/go-chi/chi"
	"github.com/go-chi/cors"
	uuid "github.com/satori/go.uuid"
	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/webhook"
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
func Router(service *Service, instrumentHandler middleware.InstrumentHandlerDef) chi.Router {
	r := chi.NewRouter()
	merchantSignedMiddleware := service.MerchantSignedMiddleware()

	if os.Getenv("ENV") == "local" {
		r.Method("OPTIONS", "/", middleware.InstrumentHandler("CreateOrderOptions", corsMiddleware([]string{"POST"})(nil)))
		r.Method("POST", "/", middleware.InstrumentHandler("CreateOrder", corsMiddleware([]string{"POST"})(CreateOrder(service))))
	} else {
		r.Method("POST", "/", middleware.InstrumentHandler("CreateOrder", CreateOrder(service)))
	}

	r.Method("OPTIONS", "/{orderID}", middleware.InstrumentHandler("GetOrderOptions", corsMiddleware([]string{"GET"})(nil)))
	r.Method("GET", "/{orderID}", middleware.InstrumentHandler("GetOrder", corsMiddleware([]string{"GET"})(GetOrder(service))))

	r.Method("DELETE", "/{orderID}", middleware.InstrumentHandler("CancelOrder", corsMiddleware([]string{"DELETE"})(merchantSignedMiddleware(CancelOrder(service)))))
	r.Method("PATCH", "/{orderID}/set-trial", middleware.InstrumentHandler("SetOrderTrialDays", corsMiddleware([]string{"PATCH"})(merchantSignedMiddleware(SetOrderTrialDays(service)))))

	r.Method("GET", "/{orderID}/transactions", middleware.InstrumentHandler("GetTransactions", GetTransactions(service)))
	r.Method("POST", "/{orderID}/transactions/uphold", middleware.InstrumentHandler("CreateUpholdTransaction", CreateUpholdTransaction(service)))
	r.Method("POST", "/{orderID}/transactions/gemini", middleware.InstrumentHandler("CreateGeminiTransaction", CreateGeminiTransaction(service)))
	r.Method("POST", "/{orderID}/transactions/anonymousCard", instrumentHandler("CreateAnonCardTransaction", CreateAnonCardTransaction(service)))

	// api routes for order receipt validation
	r.Method("POST", "/{orderID}/submit-receipt", middleware.InstrumentHandler("SubmitReceipt", corsMiddleware([]string{"POST"})(SubmitReceipt(service))))

	r.Route("/{orderID}/credentials", func(cr chi.Router) {
		cr.Use(corsMiddleware([]string{"GET", "POST"}))
		cr.Method("POST", "/", middleware.InstrumentHandler("CreateOrderCreds", CreateOrderCreds(service)))
		cr.Method("GET", "/", middleware.InstrumentHandler("GetOrderCreds", GetOrderCreds(service)))
		cr.Method("GET", "/{itemID}", middleware.InstrumentHandler("GetOrderCredsByID", GetOrderCredsByID(service)))
		cr.Method("DELETE", "/", middleware.InstrumentHandler("DeleteOrderCreds", merchantSignedMiddleware(DeleteOrderCreds(service))))
	})

	return r
}

// CredentialRouter handles calls relating to credentials
func CredentialRouter(service *Service) chi.Router {
	r := chi.NewRouter()
	merchantSignedMiddleware := service.MerchantSignedMiddleware()

	r.Method("POST", "/subscription/verifications", middleware.InstrumentHandler("VerifyCredentialV1", merchantSignedMiddleware(VerifyCredentialV1(service))))
	return r
}

// CredentialV2Router handles calls relating to credentials
func CredentialV2Router(service *Service) chi.Router {
	r := chi.NewRouter()
	merchantSignedMiddleware := service.MerchantSignedMiddleware()

	r.Method("POST", "/subscription/verifications", middleware.InstrumentHandler("VerifyCredentialV2", merchantSignedMiddleware(VerifyCredentialV2(service))))
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

// CreateKeyResponse includes information about the created key
type CreateKeyResponse struct {
	*Key
	SecretKey string `json:"secretKey"`
}

// CreateKey is the handler for creating keys for a merchant
func CreateKey(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		reqMerchant := chi.URLParam(r, "merchantID")

		var req CreateKeyRequest
		err := requestutils.ReadJSON(r.Context(), r.Body, &req)
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

		sk, err := key.GetSecretKey()
		if err != nil {
			return handlers.WrapError(err, "Error create api keys", http.StatusInternalServerError)
		}

		if sk == nil {
			err = errors.New("secret key was nil")
			return handlers.WrapError(err, "Error create api keys", http.StatusInternalServerError)
		}

		resp := CreateKeyResponse{
			Key:       key,
			SecretKey: *sk,
		}
		return handlers.RenderContent(r.Context(), resp, w, http.StatusOK)
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
		err := requestutils.ReadJSON(r.Context(), r.Body, &req)
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
		merchantID := chi.URLParam(r, "merchantID")
		expired := r.URL.Query().Get("expired")
		showExpired := expired == "true"

		var keys *[]Key
		keys, err := service.Datastore.GetKeysByMerchant(merchantID, showExpired)
		if err != nil {
			return handlers.WrapError(err, "Error Getting Keys for Merchant", http.StatusInternalServerError)
		}

		return handlers.RenderContent(r.Context(), keys, w, http.StatusOK)
	})
}

// VoteRouter for voting endpoint
func VoteRouter(service *Service, instrumentHandler middleware.InstrumentHandlerDef) chi.Router {
	r := chi.NewRouter()
	r.Method("POST", "/", instrumentHandler("MakeVote", MakeVote(service)))
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
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {

		ctx := r.Context()
		sublogger := logging.Logger(ctx, "payments").With().Str("func", "CreateOrderHandler").Logger()

		var req CreateOrderRequest
		err := requestutils.ReadJSON(r.Context(), r.Body, &req)
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
	}
}

// SetOrderTrialDaysInput - SetOrderTrialDays handler input
type SetOrderTrialDaysInput struct {
	TrialDays int64 `json:"trialDays" valid:"int"`
}

// SetOrderTrialDays is the handler for cancelling an order
func SetOrderTrialDays(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var (
			ctx     = r.Context()
			orderID = new(inputs.ID)
		)
		if err := inputs.DecodeAndValidateString(context.Background(), orderID, chi.URLParam(r, "orderID")); err != nil {
			return handlers.ValidationError(
				"Error validating request url parameter",
				map[string]interface{}{
					"orderID": err.Error(),
				},
			)
		}

		// validate order merchant and caveats (to make sure this is the right merch)
		if err := service.ValidateOrderMerchantAndCaveats(r, *orderID.UUID()); err != nil {
			return handlers.ValidationError(
				"Error validating request merchant and caveats",
				map[string]interface{}{
					"orderMerchantAndCaveats": err.Error(),
				},
			)
		}

		var input SetOrderTrialDaysInput
		err := requestutils.ReadJSON(r.Context(), r.Body, &input)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		_, err = govalidator.ValidateStruct(input)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		err = service.SetOrderTrialDays(ctx, orderID.UUID(), input.TrialDays)
		if err != nil {
			return handlers.WrapError(err, "Error setting the trial days on the order", http.StatusInternalServerError)
		}

		return handlers.RenderContent(r.Context(), nil, w, http.StatusOK)
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

		err := service.ValidateOrderMerchantAndCaveats(r, *orderID.UUID())
		if err != nil {
			return handlers.WrapError(err, "Error validating auth merchant and caveats", http.StatusForbidden)
		}

		err = service.CancelOrder(*orderID.UUID())
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
	ExternalTransactionID string `json:"externalTransactionId" valid:"required,uuid"`
}

// CreateGeminiTransaction creates a transaction against an order
func CreateGeminiTransaction(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var req CreateTransactionRequest
		err := requestutils.ReadJSON(r.Context(), r.Body, &req)
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
		transaction, err := service.Datastore.GetTransaction(req.ExternalTransactionID)
		if err != nil {
			return handlers.WrapError(err, "externalTransactinID has already been submitted to an order", http.StatusConflict)
		}

		if transaction != nil {
			// if the transaction is already added, then do an update
			transaction, err = service.UpdateTransactionFromRequest(r.Context(), req, *orderID.UUID(), service.getGeminiCustodialTx)
			if err != nil {
				return handlers.WrapError(err, "Error updating the transaction", http.StatusBadRequest)
			}
			// return 200 in event of already created transaction
			return handlers.RenderContent(r.Context(), transaction, w, http.StatusOK)
		}

		transaction, err = service.CreateTransactionFromRequest(r.Context(), req, *orderID.UUID(), service.getGeminiCustodialTx)
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
		err := requestutils.ReadJSON(r.Context(), r.Body, &req)
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
		transaction, err := service.Datastore.GetTransaction(req.ExternalTransactionID)
		if err != nil {
			return handlers.WrapError(err, "externalTransactinID has already been submitted to an order", http.StatusConflict)
		}

		if transaction != nil {
			// if the transaction is already added, then do an update
			transaction, err = service.UpdateTransactionFromRequest(r.Context(), req, *orderID.UUID(), getUpholdCustodialTxWithRetries)
			if err != nil {
				return handlers.WrapError(err, "Error updating the transaction", http.StatusBadRequest)
			}
			// return 200 in event of already created transaction
			return handlers.RenderContent(r.Context(), transaction, w, http.StatusOK)
		}

		transaction, err = service.CreateTransactionFromRequest(r.Context(), req, *orderID.UUID(), getUpholdCustodialTxWithRetries)
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
		err := requestutils.ReadJSON(r.Context(), r.Body, &req)
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
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var (
			req    = new(CreateOrderCredsRequest)
			ctx    = r.Context()
			logger = logging.Logger(ctx, "skus.CreateOrderCreds")
		)
		err := requestutils.ReadJSON(r.Context(), r.Body, req)
		if err != nil {
			logger.Error().Err(err).Msg("failed to read body payload")
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			logger.Error().Err(err).Msg("failed to validate struct")
			return handlers.WrapValidationError(err)
		}

		var orderID = new(inputs.ID)
		if err := inputs.DecodeAndValidateString(context.Background(), orderID, chi.URLParam(r, "orderID")); err != nil {
			logger.Error().Err(err).Msg("failed to validate order id")
			return handlers.ValidationError(
				"Error validating request url parameter",
				map[string]interface{}{
					"orderID": err.Error(),
				},
			)
		}

		orderItem, err := service.Datastore.GetOrderItem(r.Context(), req.ItemID)
		if err != nil {
			logger.Error().Err(err).Msg("error getting the order item for creds")
			return handlers.WrapError(err, "Error validating no credentials exist for order", http.StatusBadRequest)
		}

		// TLV2 check to see if we have credentials signed that match incoming blinded tokens
		if orderItem.CredentialType == timeLimitedV2 {
			alreadySubmitted, err := service.Datastore.AreTimeLimitedV2CredsSubmitted(r.Context(), req.BlindedCreds...)
			if err != nil {
				// This is an existing error message so don't want to change it incase client are relying on it.
				return handlers.WrapError(err, "Error validating credentials exist for order", http.StatusBadRequest)
			}
			if alreadySubmitted {
				// since these are already submitted, no need to create order credentials
				// return ok
				return handlers.RenderContent(r.Context(), nil, w, http.StatusOK)
			}
		}

		// check if we already have a signing request for this order, delete order creds will
		// delete the prior signing request.  this allows subscriptions to manage how many
		// order creds are handed out.
		signingOrderRequests, err := service.Datastore.GetSigningOrderRequestOutboxByOrderItem(r.Context(), req.ItemID)
		if err != nil {
			// This is an existing error message so don't want to change it incase client are relying on it.
			return handlers.WrapError(err, "Error validating no credentials exist for order", http.StatusBadRequest)
		}

		if len(signingOrderRequests) > 0 {
			return handlers.WrapError(err, "There are existing order credentials created for this order", http.StatusConflict)
		}

		err = service.CreateOrderItemCredentials(r.Context(), *orderID.UUID(), req.ItemID, req.BlindedCreds)
		if err != nil {
			logger.Error().Err(err).Msg("failed to create the order credentials")
			return handlers.WrapError(err, "Error creating order creds", http.StatusBadRequest)
		}

		return handlers.RenderContent(r.Context(), nil, w, http.StatusOK)
	}
}

// GetOrderCreds is the handler for fetching all order credentials associated with an order.
// This endpoint handles the retrieval of all order credential types i.e. single-use, time-limited and time-limited-v2.
func GetOrderCreds(service *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var orderID = new(inputs.ID)

		if err := inputs.DecodeAndValidateString(context.Background(), orderID, chi.URLParam(r, "orderID")); err != nil {
			return handlers.ValidationError(
				"Error validating request url parameter",
				map[string]interface{}{
					"orderID": err.Error(),
				},
			)
		}

		creds, status, err := service.GetCredentials(r.Context(), *orderID.UUID())
		if err != nil {
			if errors.Is(err, errSetRetryAfter) {
				// error specifies a retry after period, add to response header
				avg, err := service.Datastore.GetOutboxMovAvgDurationSeconds()
				if err != nil {
					return handlers.WrapError(err, "Error getting credential retry-after", status)
				}
				w.Header().Set("Retry-After", strconv.FormatInt(avg, 10))
			} else {
				return handlers.WrapError(err, "Error getting credentials", status)
			}
		}
		return handlers.RenderContent(r.Context(), creds, w, status)
	}
}

// DeleteOrderCreds is the handler for deleting order credentials
func DeleteOrderCreds(service *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var orderID = new(inputs.ID)
		if err := inputs.DecodeAndValidateString(context.Background(), orderID, chi.URLParam(r, "orderID")); err != nil {
			return handlers.ValidationError(
				"Error validating request url parameter",
				map[string]interface{}{
					"orderID": err.Error(),
				},
			)
		}

		err := service.ValidateOrderMerchantAndCaveats(r, *orderID.UUID())
		if err != nil {
			return handlers.WrapError(err, "Error validating auth merchant and caveats", http.StatusForbidden)
		}

		// is signed param
		isSigned := r.URL.Query().Get("isSigned") == "true"

		err = service.DeleteOrderCreds(r.Context(), *orderID.UUID(), isSigned)
		if err != nil {
			return handlers.WrapError(err, "Error deleting credentials", http.StatusBadRequest)
		}

		return handlers.RenderContent(r.Context(), "Order credentials successfully deleted", w, http.StatusOK)
	}
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

		creds, status, err := service.GetItemCredentials(r.Context(), *orderID.UUID(), *itemID.UUID())
		if err != nil {
			if errors.Is(err, errSetRetryAfter) {
				// error specifies a retry after period, add to response header
				avg, err := service.Datastore.GetOutboxMovAvgDurationSeconds()
				if err != nil {
					return handlers.WrapError(err, "Error getting credential retry-after", status)
				}
				w.Header().Set("Retry-After", strconv.FormatInt(avg, 10))
			} else {
				return handlers.WrapError(err, "Error getting credentials", status)
			}
		}
		if creds == nil {
			return handlers.RenderContent(r.Context(), map[string]interface{}{}, w, status)
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
		var (
			req VoteRequest
			ctx = r.Context()
		)
		err := requestutils.ReadJSON(ctx, r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		logger := logging.Logger(ctx, "skus.MakeVote")

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		err = service.Vote(ctx, req.Credentials, req.Vote)
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

// VerifyCredentialV2 - version 2 of verify credential
func VerifyCredentialV2(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()
		logger := logging.Logger(ctx, "VerifyCredentialV2")
		logger.Debug().Msg("starting VerifyCredentialV2 controller")

		var req = new(VerifyCredentialRequestV2)
		if err := inputs.DecodeAndValidateReader(ctx, req, r.Body); err != nil {
			logger.Error().Err(err).Msg("failed to read request")
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		return service.verifyCredential(ctx, req, w)
	})
}

// VerifyCredentialV1 is the handler for verifying subscription credentials
func VerifyCredentialV1(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()

		logger := logging.Logger(r.Context(), "VerifyCredentialV1")
		logger.Debug().Msg("starting VerifyCredentialV1 controller")

		var req = new(VerifyCredentialRequestV1)

		err := requestutils.ReadJSON(r.Context(), r.Body, &req)
		if err != nil {
			logger.Error().Err(err).Msg("failed to read request")
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}
		logger.Debug().Msg("read verify credential post body")

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			logger.Error().Err(err).Msg("failed to validate request")
			return handlers.WrapError(err, "Error in request validation", http.StatusBadRequest)
		}

		logger.Debug().Msg("validated verify credential post body")

		return service.verifyCredential(ctx, req, w)
	})
}

// WebhookRouter - handles calls from various payment method webhooks informing payments of completion
func WebhookRouter(service *Service) chi.Router {
	r := chi.NewRouter()
	r.Method("POST", "/stripe", middleware.InstrumentHandler("HandleStripeWebhook", HandleStripeWebhook(service)))
	r.Method("POST", "/android", middleware.InstrumentHandler("HandleAndroidWebhook", HandleAndroidWebhook(service)))
	r.Method("POST", "/ios", middleware.InstrumentHandler("HandleIOSWebhook", HandleIOSWebhook(service)))
	return r
}

// HandleAndroidWebhook is the handler for the Google Playstore webhooks
func HandleAndroidWebhook(service *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {

		var (
			ctx              = r.Context()
			req              = new(AndroidNotification)
			validationErrMap = map[string]interface{}{} // for tracking our validation errors
		)

		// get logger
		logger := logging.Logger(ctx, "payments").With().
			Str("func", "HandleAndroidWebhook").
			Logger()

		// read the payload
		payload, err := requestutils.Read(r.Context(), r.Body)
		if err != nil {
			logger.Error().Err(err).Msg("failed to read the payload")
			return handlers.WrapValidationError(err)
		}

		// validate the payload
		if err := inputs.DecodeAndValidate(context.Background(), req, payload); err != nil {
			logger.Debug().Str("payload", string(payload)).
				Msg("failed to decode and validate the payload")
			validationErrMap["request-body-decode"] = err.Error()
		}

		// extract out the Developer notification
		dn, err := req.Message.GetDeveloperNotification()
		if err != nil {
			validationErrMap["invalid-developer-notification"] = err.Error()
		}

		if dn == nil || dn.SubscriptionNotification.PurchaseToken == "" {
			logger.Error().Interface("validation-errors", validationErrMap).
				Msg("failed to get developer notification from message")
			validationErrMap["invalid-developer-notification-token"] = "notification has no purchase token"
		}

		// if we had any validation errors, return the validation error map to the caller
		if len(validationErrMap) != 0 {
			return handlers.ValidationError("Error validating request url", validationErrMap)
		}

		err = service.verifyDeveloperNotification(ctx, dn)
		if err != nil {
			logger.Error().Err(err).Msg("failed to verify subscription notification")
			switch {
			case errors.Is(err, errNotFound):
				return handlers.WrapError(err, "failed to verify subscription notification",
					http.StatusNotFound)
			default:
				return handlers.WrapError(err, "failed to verify subscription notification",
					http.StatusInternalServerError)
			}
		}

		return handlers.RenderContent(ctx, "event received", w, http.StatusOK)
	}
}

// HandleIOSWebhook is the handler for ios iap webhooks
func HandleIOSWebhook(service *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {

		var (
			ctx              = r.Context()
			req              = new(IOSNotification)
			validationErrMap = map[string]interface{}{} // for tracking our validation errors
		)

		// get logger
		logger := logging.Logger(ctx, "payments").With().
			Str("func", "HandleIOSWebhook").
			Logger()

		// read the payload
		payload, err := requestutils.Read(r.Context(), r.Body)
		if err != nil {
			logger.Error().Err(err).Msg("failed to read the payload")
			// no need to go further
			return handlers.WrapValidationError(err)
		}

		// validate the payload
		if err := inputs.DecodeAndValidate(context.Background(), req, payload); err != nil {
			logger.Debug().Str("payload", string(payload)).Msg("failed to decode and validate the payload")
			logger.Warn().Err(err).Msg("failed to decode and validate the payload")
			validationErrMap["request-body-decode"] = err.Error()
		}

		// transaction info
		txInfo, err := req.GetTransactionInfo(ctx)
		if err != nil {
			logger.Warn().Err(err).Msg("failed to get transaction info from message")
			validationErrMap["invalid-transaction-info"] = err.Error()
		}

		// renewal info
		renewalInfo, err := req.GetRenewalInfo(ctx)
		if err != nil {
			logger.Warn().Err(err).Msg("failed to get renewal info from message")
			validationErrMap["invalid-renewal-info"] = err.Error()
		}

		// if we had any validation errors, return the validation error map to the caller
		if len(validationErrMap) != 0 {
			return handlers.ValidationError("Error validating request url", validationErrMap)
		}

		err = service.verifyIOSNotification(ctx, txInfo, renewalInfo)
		if err != nil {
			logger.Error().Err(err).Msg("failed to verify ios subscription notification")
			switch {
			case errors.Is(err, errNotFound):
				return handlers.WrapError(err, "failed to verify ios subscription notification",
					http.StatusNotFound)
			default:
				return handlers.WrapError(err, "failed to verify ios subscription notification",
					http.StatusInternalServerError)
			}
		}
		return handlers.RenderContent(ctx, "event received", w, http.StatusOK)
	}
}

// HandleStripeWebhook is the handler for stripe checkout session webhooks
func HandleStripeWebhook(service *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
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

		b, err := requestutils.Read(r.Context(), r.Body)
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
		if event.Type == StripeInvoiceUpdated || event.Type == StripeInvoicePaid {
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
					err = service.RenewOrder(ctx, orderID)
					if err != nil {
						sublogger.Error().Err(err).Msg("failed to renew the order")
						return handlers.WrapError(err, "error renewing order", http.StatusInternalServerError)
					}
					// end flow for renew order
					return handlers.RenderContent(r.Context(), "subscription renewed", w, http.StatusOK)
				}

				// not a renewal, first time
				// and update the order's expires at as it was just paid
				err = service.Datastore.UpdateOrder(orderID, OrderStatusPaid)
				if err != nil {
					sublogger.Error().Err(err).Msg("failed to update order status")
					return handlers.WrapError(err, "error updating order status", http.StatusInternalServerError)
				}
				err = service.Datastore.AppendOrderMetadata(ctx, &orderID, "stripeSubscriptionId", subscription.ID)
				if err != nil {
					sublogger.Error().Err(err).Msg("failed to update order metadata")
					return handlers.WrapError(err, "error updating order metadata", http.StatusInternalServerError)
				}
				// set paymentProcessor as stripe
				err = service.Datastore.AppendOrderMetadata(ctx, &orderID, paymentProcessor, StripePaymentMethod)
				if err != nil {
					sublogger.Error().Err(err).Msg("failed to update order to add the payment processor")
					return handlers.WrapError(err, "failed to update order to add the payment processor", http.StatusInternalServerError)
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
			err = service.Datastore.AppendOrderMetadata(ctx, &orderID, "stripeSubscriptionId", subscription.ID)
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
	}
}

// SubmitReceipt submit a vendor verifiable receipt that proves order is paid
func SubmitReceipt(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {

		var (
			ctx              = r.Context()
			req              SubmitReceiptRequestV1     // the body of the request
			orderID          = new(inputs.ID)           // the order id
			validationErrMap = map[string]interface{}{} // for tracking our validation errors
		)

		logger := logging.Logger(ctx, "skus").With().Str("func", "SubmitReceipt").Logger()

		// validate the order id
		if err := inputs.DecodeAndValidateString(context.Background(), orderID, chi.URLParam(r, "orderID")); err != nil {
			logger.Warn().Err(err).Msg("Failed to decode/validate order id from url")
			validationErrMap["orderID"] = err.Error()
		}

		// read the payload
		payload, err := requestutils.Read(r.Context(), r.Body)
		if err != nil {
			logger.Warn().Err(err).Msg("Failed to read the payload")
			validationErrMap["request-body"] = err.Error()
		}

		// validate the payload
		if err := inputs.DecodeAndValidate(context.Background(), &req, payload); err != nil {
			logger.Debug().Str("payload", string(payload)).Msg("Failed to decode and validate the payload")
			logger.Warn().Err(err).Msg("Failed to decode and validate the payload")
			validationErrMap["request-body"] = err.Error()
		}

		// validate the receipt
		externalID, err := service.validateReceipt(ctx, orderID.UUID(), req)
		if err != nil {
			if errors.Is(err, errNotFound) {
				return handlers.WrapError(err, "order not found", http.StatusNotFound)
			}
			logger.Warn().Err(err).Msg("Failed to validate the receipt with vendor")
			validationErrMap["receiptErrors"] = err.Error()
			// return codified errors for application
			if errors.Is(err, errPurchaseFailed) {
				return handlers.CodedValidationError(err.Error(), purchaseFailedErrCode, validationErrMap)
			} else if errors.Is(err, errPurchasePending) {
				return handlers.CodedValidationError(err.Error(), purchasePendingErrCode, validationErrMap)
			} else if errors.Is(err, errPurchaseDeferred) {
				return handlers.CodedValidationError(err.Error(), purchaseDeferredErrCode, validationErrMap)
			} else if errors.Is(err, errPurchaseStatusUnknown) {
				return handlers.CodedValidationError(err.Error(), purchaseStatusUnknownErrCode, validationErrMap)
			} else {
				// unknown error
				return handlers.CodedValidationError("error validating receipt", purchaseValidationErrCode, validationErrMap)
			}
		}

		// if we had any validation errors, return the validation error map to the caller
		if len(validationErrMap) != 0 {
			return handlers.ValidationError("error validating request", validationErrMap)
		}
		// does this external id exist already
		exists, err := service.ExternalIDExists(ctx, externalID)
		if err != nil {
			logger.Warn().Err(err).Msg("failed to lookup external id existance")
			return handlers.WrapError(err, "failed to lookup external id", http.StatusInternalServerError)
		}

		if exists {
			return handlers.WrapError(err, "receipt has already been submitted", http.StatusBadRequest)
		}

		// set order paid and include the vendor and external id to metadata
		if err := service.UpdateOrderStatusPaidWithMetadata(ctx, orderID.UUID(), datastore.Metadata{
			"vendor":         req.Type.String(),
			"externalID":     externalID,
			paymentProcessor: req.Type.String(),
		}); err != nil {
			logger.Warn().Err(err).Msg("Failed to update the order with appropriate metadata")
			return handlers.WrapError(err, "failed to store status of order", http.StatusInternalServerError)
		}

		return handlers.RenderContent(r.Context(), SubmitReceiptResponseV1{
			ExternalID: externalID,
			Vendor:     req.Type.String(),
		}, w, http.StatusOK)
	})
}
