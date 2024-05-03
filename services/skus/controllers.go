package skus

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/asaskevich/govalidator"
	"github.com/go-chi/chi"
	"github.com/go-chi/cors"
	"github.com/go-playground/validator/v10"
	uuid "github.com/satori/go.uuid"
	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/webhook"
	"google.golang.org/api/idtoken"

	"github.com/brave-intl/bat-go/libs/clients/radom"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/inputs"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/middleware"
	"github.com/brave-intl/bat-go/libs/requestutils"
	"github.com/brave-intl/bat-go/libs/responses"

	"github.com/brave-intl/bat-go/services/skus/handler"
	"github.com/brave-intl/bat-go/services/skus/model"
)

const (
	reqBodyLimit10MB = 10 << 20
)

type middlewareFn func(next http.Handler) http.Handler

func Router(
	svc *Service,
	authMwr middlewareFn,
	metricsMwr middleware.InstrumentHandlerDef,
	copts cors.Options,
) chi.Router {
	r := chi.NewRouter()

	orderh := handler.NewOrder(svc)

	corsMwrPost := NewCORSMwr(copts, http.MethodPost)

	if os.Getenv("ENV") == "local" {
		r.Method(
			http.MethodOptions,
			"/",
			metricsMwr("CreateOrderOptions", corsMwrPost(nil)),
		)

		r.Method(
			http.MethodPost,
			"/",
			metricsMwr(
				"CreateOrder",
				corsMwrPost(handlers.AppHandler(orderh.Create)),
			),
		)
	} else {
		r.Method(http.MethodPost, "/", metricsMwr("CreateOrder", handlers.AppHandler(orderh.Create)))
	}

	{
		corsMwrGet := NewCORSMwr(copts, http.MethodGet)
		r.Method(http.MethodOptions, "/{orderID}", metricsMwr("GetOrderOptions", corsMwrGet(nil)))
		r.Method(http.MethodGet, "/{orderID}", metricsMwr("GetOrder", corsMwrGet(GetOrder(svc))))
	}

	r.Method(
		http.MethodDelete,
		"/{orderID}",
		metricsMwr("CancelOrder", NewCORSMwr(copts, http.MethodDelete)(authMwr(CancelOrder(svc)))),
	)

	r.Method(
		http.MethodPatch,
		"/{orderID}/set-trial",
		metricsMwr("SetOrderTrialDays", NewCORSMwr(copts, http.MethodPatch)(authMwr(handleSetOrderTrialDays(svc)))),
	)

	r.Method(http.MethodGet, "/{orderID}/transactions", metricsMwr("GetTransactions", GetTransactions(svc)))
	r.Method(http.MethodPost, "/{orderID}/transactions/uphold", metricsMwr("CreateUpholdTransaction", CreateUpholdTransaction(svc)))
	r.Method(http.MethodPost, "/{orderID}/transactions/gemini", metricsMwr("CreateGeminiTransaction", CreateGeminiTransaction(svc)))

	r.Method(
		http.MethodPost,
		"/{orderID}/transactions/anonymousCard",
		metricsMwr("CreateAnonCardTransaction", CreateAnonCardTransaction(svc)),
	)

	// Receipt validation.
	{
		valid := validator.New()

		r.Method(http.MethodPost, "/{orderID}/submit-receipt", metricsMwr("SubmitReceipt", corsMwrPost(handleSubmitReceipt(svc, valid))))
		r.Method(http.MethodPost, "/receipt", metricsMwr("createOrderFromReceipt", corsMwrPost(handleCreateOrderFromReceipt(svc, valid))))
		r.Method(http.MethodPost, "/{orderID}/receipt", metricsMwr("checkOrderReceipt", authMwr(handleCheckOrderReceipt(svc, valid))))
	}

	r.Route("/{orderID}/credentials", func(cr chi.Router) {
		cr.Use(NewCORSMwr(copts, http.MethodGet, http.MethodPost))
		cr.Method(http.MethodGet, "/", metricsMwr("GetOrderCreds", GetOrderCreds(svc)))
		cr.Method(http.MethodPost, "/", metricsMwr("CreateOrderCreds", CreateOrderCreds(svc)))
		cr.Method(http.MethodDelete, "/", metricsMwr("DeleteOrderCreds", authMwr(DeleteOrderCreds(svc))))

		// Handle the old endpoint while the new is being rolled out:
		// - true: the handler uses itemID as the request id, which is the old mode;
		// - false: the handler uses the requestID from the URI.
		cr.Method(http.MethodGet, "/{itemID}", metricsMwr("GetOrderCredsByID", getOrderCredsByID(svc, true)))
		cr.Method(http.MethodGet, "/items/{itemID}/batches/{requestID}", metricsMwr("GetOrderCredsByID", getOrderCredsByID(svc, false)))

		cr.Method(http.MethodPut, "/items/{itemID}/batches/{requestID}", metricsMwr("CreateOrderItemCreds", createItemCreds(svc)))
	})

	return r
}

// CredentialRouter handles requests to /v1/credentials.
func CredentialRouter(svc *Service, authMwr middlewareFn) chi.Router {
	r := chi.NewRouter()

	r.Method(
		http.MethodPost,
		"/subscription/verifications",
		middleware.InstrumentHandler("VerifyCredentialV1", authMwr(VerifyCredentialV1(svc))),
	)

	return r
}

