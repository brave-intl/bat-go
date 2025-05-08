package skus

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/go-chi/chi"
	"github.com/go-chi/cors"
	"github.com/go-playground/validator/v10"
	uuid "github.com/satori/go.uuid"
	"github.com/stripe/stripe-go/v72/webhook"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/inputs"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/middleware"
	"github.com/brave-intl/bat-go/libs/requestutils"
	"github.com/brave-intl/bat-go/libs/responses"

	"github.com/brave-intl/bat-go/services/skus/handler"
	"github.com/brave-intl/bat-go/services/skus/model"
	"github.com/brave-intl/bat-go/services/skus/radom"
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
		r.Method(http.MethodGet, "/{orderID}", metricsMwr("GetOrder", corsMwrGet(handleGetOrder(svc))))
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

	r.Method(
		http.MethodPost,
		"/{orderID}/transactions/anonymousCard",
		metricsMwr("CreateAnonCardTransaction", CreateAnonCardTransaction(svc)),
	)

	// Receipt validation.
	{
		valid := validator.New()

		// /submit-receipt is deprecated.
		// Use /receipt instead.
		// It received 0 requests in June 2024.
		r.Method(http.MethodPost, "/{orderID}/submit-receipt", metricsMwr("SubmitReceipt", corsMwrPost(handleSubmitReceipt(svc, valid))))
		r.Method(http.MethodPost, "/receipt", metricsMwr("createOrderFromReceipt", corsMwrPost(handleCreateOrderFromReceipt(svc, valid))))
		r.Method(http.MethodPost, "/{orderID}/receipt", metricsMwr("checkOrderReceipt", authMwr(handleCheckOrderReceipt(svc, valid))))
	}

	credh := handler.NewCred(svc)

	r.Route("/{orderID}/credentials", func(cr chi.Router) {
		cr.Use(NewCORSMwr(copts, http.MethodGet, http.MethodPost))
		cr.Method(http.MethodGet, "/", metricsMwr("GetOrderCreds", GetOrderCreds(svc)))
		cr.Method(http.MethodPost, "/", metricsMwr("CreateOrderCreds", CreateOrderCreds(svc)))
		cr.Method(http.MethodDelete, "/", metricsMwr("DeleteOrderCreds", authMwr(deleteOrderCreds(svc))))

		// For now, this endpoint is placed directly under /credentials.
		// It would make sense to put it under /items/item_id, had the caller known the item id.
		// However, the caller of this endpoint does not possess that knowledge, and it would have to call the order endpoint to get it.
		// This extra round-trip currently does not make sense.
		// So until Bundles came along we can benefit from the fact that there is one item per order.
		// By the time Bundles arrive, the caller would either have to fetch order anyway, or this can be communicated in another way.
		cr.Method(http.MethodGet, "/batches/count", metricsMwr("CountBatches", authMwr(handlers.AppHandler(credh.CountBatches))))

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

	valid := validator.New()

	r.Method(
		http.MethodPost,
		"/subscription/verifications",
		middleware.InstrumentHandler("handleVerifyCredV1", authMwr(handleVerifyCredV1(svc, valid))),
	)

	return r
}

// CredentialV2Router handles requests to /v2/credentials.
func CredentialV2Router(svc *Service, authMwr middlewareFn) chi.Router {
	r := chi.NewRouter()

	valid := validator.New()

	r.Method(
		http.MethodPost,
		"/subscription/verifications",
		middleware.InstrumentHandler("handleVerifyCredV2", authMwr(handleVerifyCredV2(svc, valid))),
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

func handleSetOrderTrialDays(svc *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()

		orderID, err := uuid.FromString(chi.URLParamFromCtx(ctx, "orderID"))
		if err != nil {
			return handlers.ValidationError("request", map[string]interface{}{"orderID": err.Error()})
		}

		data, err := io.ReadAll(io.LimitReader(r.Body, reqBodyLimit10MB))
		if err != nil {
			return handlers.WrapError(err, "failed to read request body", http.StatusBadRequest)
		}

		req := &model.SetTrialDaysRequest{}
		if err := json.Unmarshal(data, req); err != nil {
			return handlers.WrapError(err, "failed to parse request", http.StatusBadRequest)
		}

		now := time.Now().UTC()

		if err := svc.setOrderTrialDays(ctx, orderID, req, now); err != nil {
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

		if err := service.CancelOrderLegacy(oid); err != nil {
			return handlers.WrapError(err, "Error retrieving the order", http.StatusInternalServerError)
		}

		return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
	})
}

func handleGetOrder(svc *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()

		lg := logging.Logger(ctx, "skus").With().Str("func", "handleGetOrder").Logger()

		orderID, err := uuid.FromString(chi.URLParamFromCtx(ctx, "orderID"))
		if err != nil {
			lg.Err(err).Msg("failed to parse order id")

			return handlers.ValidationError("request", map[string]interface{}{"orderID": err.Error()})
		}

		order, err := svc.getTransformOrder(ctx, orderID)
		if err != nil {
			lg.Err(err).Msg("failed to get transform order")

			switch {
			case errors.Is(err, context.Canceled):
				return handlers.WrapError(model.ErrSomethingWentWrong, "request has been cancelled", model.StatusClientClosedConn)

			case errors.Is(err, model.ErrOrderNotFound):
				return handlers.WrapError(err, "order not found", http.StatusNotFound)

			default:
				return handlers.WrapError(err, "Error retrieving the order", http.StatusInternalServerError)
			}
		}

		if isRadomCheckoutSession(order) {
			sid, ok := order.RadomSessID()
			if !ok {
				lg.Err(model.ErrNoRadomCheckoutSessionID).Msg("failed to get radom session id")

				return handlers.WrapError(model.ErrNoRadomCheckoutSessionID, "radom session id not found", http.StatusInternalServerError)
			}

			order.UpdateCheckoutSessionID(sid)
		}

		return handlers.RenderContent(ctx, order, w, http.StatusOK)
	}
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
			lg.Err(err).Msg("failed to create the order credentials")

			switch {
			case errors.Is(err, model.ErrOrderNotFound):
				return handlers.WrapError(err, "order not found", http.StatusNotFound)

			case errors.Is(err, errCredsAlreadySubmittedMismatch):
				return handlers.WrapError(err, "Order credentials already exist", http.StatusConflict)

			default:
				return handlers.WrapError(err, "Error creating order creds", http.StatusBadRequest)
			}
		}

		return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
	}
}