// CredentialV2Router handles requests to /v2/credentials.
func CredentialV2Router(svc *Service, authMwr middlewareFn) chi.Router {
	r := chi.NewRouter()

	r.Method(
		http.MethodPost,
		"/subscription/verifications",
		middleware.InstrumentHandler("VerifyCredentialV2", authMwr(VerifyCredentialV2(svc))),
	)

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

type setTrialDaysRequest struct {
	TrialDays int64 `json:"trialDays"`
}

// TODO: refactor this to avoid multiple fetches of an order.
func handleSetOrderTrialDays(svc *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()

		orderID, err := uuid.FromString(chi.URLParamFromCtx(ctx, "orderID"))
		if err != nil {
			return handlers.ValidationError("request", map[string]interface{}{"orderID": err.Error()})
		}

		if err := svc.validateOrderMerchantAndCaveats(ctx, orderID); err != nil {
			return handlers.ValidationError("merchant and caveats", map[string]interface{}{"orderMerchantAndCaveats": err.Error()})
		}

		data, err := io.ReadAll(io.LimitReader(r.Body, reqBodyLimit10MB))
		if err != nil {
			return handlers.WrapError(err, "failed to read request body", http.StatusBadRequest)
		}

		req := &setTrialDaysRequest{}
		if err := json.Unmarshal(data, req); err != nil {
			return handlers.WrapError(err, "failed to parse request", http.StatusBadRequest)
		}

		if err := svc.SetOrderTrialDays(ctx, &orderID, req.TrialDays); err != nil {
			return handlers.WrapError(err, "Error setting the trial days on the order", http.StatusInternalServerError)
		}

		return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
	})
}

// CancelOrder handles requests for cancelling orders.
func CancelOrder(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()
		orderID := &inputs.ID{}

		if err := inputs.DecodeAndValidateString(ctx, orderID, chi.URLParam(r, "orderID")); err != nil {
			return handlers.ValidationError(
				"Error validating request url parameter",
				map[string]interface{}{"orderID": err.Error()},
			)
		}

		oid := *orderID.UUID()

		if err := service.validateOrderMerchantAndCaveats(ctx, oid); err != nil {
			return handlers.WrapError(err, "Error validating auth merchant and caveats", http.StatusForbidden)
		}

		if err := service.CancelOrder(oid); err != nil {
			return handlers.WrapError(err, "Error retrieving the order", http.StatusInternalServerError)
		}

		return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
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

// CreateOrderCredsRequest includes the item ID and blinded credentials which to be signed.
type CreateOrderCredsRequest struct {
	ItemID       uuid.UUID `json:"itemId" valid:"-"`
	BlindedCreds []string  `json:"blindedCreds" valid:"base64"`
}

// CreateOrderCreds handles requests for creating credentials.
func CreateOrderCreds(svc *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()
		lg := logging.Logger(ctx, "skus.CreateOrderCreds")

		req := &CreateOrderCredsRequest{}
		if err := requestutils.ReadJSON(ctx, r.Body, req); err != nil {
			lg.Error().Err(err).Msg("failed to read body payload")
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		if _, err := govalidator.ValidateStruct(req); err != nil {
			lg.Error().Err(err).Msg("failed to validate struct")
			return handlers.WrapValidationError(err)
		}

		orderID := &inputs.ID{}
		if err := inputs.DecodeAndValidateString(ctx, orderID, chi.URLParam(r, "orderID")); err != nil {
			lg.Error().Err(err).Msg("failed to validate order id")
			return handlers.ValidationError(
				"Error validating request url parameter",
				map[string]interface{}{
					"orderID": err.Error(),
				},
			)
		}

		// Use the itemID for the request id so the old credential uniqueness constraint remains enforced.
		reqID := req.ItemID

		if err := svc.CreateOrderItemCredentials(ctx, *orderID.UUID(), req.ItemID, reqID, req.BlindedCreds); err != nil {
			lg.Error().Err(err).Msg("failed to create the order credentials")
			if errors.Is(err, errCredsAlreadySubmittedMismatch) {
				return handlers.WrapError(err, "Order credentials already exist", http.StatusConflict)
			}
			return handlers.WrapError(err, "Error creating order creds", http.StatusBadRequest)
		}

		return handlers.RenderContent(ctx, nil, w, http.StatusOK)
	}
}

// createItemCredsRequest includes the blinded credentials to be signed.
type createItemCredsRequest struct {
	BlindedCreds []string `json:"blindedCreds" valid:"base64"`
}

// createItemCreds handles requests for creating credentials for an item.
func createItemCreds(svc *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()
		lg := logging.Logger(ctx, "skus.createItemCreds")

		req := &createItemCredsRequest{}
		if err := requestutils.ReadJSON(ctx, r.Body, req); err != nil {
			lg.Error().Err(err).Msg("failed to read body payload")
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		if _, err := govalidator.ValidateStruct(req); err != nil {
			lg.Error().Err(err).Msg("failed to validate struct")
			return handlers.WrapValidationError(err)
		}

		orderID := &inputs.ID{}
		if err := inputs.DecodeAndValidateString(ctx, orderID, chi.URLParamFromCtx(ctx, "orderID")); err != nil {
			lg.Error().Err(err).Msg("failed to validate order id")
			return handlers.ValidationError("Error validating request url parameter", map[string]interface{}{
				"orderID": err.Error(),
			})
		}

		itemID := &inputs.ID{}
		if err := inputs.DecodeAndValidateString(ctx, itemID, chi.URLParamFromCtx(ctx, "itemID")); err != nil {
			lg.Error().Err(err).Msg("failed to validate item id")
			return handlers.ValidationError("Error validating request url parameter", map[string]interface{}{
				"itemID": err.Error(),
			})
		}

		reqID := &inputs.ID{}
		if err := inputs.DecodeAndValidateString(ctx, reqID, chi.URLParamFromCtx(ctx, "requestID")); err != nil {
			lg.Error().Err(err).Msg("failed to validate request id")
			return handlers.ValidationError("Error validating request url parameter", map[string]interface{}{
				"requestID": err.Error(),
			})
		}

		if err := svc.CreateOrderItemCredentials(ctx, *orderID.UUID(), *itemID.UUID(), *reqID.UUID(), req.BlindedCreds); err != nil {
			lg.Error().Err(err).Msg("failed to create the order credentials")
			if errors.Is(err, errCredsAlreadySubmittedMismatch) {
				return handlers.WrapError(err, "Order credentials already exist", http.StatusConflict)
			}
			return handlers.WrapError(err, "Error creating order creds", http.StatusBadRequest)
		}

		return handlers.RenderContent(ctx, nil, w, http.StatusOK)
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

// DeleteOrderCreds handles requests for deleting order credentials.
func DeleteOrderCreds(service *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()
		orderID := &inputs.ID{}
		if err := inputs.DecodeAndValidateString(ctx, orderID, chi.URLParam(r, "orderID")); err != nil {
			return handlers.ValidationError(
				"Error validating request url parameter",
				map[string]interface{}{"orderID": err.Error()},
			)
		}

		id := *orderID.UUID()
		if err := service.validateOrderMerchantAndCaveats(ctx, id); err != nil {
			return handlers.WrapError(err, "Error validating auth merchant and caveats", http.StatusForbidden)
		}

		isSigned := r.URL.Query().Get("isSigned") == "true"
		if err := service.DeleteOrderCreds(ctx, id, isSigned); err != nil {
			return handlers.WrapError(err, "Error deleting credentials", http.StatusBadRequest)
		}

		return handlers.RenderContent(ctx, "Order credentials successfully deleted", w, http.StatusOK)
	}
}

// getOrderCredsByID handles requests for fetching order credentials by an item id.
//
// Requests may come in via two endpoints:
// - /{itemID} – legacyMode, reqID == itemID
// - /items/{itemID}/batches/{requestID} – new mode, reqID == requestID.
//
// The legacy mode will be gone after confirming a successful rollout.
//
// TODO: Clean up the legacy mode.
func getOrderCredsByID(svc *Service, legacyMode bool) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()

		orderID := &inputs.ID{}
		if err := inputs.DecodeAndValidateString(ctx, orderID, chi.URLParamFromCtx(ctx, "orderID")); err != nil {
			return handlers.ValidationError("Error validating request url parameter", map[string]interface{}{
				"orderID": err.Error(),
			})
		}

		itemID := &inputs.ID{}
		if err := inputs.DecodeAndValidateString(ctx, itemID, chi.URLParamFromCtx(ctx, "itemID")); err != nil {
			return handlers.ValidationError("Error validating request url parameter", map[string]interface{}{
				"itemID": err.Error(),
			})
		}

		var reqID uuid.UUID
		if legacyMode {
			reqID = *itemID.UUID()
		} else {
			reqIDRaw := &inputs.ID{}
			if err := inputs.DecodeAndValidateString(ctx, reqIDRaw, chi.URLParamFromCtx(ctx, "requestID")); err != nil {
				return handlers.ValidationError("Error validating request url parameter", map[string]interface{}{
					"requestID": err.Error(),
				})
			}

			reqID = *reqIDRaw.UUID()
		}

		itemIDv := *itemID.UUID()
		creds, status, err := svc.GetItemCredentials(ctx, *orderID.UUID(), itemIDv, reqID)
		if err != nil {
			if !errors.Is(err, errSetRetryAfter) {
				return handlers.WrapError(err, "Error getting credentials", status)
			}

			// Add to response header as error specifies a retry after period.
			avg, err := svc.Datastore.GetOutboxMovAvgDurationSeconds()
			if err != nil {
				return handlers.WrapError(err, "Error getting credential retry-after", status)
			}

			w.Header().Set("Retry-After", strconv.FormatInt(avg, 10))
		}

		if legacyMode {
			suCreds, ok := creds.([]OrderCreds)
			if !ok {
				return handlers.WrapError(err, "Error getting credentials", http.StatusInternalServerError)
			}

			for i := range suCreds {
				if uuid.Equal(suCreds[i].ID, itemIDv) {
					return handlers.RenderContent(ctx, suCreds[i], w, status)
				}
			}

			return handlers.WrapError(err, "Error getting credentials", http.StatusNotFound)
		}

		if creds == nil {
			return handlers.RenderContent(ctx, map[string]interface{}{}, w, status)
		}

		return handlers.RenderContent(ctx, creds, w, status)
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
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {

		ctx := r.Context()
		l := logging.Logger(ctx, "VerifyCredentialV2")

		var req = new(VerifyCredentialRequestV2)
		if err := inputs.DecodeAndValidateReader(ctx, req, r.Body); err != nil {
			l.Error().Err(err).Msg("failed to read request")
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		appErr := service.verifyCredential(ctx, req, w)
		if appErr != nil {
			l.Error().Err(appErr).Msg("failed to verify credential")
		}

		return appErr
	}
}

// VerifyCredentialV1 is the handler for verifying subscription credentials
func VerifyCredentialV1(service *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()
		l := logging.Logger(r.Context(), "VerifyCredentialV1")

		var req = new(VerifyCredentialRequestV1)

		err := requestutils.ReadJSON(r.Context(), r.Body, &req)
		if err != nil {
			l.Error().Err(err).Msg("failed to read request")
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}
		l.Debug().Msg("read verify credential post body")

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			l.Error().Err(err).Msg("failed to validate request")
			return handlers.WrapError(err, "Error in request validation", http.StatusBadRequest)
		}

		appErr := service.verifyCredential(ctx, req, w)
		if appErr != nil {
			l.Error().Err(appErr).Msg("failed to verify credential")
		}

		return appErr
	}
}