// createItemCredsRequest includes the blinded credentials to be signed.
type createItemCredsRequest struct {
	BlindedCreds []string `json:"blindedCreds" valid:"base64"`
}

// createItemCreds handles requests for creating credentials for an item.
func createItemCreds(svc *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		b, err := io.ReadAll(io.LimitReader(r.Body, reqBodyLimit10MB))
		if err != nil {
			return handlers.WrapError(err, "error reading body", http.StatusBadRequest)
		}

		req := &createItemCredsRequest{}
		if err := json.Unmarshal(b, req); err != nil {
			return handlers.WrapError(err, "error decoding body", http.StatusBadRequest)
		}

		ctx := r.Context()
		lg := logging.Logger(ctx, "skus.createItemCreds")

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
			lg.Err(err).Msg("failed to create the order credentials")

			switch {
			case errors.Is(err, model.ErrOrderNotFound):
				return handlers.WrapError(err, "order not found", http.StatusNotFound)

			case errors.Is(err, errCredsAlreadySubmittedMismatch):
				return handlers.WrapError(err, "Order credentials already exist", http.StatusConflict)

			default:
				return handlers.WrapError(err, "Error creating order creds", http.StatusBadRequest)
			}
		}

		return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
	}
}

// GetOrderCreds is the handler for fetching all order credentials associated with an order.
// This endpoint handles the retrieval of all order credential types i.e. single-use, time-limited and time-limited-v2.
func GetOrderCreds(service *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()

		var orderID = new(inputs.ID)

		if err := inputs.DecodeAndValidateString(ctx, orderID, chi.URLParam(r, "orderID")); err != nil {
			return handlers.ValidationError(
				"Error validating request url parameter",
				map[string]interface{}{
					"orderID": err.Error(),
				},
			)
		}

		l := logging.Logger(ctx, "skus").With().Str("func", "GetOrderCreds").Logger()

		creds, status, err := service.GetCredentials(ctx, *orderID.UUID())
		if err != nil {
			if errors.Is(err, errSetRetryAfter) {
				// error specifies a retry after period, add to response header
				avg, err := service.Datastore.GetOutboxMovAvgDurationSeconds()
				if err != nil {
					return handlers.WrapError(err, "Error getting credential retry-after", status)
				}
				w.Header().Set("Retry-After", strconv.FormatInt(avg, 10))
			} else {
				l.Error().Err(err).Str("orderID", orderID.String()).Int("status", status).Msg("failed to get order creds")
				return handlers.WrapError(err, "Error getting credentials", status)
			}
		}
		return handlers.RenderContent(ctx, creds, w, status)
	}
}