// WebhookRouter - handles calls from various payment method webhooks informing payments of completion
func WebhookRouter(svc *Service) chi.Router {
	r := chi.NewRouter()

	r.Method(http.MethodPost, "/stripe", middleware.InstrumentHandler("HandleStripeWebhook", handleStripeWebhook(svc)))
	r.Method(http.MethodPost, "/radom", middleware.InstrumentHandler("HandleRadomWebhook", HandleRadomWebhook(svc)))
	r.Method(http.MethodPost, "/android", middleware.InstrumentHandler("HandleAndroidWebhook", HandleAndroidWebhook(svc)))
	r.Method(http.MethodPost, "/ios", middleware.InstrumentHandler("HandleIOSWebhook", handleIOSWebhook(svc)))
	r.Method(http.MethodPost, "/iosx", middleware.InstrumentHandler("handleWebhookAppStore", handleWebhookAppStore(svc)))

	return r
}

// HandleAndroidWebhook is the handler for the Google Playstore webhooks
func HandleAndroidWebhook(service *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()

		l := logging.Logger(ctx, "payments").With().Str("func", "HandleAndroidWebhook").Logger()

		if err := service.gcpValidator.validate(ctx, r); err != nil {
			l.Error().Err(err).Msg("invalid request")
			return handlers.WrapError(err, "invalid request", http.StatusUnauthorized)
		}

		payload, err := io.ReadAll(io.LimitReader(r.Body, reqBodyLimit10MB))
		if err != nil {
			l.Error().Err(err).Msg("failed to read payload")
			return handlers.WrapValidationError(err)
		}

		l.Info().Str("payload", string(payload)).Msg("")

		var validationErrMap = map[string]interface{}{}

		var req AndroidNotification
		if err := inputs.DecodeAndValidate(context.Background(), &req, payload); err != nil {
			validationErrMap["request-body-decode"] = err.Error()
			l.Error().Interface("validation_map", validationErrMap).Msg("validation_error")
			return handlers.ValidationError("Error validating request", validationErrMap)
		}

		l.Info().Interface("req", req).Msg("")

		dn, err := req.Message.GetDeveloperNotification()
		if err != nil {
			validationErrMap["invalid-developer-notification"] = err.Error()
			l.Error().Interface("validation_map", validationErrMap).Msg("validation_error")
			return handlers.ValidationError("Error validating request", validationErrMap)
		}

		l.Info().Interface("developer_notification", dn).Msg("")

		if dn == nil || dn.SubscriptionNotification.PurchaseToken == "" {
			validationErrMap["invalid-developer-notification-token"] = "notification has no purchase token"
			l.Error().Interface("validation_map", validationErrMap).Msg("validation_error")
			return handlers.ValidationError("Error validating request", validationErrMap)
		}

		l.Info().Msg("verify_developer_notification")

		if err := service.verifyDeveloperNotification(ctx, dn); err != nil {
			l.Error().Err(err).Msg("failed to verify subscription notification")

			switch {
			case errors.Is(err, errNotFound), errors.Is(err, model.ErrOrderNotFound):
				return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
			default:
				return handlers.WrapError(err, "failed to verify subscription notification", http.StatusInternalServerError)
			}
		}

		return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
	}
}

const (
	errAuthHeaderEmpty  model.Error = "skus: gcp authorization header is empty"
	errAuthHeaderFormat model.Error = "skus: gcp authorization header invalid format"
	errInvalidIssuer    model.Error = "skus: gcp invalid issuer"
	errInvalidEmail     model.Error = "skus: gcp invalid email"
	errEmailNotVerified model.Error = "skus: gcp email not verified"
)

type gcpTokenValidator interface {
	Validate(ctx context.Context, idToken string, audience string) (*idtoken.Payload, error)
}

type gcpValidatorConfig struct {
	audience       string
	issuer         string
	serviceAccount string
	disabled       bool
}

type gcpPushNotificationValidator struct {
	validator gcpTokenValidator
	cfg       gcpValidatorConfig
}

func newGcpPushNotificationValidator(gcpTokenValidator gcpTokenValidator, cfg gcpValidatorConfig) *gcpPushNotificationValidator {
	return &gcpPushNotificationValidator{
		validator: gcpTokenValidator,
		cfg:       cfg,
	}
}

func (g *gcpPushNotificationValidator) validate(ctx context.Context, r *http.Request) error {
	if g.cfg.disabled {
		return nil
	}

	ah := r.Header.Get("Authorization")
	if ah == "" {
		return errAuthHeaderEmpty
	}

	token := strings.Split(ah, " ")
	if len(token) != 2 {
		return errAuthHeaderFormat
	}

	p, err := g.validator.Validate(ctx, token[1], g.cfg.audience)
	if err != nil {
		return fmt.Errorf("invalid authentication token: %w", err)
	}

	if p.Issuer == "" || p.Issuer != g.cfg.issuer {
		return errInvalidIssuer
	}

	if p.Claims["email"] != g.cfg.serviceAccount {
		return errInvalidEmail
	}

	if p.Claims["email_verified"] != true {
		return errEmailNotVerified
	}

	return nil
}

func handleIOSWebhook(service *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()

		l := logging.Logger(ctx, "skus").With().Str("func", "handleIOSWebhook").Logger()

		data, err := io.ReadAll(io.LimitReader(r.Body, reqBodyLimit10MB))
		if err != nil {
			l.Error().Err(err).Msg("error reading request body")
			return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
		}

		req := &IOSNotification{}
		if err := inputs.DecodeAndValidate(ctx, req, data); err != nil {
			l.Warn().Err(err).Msg("failed to decode and validate the payload")

			return handlers.ValidationError("request", map[string]interface{}{"request-body-decode": err.Error()})
		}

		txInfo, err := req.GetTransactionInfo(ctx)
		if err != nil {
			l.Warn().Err(err).Msg("failed to get transaction info from message")

			return handlers.ValidationError("request", map[string]interface{}{"invalid-transaction-info": err.Error()})
		}

		renewalInfo, err := req.GetRenewalInfo(ctx)
		if err != nil {
			l.Warn().Err(err).Msg("failed to get renewal info from message")

			return handlers.ValidationError("request", map[string]interface{}{"invalid-renewal-info": err.Error()})
		}

		if err := service.verifyIOSNotification(ctx, txInfo, renewalInfo); err != nil {
			l.Error().Err(err).Msg("failed to verify ios subscription notification")

			switch {
			case errors.Is(err, errNotFound), errors.Is(err, model.ErrOrderNotFound):
				return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
			default:
				return handlers.WrapError(err, "failed to verify ios subscription notification", http.StatusInternalServerError)
			}
		}

		return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
	}
}

func handleWebhookAppStore(svc *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		return handleWebhookAppStoreH(w, r, svc)
	}
}

func handleWebhookAppStoreH(w http.ResponseWriter, r *http.Request, svc *Service) *handlers.AppError {
	ctx := r.Context()

	lg := logging.Logger(ctx, "skus").With().Str("func", "handleWebhookAppStore").Logger()

	data, err := io.ReadAll(io.LimitReader(r.Body, reqBodyLimit10MB))
	if err != nil {
		lg.Error().Err(err).Msg("failed to read request body")

		return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
	}

	spayload := &struct {
		SignedPayload string `json:"signedPayload"`
	}{}

	if err := json.Unmarshal(data, spayload); err != nil {
		lg.Error().Err(err).Str("data", string(data)).Msg("failed to unmarshal responseBodyV2")

		return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
	}

	ntf, err := parseAppStoreSrvNotification(svc.assnCertVrf, spayload.SignedPayload)
	if err != nil {
		// TODO: Reply with http.StatusOK after testing is complete.
		// None of these errors is recoverable.

		lg.Error().Err(err).Str("payload", spayload.SignedPayload).Msg("failed to parse app store notification")

		return handlers.ValidationError("request", map[string]interface{}{"parse-signed-payload": err.Error()})
	}

	if err := svc.processAppStoreNotification(ctx, ntf); err != nil {
		lg.Error().Err(err).Msg("failed to process app store notification")

		switch {
		case errors.Is(err, context.Canceled):
			// There is no const for 499.
			// Should retry.
			return handlers.WrapError(model.ErrSomethingWentWrong, "request has been cancelled", model.StatusClientClosedConn)

		case errors.Is(err, model.ErrOrderNotFound):
			// Order was not found, so nothing can be done.
			// It might be an issue for VPN:
			// - user has not linked yet;
			// - billing cycle comes through, subscription renews;
			// - user links immediately after billing cycle (they get 1 month);
			// - there might be a small gap between order's expiration and next successful renewal;
			// - the grace period should handle it.
			// A better option is to create orders when the user subscribes, similar to Leo.
			// Or allow for 1 or 2 retry attempts for the notification, but this requires tracking.
			// (Which can easily be done by setting ntf.val.NotificationUUID in Redis with EX).

			return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)

		case errors.Is(err, model.ErrNoRowsChangedOrder), errors.Is(err, model.ErrNoRowsChangedOrderPayHistory):
			// No rows have changed whilst processing.
			// This could happen in theory, but not in practice.
			// It would mean that we attempted to update with the same data as it's in the database.
			// This could happen when trying to process the same event twice, which could happen
			// if the App Store sends multiple notifications about the same event.
			// (E.g. auto-renew and billing recovery).

			return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)

		default:
			// Retry for all other errors for now.
			return handlers.WrapError(model.ErrSomethingWentWrong, "something went wrong", http.StatusInternalServerError)
		}
	}

	return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
}