// deleteOrderCreds handles requests for deleting order credentials.
func deleteOrderCreds(service *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()

		orderID, err := uuid.FromString(chi.URLParamFromCtx(ctx, "orderID"))
		if err != nil {
			return handlers.ValidationError("orderID", map[string]interface{}{"orderID": err.Error()})
		}

		if err := service.validateOrderMerchantAndCaveats(ctx, orderID); err != nil {
			return handlers.WrapError(err, "Error validating auth merchant and caveats", http.StatusForbidden)
		}

		isSigned := r.URL.Query().Get("isSigned") == "true"
		if err := service.DeleteOrderCreds(ctx, orderID, isSigned); err != nil {
			switch {
			case errors.Is(err, context.Canceled):
				return handlers.WrapError(err, "cliend ended request", model.StatusClientClosedConn)

			case errors.Is(err, model.ErrOrderNotFound):
				return handlers.WrapError(err, "order not found", http.StatusNotFound)

			case errors.Is(err, model.ErrInvalidOrderNoItems):
				return handlers.WrapError(err, "order has no items", http.StatusBadRequest)

			default:
				return handlers.WrapError(model.ErrSomethingWentWrong, "failed to delete credentials", http.StatusBadRequest)
			}
		}

		return handlers.RenderContent(ctx, "Order credentials successfully deleted", w, http.StatusOK)
	}
}