// HandleRadomWebhook handles Radom checkout session webhooks.
func HandleRadomWebhook(service *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()

		lg := logging.Logger(ctx, "payments").With().Str("func", "HandleRadomWebhook").Logger()

		// Get webhook secret.
		endpointSecret, err := appctx.GetStringFromContext(ctx, appctx.RadomWebhookSecretCTXKey)
		if err != nil {
			lg.Error().Err(err).Msg("failed to get radom_webhook_secret from context")
			return handlers.WrapError(err, "error getting radom_webhook_secret from context", http.StatusInternalServerError)
		}

		// Check verification key.
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("radom-verification-key")), []byte(endpointSecret)) != 1 {
			lg.Error().Err(err).Msg("invalid verification key from webhook")
			return handlers.WrapError(err, "invalid verification key", http.StatusBadRequest)
		}

		req := radom.WebhookRequest{}
		if err := requestutils.ReadJSON(ctx, r.Body, &req); err != nil {
			lg.Error().Err(err).Msg("failed to read request body")
			return handlers.WrapError(err, "error reading request body", http.StatusServiceUnavailable)
		}

		lg.Debug().Str("event_type", req.EventType).Str("data", fmt.Sprintf("%+v", req)).Msg("webhook event captured")

		// Handle only successful payment events.
		if req.EventType != "managedRecurringPayment" && req.EventType != "newSubscription" {
			return handlers.WrapError(err, "event type not implemented", http.StatusBadRequest)
		}

		// Lookup the order, the checkout session was created with orderId in metadata.
		rawOrderID, err := req.Data.CheckoutSession.Metadata.Get("braveOrderId")
		if err != nil || rawOrderID == "" {
			return handlers.WrapError(err, "brave metadata not found in webhook", http.StatusBadRequest)
		}

		orderID, err := uuid.FromString(rawOrderID)
		if err != nil {
			return handlers.WrapError(err, "invalid braveOrderId in request", http.StatusBadRequest)
		}

		// Set order id to paid, and update metadata values.
		if err := service.Datastore.UpdateOrder(orderID, OrderStatusPaid); err != nil {
			lg.Error().Err(err).Msg("failed to update order status")
			return handlers.WrapError(err, "error updating order status", http.StatusInternalServerError)
		}

		if err := service.Datastore.AppendOrderMetadata(
			ctx, &orderID, "radomCheckoutSession", req.Data.CheckoutSession.CheckoutSessionID); err != nil {
			lg.Error().Err(err).Msg("failed to update order metadata")
			return handlers.WrapError(err, "error updating order metadata", http.StatusInternalServerError)
		}

		if req.EventType == "newSubscription" {

			if err := service.Datastore.AppendOrderMetadata(
				ctx, &orderID, "subscriptionId", req.EventData.NewSubscription.SubscriptionID); err != nil {
				lg.Error().Err(err).Msg("failed to update order metadata")
				return handlers.WrapError(err, "error updating order metadata", http.StatusInternalServerError)
			}

			if err := service.Datastore.AppendOrderMetadata(
				ctx, &orderID, "subscriptionContractAddress",
				req.EventData.NewSubscription.Subscription.AutomatedEVMSubscription.SubscriptionContractAddress); err != nil {

				lg.Error().Err(err).Msg("failed to update order metadata")
				return handlers.WrapError(err, "error updating order metadata", http.StatusInternalServerError)
			}

		}

		// Set paymentProcessor to Radom.
		if err := service.Datastore.AppendOrderMetadata(ctx, &orderID, "paymentProcessor", model.RadomPaymentMethod); err != nil {
			lg.Error().Err(err).Msg("failed to update order to add the payment processor")
			return handlers.WrapError(err, "failed to update order to add the payment processor", http.StatusInternalServerError)
		}

		lg.Debug().Str("orderID", orderID.String()).Msg("order is now paid")
		return handlers.RenderContent(ctx, "payment successful", w, http.StatusOK)
	}
}

func handleStripeWebhook(svc *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()

		lg := logging.Logger(ctx, "skus").With().Str("func", "HandleStripeWebhook").Logger()

		secret, err := appctx.GetStringFromContext(ctx, appctx.StripeWebhookSecretCTXKey)
		if err != nil {
			lg.Error().Err(err).Msg("failed to get stripe_webhook_secret from context")
			return handlers.WrapError(err, "error getting stripe_webhook_secret from context", http.StatusInternalServerError)
		}

		data, err := io.ReadAll(io.LimitReader(r.Body, reqBodyLimit10MB))
		if err != nil {
			lg.Error().Err(err).Msg("failed to read request body")
			return handlers.WrapError(err, "error reading request body", http.StatusServiceUnavailable)
		}

		event, err := webhook.ConstructEvent(data, r.Header.Get("Stripe-Signature"), secret)
		if err != nil {
			lg.Error().Err(err).Msg("failed to verify Stripe signature")
			return handlers.WrapError(err, "error verifying webhook signature", http.StatusBadRequest)
		}

		switch event.Type {
		case whStripeInvoiceUpdated, whStripeInvoicePaid:
			invoice := &stripe.Invoice{}
			if err := json.Unmarshal(event.Data.Raw, invoice); err != nil {
				lg.Error().Err(err).Msg("failed to parse invoice")
				return handlers.WrapError(err, "error parsing webhook invoice", http.StatusBadRequest)
			}

			sub, err := svc.scClient.Subscriptions.Get(invoice.Subscription.ID, nil)
			if err != nil {
				lg.Error().Err(err).Msg("failed to get subscription")

				if isErrStripeNotFound(err) {
					return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
				}

				return handlers.WrapError(err, "error retrieving subscription", http.StatusInternalServerError)
			}

			orderID, err := uuid.FromString(sub.Metadata["orderID"])
			if err != nil {
				lg.Error().Err(err).Msg("failed to parse orderID from Stripe metadata")
				return handlers.WrapError(err, "error retrieving orderID", http.StatusInternalServerError)
			}

			ord, err := svc.orderRepo.Get(ctx, svc.Datastore.RawDB(), orderID)
			if err != nil {
				lg.Error().Err(err).Msg("failed to get order")

				if errors.Is(err, model.ErrOrderNotFound) {
					return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
				}

				return handlers.WrapError(err, "failed to get order", http.StatusInternalServerError)
			}

			if subID, ok := ord.StripeSubID(); !ok || subID != sub.ID {
				if err := svc.Datastore.AppendOrderMetadata(ctx, &orderID, "stripeSubscriptionId", sub.ID); err != nil {
					lg.Error().Err(err).Msg("failed to update order metadata stripeSubscriptionId")
					return handlers.WrapError(err, "failed to update order metadata stripeSubscriptionId", http.StatusInternalServerError)
				}
			}

			switch event.Type {
			case whStripeInvoiceUpdated:
				return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)

			case whStripeInvoicePaid:
				if err := svc.RenewOrder(ctx, orderID); err != nil {
					lg.Error().Err(err).Msg("failed to renew order")
					return handlers.WrapError(err, "error renewing order", http.StatusInternalServerError)
				}

				if err := svc.Datastore.AppendOrderMetadata(ctx, &orderID, "paymentProcessor", model.StripePaymentMethod); err != nil {
					lg.Error().Err(err).Msg("failed to update order metadata paymentProcessor")
					return handlers.WrapError(err, "failed to update order metadata paymentProcessor", http.StatusInternalServerError)
				}

				return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)

			default:
				return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
			}

		case whStripeCustSubscriptionDeleted:
			// TODO: Enable it and handle properly.

			sub := &stripe.Subscription{}
			if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
				return handlers.WrapError(err, "failed to parse subscription", http.StatusBadRequest)
			}

			orderID, err := uuid.FromString(sub.Metadata["orderID"])
			if err != nil {
				return handlers.WrapError(err, "failed to parse orderID from Stripe metadata", http.StatusInternalServerError)
			}

			if err := svc.Datastore.UpdateOrder(orderID, OrderStatusCanceled); err != nil {
				return handlers.WrapError(err, "failed to update order status canceled", http.StatusInternalServerError)
			}

			return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
		}

		return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
	}
}