const errTypeAssertion = model.Error("skus: type assertion")

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

		l := logging.Logger(ctx, "skus").With().Str("func", "getOrderCredsByID").Bool("legacy_mode", legacyMode).Logger()

		orderID := &inputs.ID{}
		if err := inputs.DecodeAndValidateString(ctx, orderID, chi.URLParamFromCtx(ctx, "orderID")); err != nil {
			l.Err(err).Msg("failed to decode and validate string for orderID")

			return handlers.ValidationError("Error validating request url parameter", map[string]interface{}{
				"orderID": err.Error(),
			})
		}

		itemID := &inputs.ID{}
		if err := inputs.DecodeAndValidateString(ctx, itemID, chi.URLParamFromCtx(ctx, "itemID")); err != nil {
			l.Err(err).Msg("failed to decode and validate string for itemID")

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
				l.Err(err).Msg("failed to decode and validate string reqIDRaw")

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
				l.Err(err).Str("orderID", orderID.String()).Str("itemID", itemIDv.String()).Int("status", status).Msg("failed to get item creds")
				return handlers.WrapError(err, "Error getting credentials", status)
			}

			// Add to response header as error specifies a retry after period.
			avg, err := svc.Datastore.GetOutboxMovAvgDurationSeconds()
			if err != nil {
				l.Err(err).Str("orderID", orderID.String()).Str("itemID", itemIDv.String()).Int("status", status).Msg("failed to get obx mov avg")
				return handlers.WrapError(err, "Error getting credential retry-after", status)
			}

			w.Header().Set("Retry-After", strconv.FormatInt(avg, 10))
		}

		if legacyMode {
			suCreds, ok := creds.([]OrderCreds)
			if !ok {
				l.Err(errTypeAssertion).Str("orderID", orderID.String()).Str("itemID", itemIDv.String()).Int("status", http.StatusInternalServerError).Msg("error getting credentials type assertion")
				return handlers.WrapError(err, "Error getting credentials", http.StatusInternalServerError)
			}

			for i := range suCreds {
				if uuid.Equal(suCreds[i].ID, itemIDv) {
					return handlers.RenderContent(ctx, suCreds[i], w, status)
				}
			}

			l.Err(errNotFound).Str("orderID", orderID.String()).Str("itemID", itemIDv.String()).Int("status", http.StatusNotFound).Msg("error finding creds legacy")

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
		ctx := r.Context()

		req := VoteRequest{}
		if err := requestutils.ReadJSON(ctx, r.Body, &req); err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		if _, err := govalidator.ValidateStruct(req); err != nil {
			return handlers.WrapValidationError(err)
		}

		lg := logging.Logger(ctx, "skus").With().Str("func", "MakeVote").Logger()

		if err := service.Vote(ctx, req.Credentials, req.Vote); err != nil {
			switch err.(type) {
			case govalidator.Error:
				lg.Warn().Err(err).Msg("failed vote validation")
				return handlers.WrapValidationError(err)

			case govalidator.Errors:
				lg.Warn().Err(err).Msg("failed multiple vote validation")
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
					lg.Warn().Err(err).Msg("failed sku validations")

					return verr
				}

				lg.Warn().Err(err).Msg("failed to perform vote")

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

func handleVerifyCredV2(svc *Service, valid *validator.Validate) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()

		lg := logging.Logger(ctx, "skus").With().Str("func", "handleVerifyCredV2").Logger()

		data, err := io.ReadAll(io.LimitReader(r.Body, reqBodyLimit10MB))
		if err != nil {
			lg.Warn().Err(err).Msg("failed to read body")

			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		req, err := parseVerifyCredRequestV2(data)
		if err != nil {
			lg.Warn().Err(err).Msg("failed to parse request")

			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		if err := validateVerifyCredRequestV2(valid, req); err != nil {
			lg.Warn().Err(err).Msg("failed to validate request")

			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		aerr := svc.verifyCredential(ctx, req, w)
		if aerr != nil {
			lg.Err(aerr).Msg("failed to verify credential")
		}

		return aerr
	}
}

func handleVerifyCredV1(svc *Service, valid *validator.Validate) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()

		lg := logging.Logger(ctx, "skus").With().Str("func", "handleVerifyCredV1").Logger()

		data, err := io.ReadAll(io.LimitReader(r.Body, reqBodyLimit10MB))
		if err != nil {
			lg.Warn().Err(err).Msg("failed to read body")

			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		req := &model.VerifyCredentialRequestV1{}
		if err := json.Unmarshal(data, req); err != nil {
			lg.Warn().Err(err).Msg("failed to parse request")

			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		if err := valid.StructCtx(ctx, req); err != nil {
			lg.Warn().Err(err).Msg("failed to validate request")

			return handlers.WrapError(err, "Error in request validation", http.StatusBadRequest)
		}

		aerr := svc.verifyCredential(ctx, req, w)
		if aerr != nil {
			lg.Err(aerr).Msg("failed to verify credential")
		}

		return aerr
	}
}

func WebhookRouter(svc *Service) chi.Router {
	r := chi.NewRouter()

	r.Method(http.MethodPost, "/stripe", middleware.InstrumentHandler("HandleStripeWebhook", handleStripeWebhook(svc)))
	r.Method(http.MethodPost, "/radom", middleware.InstrumentHandler("handleRadomWebhook", handleRadomWebhook(svc)))
	r.Method(http.MethodPost, "/android", middleware.InstrumentHandler("handleWebhookPlayStore", handleWebhookPlayStore(svc)))
	r.Method(http.MethodPost, "/ios", middleware.InstrumentHandler("handleWebhookAppStore", handleWebhookAppStore(svc)))

	return r
}

func handleWebhookPlayStore(svc *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		return handleWebhookPlayStoreH(w, r, svc)
	}
}

func handleWebhookPlayStoreH(w http.ResponseWriter, r *http.Request, svc *Service) *handlers.AppError {
	ctx := r.Context()

	lg := logging.Logger(ctx, "skus").With().Str("func", "handleWebhookPlayStore").Logger()

	if err := svc.gpsAuth.authenticate(ctx, r.Header.Get("Authorization")); err != nil {
		if errors.Is(err, errGPSDisabled) {
			lg.Warn().Msg("play store notifications disabled")

			return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
		}

		lg.Err(err).Msg("invalid request")

		return handlers.WrapError(err, "invalid request", http.StatusUnauthorized)
	}

	data, err := io.ReadAll(io.LimitReader(r.Body, reqBodyLimit10MB))
	if err != nil {
		lg.Err(err).Msg("failed to read payload")

		return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
	}

	ntf, err := parsePlayStoreDevNotification(data)
	if err != nil {
		lg.Err(err).Str("payload", string(data)).Msg("failed to parse play store notification")

		return handlers.ValidationError("request", map[string]interface{}{"parse-payload": err.Error()})
	}

	if err := svc.processPlayStoreNotification(ctx, ntf); err != nil {
		l := lg.With().Str("ntf_type", ntf.ntfType()).Int("ntf_subtype", ntf.ntfSubType()).Str("ntf_effect", ntf.effect()).Str("ntf_package", ntf.pkg()).Logger()

		switch {
		case errors.Is(err, context.Canceled):
			l.Warn().Err(err).Msg("failed to process play store notification")

			// Should retry.
			return handlers.WrapError(model.ErrSomethingWentWrong, "request has been cancelled", model.StatusClientClosedConn)

		case errors.Is(err, model.ErrOrderNotFound), errors.Is(err, errNotFound):
			l.Warn().Err(err).Msg("failed to process play store notification")

			// Order was not found, so nothing can be done.
			// It might be an issue for VPN:
			// - user has not linked yet;
			// - billing cycle comes through, subscription renews;
			// - user links immediately after billing cycle (they get 1 month);
			// - there might be a small gap between order's expiration and next successful renewal;
			// - the grace period should handle it.
			// A better option is to create orders when the user subscribes, similar to Leo.
			// Or allow for 1 or 2 retry attempts for the notification, but this requires tracking.

			return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)

		case errors.Is(err, model.ErrNoRowsChangedOrder):
			l.Warn().Err(err).Msg("failed to process play store notification")

			// No rows have changed whilst processing.
			// This could happen in theory, but not in practice.
			// It would mean that we attempted to update with the same data as it's in the database.
			// This could happen when trying to process the same event twice, which could happen
			// if the App Store sends multiple notifications about the same event.
			// (E.g. auto-renew and billing recovery).

			return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)

		default:
			l.Err(err).Msg("failed to process play store notification")

			// Retry for all other errors for now.
			return handlers.WrapError(model.ErrSomethingWentWrong, "something went wrong", http.StatusInternalServerError)
		}
	}

	msg := "skipped play store notification"
	if ntf.shouldProcess() {
		msg = "processed play store notification"
	}

	lg.Info().Str("ntf_type", ntf.ntfType()).Int("ntf_subtype", ntf.ntfSubType()).Str("ntf_effect", ntf.effect()).Str("ntf_package", ntf.pkg()).Msg(msg)

	return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
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
		lg.Err(err).Msg("failed to read request body")

		return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
	}

	spayload := &struct {
		SignedPayload string `json:"signedPayload"`
	}{}

	if err := json.Unmarshal(data, spayload); err != nil {
		lg.Err(err).Str("data", string(data)).Msg("failed to unmarshal responseBodyV2")

		return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
	}

	ntf, err := parseAppStoreSrvNotification(svc.assnCertVrf, spayload.SignedPayload)
	if err != nil {
		lg.Err(err).Str("payload", spayload.SignedPayload).Msg("failed to parse app store notification")

		return handlers.ValidationError("request", map[string]interface{}{"parse-signed-payload": err.Error()})
	}

	if err := svc.processAppStoreNotification(ctx, ntf); err != nil {
		l := lg.With().Str("ntf_type", ntf.ntfType()).Str("ntf_subtype", ntf.ntfSubType()).Str("ntf_effect", ntf.effect()).Str("ntf_package", ntf.pkg()).Logger()

		switch {
		case errors.Is(err, context.Canceled):
			l.Warn().Err(err).Msg("failed to process app store notification")

			// Should retry.
			return handlers.WrapError(model.ErrSomethingWentWrong, "request has been cancelled", model.StatusClientClosedConn)

		case errors.Is(err, model.ErrOrderNotFound):
			l.Warn().Err(err).Msg("failed to process app store notification")

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

		case errors.Is(err, model.ErrNoRowsChangedOrder):
			l.Warn().Err(err).Msg("failed to process app store notification")

			// No rows have changed whilst processing.
			// This could happen in theory, but not in practice.
			// It would mean that we attempted to update with the same data as it's in the database.
			// This could happen when trying to process the same event twice, which could happen
			// if the App Store sends multiple notifications about the same event.
			// (E.g. auto-renew and billing recovery).

			return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)

		default:
			l.Err(err).Msg("failed to process app store notification")

			// Retry for all other errors for now.
			return handlers.WrapError(model.ErrSomethingWentWrong, "something went wrong", http.StatusInternalServerError)
		}
	}

	msg := "skipped app store notification"
	if ntf.shouldProcess() {
		msg = "processed app store notification"
	}

	lg.Info().Str("ntf_type", ntf.ntfType()).Str("ntf_subtype", ntf.ntfSubType()).Str("ntf_effect", ntf.effect()).Str("ntf_package", ntf.pkg()).Msg(msg)

	return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
}