func handleSubmitReceipt(svc *Service, valid *validator.Validate) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()

		l := logging.Logger(ctx, "skus").With().Str("func", "SubmitReceipt").Logger()

		orderID, err := uuid.FromString(chi.URLParamFromCtx(ctx, "orderID"))
		if err != nil {
			l.Warn().Err(err).Msg("failed to decode orderID")

			// Preserve the legacy error in case anything depends on it.
			return handlers.ValidationError("request", map[string]interface{}{"orderID": inputs.ErrIDDecodeNotUUID})
		}

		payload, err := io.ReadAll(io.LimitReader(r.Body, reqBodyLimit10MB))
		if err != nil {
			l.Warn().Err(err).Msg("failed to read body")

			return handlers.ValidationError("request", map[string]interface{}{"request-body": err.Error()})
		}

		// TODO(clD11): remove when no longer needed.
		payloadS := string(payload)
		l.Info().Interface("payload_byte", payload).Str("payload_str", payloadS).Msg("payload")

		req, err := parseSubmitReceiptRequest(payload)
		if err != nil {
			l.Warn().Err(err).Msg("failed to deserialize request")

			return handlers.ValidationError("request", map[string]interface{}{"request-body": err.Error()})
		}

		if err := valid.StructCtx(ctx, &req); err != nil {
			verrs, ok := collectValidationErrors(err)
			if !ok {
				return handlers.ValidationError("request", map[string]interface{}{"request-body": err.Error()})
			}

			return handlers.ValidationError("request", verrs)
		}

		// TODO(clD11): remove when no longer needed.
		l.Info().Interface("req_decoded", req).Msg("req decoded")

		extID, err := svc.validateReceipt(ctx, req)
		if err != nil {
			l.Warn().Err(err).Msg("failed to validate receipt with vendor")

			return handleReceiptErr(err)
		}

		{
			_, err := svc.orderRepo.GetByExternalID(ctx, svc.Datastore.RawDB(), extID)
			if err != nil && !errors.Is(err, model.ErrOrderNotFound) {
				l.Warn().Err(err).Msg("failed to lookup external id")

				return handlers.WrapError(err, "failed to lookup external id", http.StatusInternalServerError)
			}

			if err == nil {
				return handlers.WrapError(model.ErrReceiptAlreadyLinked, "receipt has already been submitted", http.StatusConflict)
			}
		}

		mdata := newMobileOrderMdata(req, extID)

		if err := svc.UpdateOrderStatusPaidWithMetadata(ctx, &orderID, mdata); err != nil {
			l.Warn().Err(err).Msg("failed to update order with vendor metadata")
			return handlers.WrapError(err, "failed to store status of order", http.StatusInternalServerError)
		}

		result := struct {
			ExternalID string `json:"externalId"`
			Vendor     string `json:"vendor"`
		}{ExternalID: extID, Vendor: req.Type.String()}

		return handlers.RenderContent(ctx, result, w, http.StatusOK)
	}
}

func handleCreateOrderFromReceipt(svc *Service, valid *validator.Validate) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		return handleCreateOrderFromReceiptH(w, r, svc, valid)
	}
}

func handleCreateOrderFromReceiptH(w http.ResponseWriter, r *http.Request, svc *Service, valid *validator.Validate) *handlers.AppError {
	ctx := r.Context()

	lg := logging.Logger(ctx, "skus").With().Str("func", "handleCreateOrderFromReceipt").Logger()

	raw, err := io.ReadAll(io.LimitReader(r.Body, reqBodyLimit10MB))
	if err != nil {
		lg.Warn().Err(err).Msg("failed to read request")

		return handlers.ValidationError("request", map[string]interface{}{"request-body": err.Error()})
	}

	req, err := parseSubmitReceiptRequest(raw)
	if err != nil {
		lg.Warn().Err(err).Msg("failed to deserialize request")

		return handlers.ValidationError("request", map[string]interface{}{"request-body": err.Error()})
	}

	if err := valid.StructCtx(ctx, &req); err != nil {
		verrs, ok := collectValidationErrors(err)
		if !ok {
			return handlers.ValidationError("request", map[string]interface{}{"request-body": err.Error()})
		}

		return handlers.ValidationError("request", verrs)
	}

	extID, err := svc.validateReceipt(ctx, req)
	if err != nil {
		lg.Warn().Err(err).Msg("failed to validate receipt with vendor")

		return handleReceiptErr(err)
	}

	{
		ord, err := svc.orderRepo.GetByExternalID(ctx, svc.Datastore.RawDB(), extID)
		if err != nil && !errors.Is(err, model.ErrOrderNotFound) {
			lg.Warn().Err(err).Msg("failed to lookup external id")

			return handlers.WrapError(err, "failed to lookup external id", http.StatusInternalServerError)
		}

		if err == nil {
			result := model.CreateOrderWithReceiptResponse{ID: ord.ID.String()}

			return handlers.RenderContent(ctx, result, w, http.StatusConflict)
		}
	}

	ord, err := svc.createOrderWithReceipt(ctx, req, extID)
	if err != nil {
		lg.Warn().Err(err).Msg("failed to create order")

		return handlers.WrapError(err, "failed to create order", http.StatusInternalServerError)
	}

	result := model.CreateOrderWithReceiptResponse{ID: ord.ID.String()}

	return handlers.RenderContent(ctx, result, w, http.StatusCreated)
}

func handleCheckOrderReceipt(svc *Service, valid *validator.Validate) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		return handleCheckOrderReceiptH(w, r, svc, valid)
	}
}

func handleCheckOrderReceiptH(w http.ResponseWriter, r *http.Request, svc *Service, valid *validator.Validate) *handlers.AppError {
	ctx := r.Context()

	lg := logging.Logger(ctx, "skus").With().Str("func", "handleCheckOrderReceipt").Logger()

	orderID, err := uuid.FromString(chi.URLParamFromCtx(ctx, "orderID"))
	if err != nil {
		lg.Warn().Err(err).Msg("failed to parse orderID")

		return handlers.ValidationError("request", map[string]interface{}{"orderID": err.Error()})
	}

	raw, err := io.ReadAll(io.LimitReader(r.Body, reqBodyLimit10MB))
	if err != nil {
		lg.Warn().Err(err).Msg("failed to read request")

		return handlers.ValidationError("request", map[string]interface{}{"request-body": err.Error()})
	}

	req, err := parseSubmitReceiptRequest(raw)
	if err != nil {
		lg.Warn().Err(err).Msg("failed to deserialize request")

		return handlers.ValidationError("request", map[string]interface{}{"request-body": err.Error()})
	}

	if err := valid.StructCtx(ctx, &req); err != nil {
		verrs, ok := collectValidationErrors(err)
		if !ok {
			return handlers.ValidationError("request", map[string]interface{}{"request-body": err.Error()})
		}

		return handlers.ValidationError("request", verrs)
	}

	extID, err := svc.validateReceipt(ctx, req)
	if err != nil {
		lg.Warn().Err(err).Msg("failed to validate receipt with vendor")

		return handleReceiptErr(err)
	}

	if err := svc.checkOrderReceipt(ctx, orderID, extID); err != nil {
		lg.Warn().Err(err).Msg("failed to check order receipt")

		switch {
		case errors.Is(err, model.ErrOrderNotFound):
			return handlers.WrapError(err, "order not found by receipt", http.StatusNotFound)
		case errors.Is(err, model.ErrNoMatchOrderReceipt):
			return handlers.WrapError(err, "order_id does not match receipt order", http.StatusFailedDependency)
		default:
			return handlers.WrapError(model.ErrSomethingWentWrong, "failed to check order receipt", http.StatusInternalServerError)
		}
	}

	return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
}

func NewCORSMwr(opts cors.Options, methods ...string) func(next http.Handler) http.Handler {
	opts.AllowedMethods = methods

	return cors.Handler(opts)
}

func NewCORSOpts(origins []string, dbg bool) cors.Options {
	result := cors.Options{
		Debug:            dbg,
		AllowedOrigins:   origins,
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{""},
		AllowCredentials: false,
		MaxAge:           300, // Maximum value not ignored by any of major browsers
	}

	return result
}

func handleReceiptErr(err error) *handlers.AppError {
	if err == nil {
		return &handlers.AppError{
			Message: "Unexpected error",
			Code:    http.StatusInternalServerError,
			Data:    map[string]interface{}{},
		}
	}

	errStr := err.Error()
	result := &handlers.AppError{
		Message: "Error " + errStr,
		Code:    http.StatusBadRequest,
		Data: map[string]interface{}{
			"validationErrors": map[string]interface{}{"receiptErrors": errStr},
		},
	}

	switch {
	case errors.Is(err, errPurchaseFailed):
		result.ErrorCode = purchaseFailedErrCode
	case errors.Is(err, errPurchasePending):
		result.ErrorCode = purchasePendingErrCode
	case errors.Is(err, errPurchaseDeferred):
		result.ErrorCode = purchaseDeferredErrCode
	case errors.Is(err, errPurchaseStatusUnknown):
		result.ErrorCode = purchaseStatusUnknownErrCode
	default:
		result.ErrorCode = purchaseValidationErrCode
	}

	return result
}

func parseSubmitReceiptRequest(raw []byte) (model.ReceiptRequest, error) {
	buf := make([]byte, base64.StdEncoding.DecodedLen(len(raw)))

	n, err := base64.StdEncoding.Decode(buf, raw)
	if err != nil {
		return model.ReceiptRequest{}, fmt.Errorf("failed to decode input base64: %w", err)
	}

	result := model.ReceiptRequest{}
	if err := json.Unmarshal(buf[:n], &result); err != nil {
		return model.ReceiptRequest{}, fmt.Errorf("failed to decode input json: %w", err)
	}

	return result, nil
}

func collectValidationErrors(err error) (map[string]string, bool) {
	var verr validator.ValidationErrors
	if !errors.As(err, &verr) {
		return nil, false
	}

	result := make(map[string]string, len(verr))
	for i := range verr {
		result[verr[i].Field()] = verr[i].Error()
	}

	return result, true
}