// handleRadomWebhook handles Radom checkout session webhooks.
func handleRadomWebhook(s *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		return handleRadomWebhookH(w, r, s)
	}
}

func handleRadomWebhookH(w http.ResponseWriter, r *http.Request, svc *Service) *handlers.AppError {
	ctx := r.Context()

	l := logging.Logger(ctx, "skus").With().Str("func", "handleRadomWebhookH").Logger()

	if err := svc.radomAuth.Authenticate(ctx, r.Header.Get("radom-verification-key")); err != nil {
		l.Err(err).Msg("invalid request")

		return handlers.WrapError(err, "invalid request", http.StatusUnauthorized)
	}

	b, err := io.ReadAll(io.LimitReader(r.Body, reqBodyLimit10MB))
	if err != nil {
		l.Err(err).Msg("failed to read payload")

		return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
	}

	ntf, err := radom.ParseNotification(b)
	if err != nil {
		l.Err(err).Msg("failed to parse radom event")

		return handlers.WrapError(err, "failed to parse radom event", http.StatusBadRequest)
	}

	if err := svc.processRadomNotification(ctx, ntf); err != nil {
		l.Err(err).Msg("failed to process radom notification")

		return handlers.WrapError(model.ErrSomethingWentWrong, "something went wrong", http.StatusInternalServerError)
	}

	msg := "skipped radom notification"
	if ntf.ShouldProcess() {
		msg = "processed radom notification"
	}

	l.Info().Str("ntf_type", ntf.NtfType()).Str("ntf_effect", ntf.Effect()).Msg(msg)

	return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
}

func handleStripeWebhook(svc *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()

		lg := logging.Logger(ctx, "skus").With().Str("func", "handleStripeWebhook").Logger()

		secret, err := appctx.GetStringFromContext(ctx, appctx.StripeWebhookSecretCTXKey)
		if err != nil {
			lg.Err(err).Msg("failed to get stripe_webhook_secret from context")

			return handlers.WrapError(err, "error getting stripe_webhook_secret from context", http.StatusInternalServerError)
		}

		data, err := io.ReadAll(io.LimitReader(r.Body, reqBodyLimit10MB))
		if err != nil {
			lg.Err(err).Msg("failed to read request body")

			// Nothing can be done if failed to read the body.
			return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
		}

		event, err := webhook.ConstructEvent(data, r.Header.Get("Stripe-Signature"), secret)
		if err != nil {
			lg.Err(err).Msg("failed to verify stripe signature")

			return handlers.WrapError(err, "error verifying webhook signature", http.StatusBadRequest)
		}

		ntf, err := parseStripeNotification(&event)
		if err != nil {
			if errors.Is(err, errStripeSkipEvent) {
				return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
			}

			// Unlikely to be able to do anything about it.
			// Consider responding with http.StatusOK.

			return handlers.WrapError(err, "failed to parse event", http.StatusBadRequest)
		}

		if err := svc.processStripeNotification(ctx, ntf); err != nil {
			l := lg.With().Str("ntf_type", ntf.ntfType()).Str("ntf_subtype", ntf.ntfSubType()).Str("ntf_effect", ntf.effect()).Logger()

			switch {
			case errors.Is(err, context.Canceled):
				l.Warn().Err(err).Msg("failed to process stripe notification")

				// Should retry.
				return handlers.WrapError(model.ErrSomethingWentWrong, "request has been cancelled", model.StatusClientClosedConn)

			case errors.Is(err, model.ErrOrderNotFound):
				l.Warn().Err(err).Msg("failed to process stripe notification")

				return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)

			case errors.Is(err, model.ErrNoRowsChangedOrder):
				l.Warn().Err(err).Msg("failed to process stripe notification")

				// No rows have changed whilst processing.
				// This could happen in theory, but not in practice.
				// It would mean that we attempted to update with the same data as it's in the database.

				return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)

			default:
				l.Err(err).Msg("failed to process stripe notification")

				// Retry for all other errors for now.
				return handlers.WrapError(model.ErrSomethingWentWrong, "something went wrong", http.StatusInternalServerError)
			}
		}

		msg := "skipped stripe notification"
		if ntf.shouldProcess() {
			msg = "processed stripe notification"
		}

		lg.Info().Str("ntf_type", ntf.ntfType()).Str("ntf_subtype", ntf.ntfSubType()).Str("ntf_effect", ntf.effect()).Msg(msg)

		return handlers.RenderContent(ctx, struct{}{}, w, http.StatusOK)
	}
}

// handleSubmitReceipt was used for linking IAP subscriptions.
//
// Deprecated: This endpoint is deprecated, and will be shut down soon.
// It received 0 requests in June 2024.
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

		req, err := parseSubmitReceiptRequest(payload)
		if err != nil {
			l.Warn().Err(err).Msg("failed to parse request")

			return handlers.ValidationError("request", map[string]interface{}{"request-body": err.Error()})
		}

		if err := valid.StructCtx(ctx, &req); err != nil {
			verrs, ok := collectValidationErrors(err)
			if !ok {
				return handlers.ValidationError("request", map[string]interface{}{"request-body": err.Error()})
			}

			return handlers.ValidationError("request", verrs)
		}

		rcpt, err := svc.processSubmitReceipt(ctx, req, orderID)
		if err != nil {
			// Found an existing order.
			if errors.Is(err, model.ErrReceiptAlreadyLinked) {
				return handlers.WrapError(err, "receipt has already been submitted", http.StatusConflict)
			}

			if errors.Is(err, model.ErrNoMatchOrderReceipt) {
				return handlers.WrapError(err, "order_id does not match receipt order", http.StatusConflict)
			}

			// Use new so that the shorter IF and narrow scope are possible (via if := ...; {}).
			// It's an example of one of the few legit uses for 'new'.
			if rverr := new(receiptValidError); errors.As(err, &rverr) {
				l.Warn().Err(err).Msg("failed to validate receipt with vendor")

				return handleReceiptErr(rverr.err)
			}

			l.Warn().Err(err).Msg("failed to create order")

			return handlers.WrapError(err, "failed to process submit receipt request", http.StatusInternalServerError)
		}

		result := &struct {
			ExternalID string `json:"externalId"`
			Vendor     string `json:"vendor"`
		}{ExternalID: rcpt.ExtID, Vendor: rcpt.Type.String()}

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
		lg.Warn().Err(err).Msg("failed to parse request")

		return handlers.ValidationError("request", map[string]interface{}{"request-body": err.Error()})
	}

	if err := valid.StructCtx(ctx, &req); err != nil {
		verrs, ok := collectValidationErrors(err)
		if !ok {
			return handlers.ValidationError("request", map[string]interface{}{"request-body": err.Error()})
		}

		return handlers.ValidationError("request", verrs)
	}

	ord, err := svc.createOrderWithReceipt(ctx, req)
	if err != nil {
		// Found an existing order, respond with the id (ord guaranteed not to be nil).
		if errors.Is(err, model.ErrOrderExistsForReceipt) {
			result := model.CreateOrderWithReceiptResponse{ID: ord.ID.String()}

			return handlers.RenderContent(ctx, result, w, http.StatusConflict)
		}

		// Use new so that the shorter IF and narrow scope are possible (via if := ...; {}).
		// It's an example of one of the few legit uses for 'new'.
		if rverr := new(receiptValidError); errors.As(err, &rverr) {
			lg.Warn().Err(err).Msg("failed to validate receipt with vendor")

			return handleReceiptErr(rverr.err)
		}

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
		lg.Warn().Err(err).Msg("failed to parse request")

		return handlers.ValidationError("request", map[string]interface{}{"request-body": err.Error()})
	}

	if err := valid.StructCtx(ctx, &req); err != nil {
		verrs, ok := collectValidationErrors(err)
		if !ok {
			return handlers.ValidationError("request", map[string]interface{}{"request-body": err.Error()})
		}

		return handlers.ValidationError("request", verrs)
	}

	if err := svc.checkOrderReceipt(ctx, req, orderID); err != nil {
		// Use new so that the shorter IF and narrow scope are possible (via if := ...; {}).
		// It's an example of one of the few legit uses for 'new'.
		if rverr := new(receiptValidError); errors.As(err, &rverr) {
			lg.Warn().Err(err).Msg("failed to validate receipt with vendor")

			return handleReceiptErr(rverr.err)
		}

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
	case errors.Is(err, errIOSPurchaseNotFound):
		result.ErrorCode = "purchase_not_found"

	case errors.Is(err, errGPSSubPurchaseExpired):
		result.ErrorCode = "purchase_expired"

	case errors.Is(err, errGPSSubPurchasePending):
		result.ErrorCode = "purchase_pending"

	default:
		result.ErrorCode = "validation_failed"
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

func parseVerifyCredRequestV2(raw []byte) (*model.VerifyCredentialRequestV2, error) {
	result := &model.VerifyCredentialRequestV2{}

	if err := json.Unmarshal(raw, result); err != nil {
		return nil, err
	}

	copaque, err := parseVerifyCredOpaque(result.Credential)
	if err != nil {
		return nil, err
	}

	result.CredentialOpaque = copaque

	return result, nil
}

func parseVerifyCredOpaque(raw string) (*model.VerifyCredentialOpaque, error) {
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}

	result := &model.VerifyCredentialOpaque{}
	if err = json.Unmarshal(data, result); err != nil {
		return nil, err
	}

	return result, nil
}

func validateVerifyCredRequestV2(valid *validator.Validate, req *model.VerifyCredentialRequestV2) error {
	if err := valid.Struct(req); err != nil {
		return err
	}

	return valid.Struct(req.CredentialOpaque)
}
