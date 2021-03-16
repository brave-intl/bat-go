package payment

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/middleware"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/cryptography"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/brave-intl/bat-go/utils/outputs"
	"github.com/brave-intl/bat-go/utils/requestutils"
	"github.com/go-chi/chi"
	uuid "github.com/satori/go.uuid"
	stripe "github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/client"
	"github.com/stripe/stripe-go/webhook"
)

// WebhookRouter handles calls relating to payments
func WebhookRouter(service *Service) chi.Router {
	r := chi.NewRouter()
	r.Method("POST", "/stripe", middleware.InstrumentHandler("HandleStripeWebhook", HandleStripeWebhook(service)))
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

		// Validates the SKU is one of our previously created SKUs
		for _, item := range req.Items {
			if !IsValidSKU(item.SKU) {
				return handlers.WrapError(err, "Invalid SKU Token provided in request", http.StatusBadRequest)
			}
		}

		order, err := service.CreateOrderFromRequest(req)

		if err != nil {
			return handlers.WrapError(err, "Error creating the order in the database", http.StatusInternalServerError)
		}

		for i, item := range order.Items {
			// FIXME
			if item.SKU == "brave-together-free" || item.SKU == "brave-together-paid" {
				order.Items[i].Type = "time-limited"
			} else {
				order.Items[i].Type = "single-use"
			}
		}

		return handlers.RenderContent(r.Context(), order, w, http.StatusCreated)
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

		order, err := service.Datastore.GetOrder(*orderID.UUID())
		if err != nil {
			return handlers.WrapError(err, "Error retrieving the order", http.StatusInternalServerError)
		}

		status := http.StatusOK
		if order == nil {
			status = http.StatusNotFound
		}

		// FIXME
		for i, item := range order.Items {
			if item.SKU == "brave-together-free" || item.SKU == "brave-together-paid" {
				order.Items[i].Type = "time-limited"
			} else {
				order.Items[i].Type = "single-use"
			}
		}

		if order != nil && !order.IsPaid() && order.IsStripePayable() {
			type OrderWithStripeCheckoutSessionID struct {
				*Order
				StripeCheckoutSessionID string `json:"stripeCheckoutSessionId"`
			}

			s, err := service.Datastore.GetOrderMetadata(*orderID.UUID(), "stripeCheckoutSessionId")
			stripeCheckoutSessionID := s[1 : len(s)-1]

			if err != nil {
				return handlers.WrapError(err, "Error retrieving stripeCheckoutSessionId", http.StatusInternalServerError)
			}

			orderWithStripeCheckoutSessionID := OrderWithStripeCheckoutSessionID{
				Order: order,
				StripeCheckoutSessionID: stripeCheckoutSessionID,
			}

			return handlers.RenderContent(r.Context(), orderWithStripeCheckoutSessionID, w, http.StatusOK)
		}

		return handlers.RenderContent(r.Context(), order, w, status)
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

		stripeSubscriptionID, _ := service.Datastore.GetOrderMetadata(*orderID.UUID(), "stripeSubscriptionId")

		if stripeSubscriptionID != "" {
			stripeSubscriptionID = stripeSubscriptionID[1 : len(stripeSubscriptionID)-1]
			sc := &client.API{}
			// os.Getenv("STRIPE_SECRET")
			sc.Init("sk_test_51HlmudHof20bphG6m8eJi9BvbPMLkMX4HPqLIiHmjdKAX21oJeO3S6izMrYTmiJm3NORBzUK1oM8STqClDRT3xQ700vyUyabNo", nil)
			subscription, err := sc.Subscriptions.Update(stripeSubscriptionID, &stripe.SubscriptionParams{
				CancelAtPeriodEnd: stripe.Bool(true),
			})
			if err != nil {
				return handlers.WrapError(err, "error canceling subscription", http.StatusInternalServerError)
			}
			return handlers.RenderContent(r.Context(), "Subscription "+subscription.ID+" successfully canceled", w, http.StatusOK)
		}

		err := service.Datastore.UpdateOrder(*orderID.UUID(), "canceled")
		if err != nil {
			return handlers.WrapError(err, "error canceling order", http.StatusInternalServerError)
		}

		return handlers.RenderContent(r.Context(), "Order successfully canceled", w, http.StatusOK)
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
			err = fmt.Errorf("External Transaction ID: %s has already been added to the order", req.ExternalTransactionID.String())
			return handlers.WrapError(err, "Error creating the transaction", http.StatusBadRequest)
		}

		transaction, err = service.CreateTransactionFromRequest(req, *orderID.UUID())
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
// FIXME - Allow returning both limited and one-time use credentials
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

		order, err := service.Datastore.GetOrder(*orderID.UUID())

		var credentials []TimeLimitedCreds
		timeLimitedSecret := cryptography.NewTimeLimitedSecret([]byte(os.Getenv("BRAVE_MERCHANT_KEY")))
		// FIXME - We should scope this to subscription paid date in case of paid order
		issuedAt := order.CreatedAt
		expiresAt := issuedAt.AddDate(0, 0, 35)
		if order.IsPaid() {
			for _, item := range order.Items {
				if item.SKU == "brave-together-free" || item.SKU == "brave-together-paid" {
					result, err := timeLimitedSecret.Derive(issuedAt, expiresAt)
					if err != nil {
						return handlers.WrapError(err, "Error generating time-limited credential", http.StatusInternalServerError)
					}

					credentials = append(credentials, TimeLimitedCreds{
						ID:        item.ID,
						OrderID:   order.ID,
						IssuedAt:  issuedAt.Format("2006-01-02"),
						ExpiresAt: expiresAt.Format("2006-01-02"),
						Token:     result,
					})
				}
			}

			if len(credentials) > 0 {
				return handlers.RenderContent(r.Context(), credentials, w, http.StatusOK)
			}
		}

		creds, err := service.Datastore.GetOrderCreds(*orderID.UUID(), false)
		if err != nil {
			return handlers.WrapError(err, "Error getting claim", http.StatusBadRequest)
		}

		if creds == nil {
			return &handlers.AppError{
				Message: "Credentials do not exist",
				Code:    http.StatusNotFound,
				Data:    map[string]interface{}{},
			}
		}

		status := http.StatusOK
		for i := 0; i < len(*creds); i++ {
			if (*creds)[i].SignedCreds == nil {
				status = http.StatusAccepted
				break
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

		err := service.Datastore.DeleteOrderCreds(*orderID.UUID(), false)
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
		response := &outputs.PaginationResponse{
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

// HandleStripeWebhook is the handler for stripe checkout session webhooks
func HandleStripeWebhook(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		// os.Getenv("STRIPE_SECRET")
		stripe.Key = "sk_test_51HlmudHof20bphG6m8eJi9BvbPMLkMX4HPqLIiHmjdKAX21oJeO3S6izMrYTmiJm3NORBzUK1oM8STqClDRT3xQ700vyUyabNo"
		// os.Getenv("STRIPE_WEBHOOK_SECRET")
		endpointSecret := "whsec_Nm4yLVIpnG2cW5fW3kHQHxXBAJCL9dUj"

		const MaxBodyBytes = int64(65536)
		r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return handlers.WrapError(err, "error reading request body", http.StatusServiceUnavailable)
		}

		event, err := webhook.ConstructEvent(body, r.Header.Get("Stripe-Signature"), endpointSecret)
		if err != nil {
			return handlers.WrapError(err, "error verifying webhook signature", http.StatusBadRequest)
		}

		// Handle invoice events
		if event.Type == "invoice.updated" {

			// Retrieve invoice from update events
			var invoice stripe.Invoice
			err := json.Unmarshal(event.Data.Raw, &invoice)
			if err != nil {
				return handlers.WrapError(err, "error parsing webhook JSON", http.StatusBadRequest)
			}

			// Get the subscription and orderID connected to this invoice
			sc := &client.API{}
			// os.Getenv("STRIPE_SECRET")
			sc.Init("sk_test_51HlmudHof20bphG6m8eJi9BvbPMLkMX4HPqLIiHmjdKAX21oJeO3S6izMrYTmiJm3NORBzUK1oM8STqClDRT3xQ700vyUyabNo", nil)
			subscription, err := sc.Subscriptions.Get(invoice.Subscription.ID, nil)
			if err != nil {
				return handlers.WrapError(err, "error retrieving subscription", http.StatusInternalServerError)
			}
			orderID, err := uuid.FromString(subscription.Metadata["orderID"])
			if err != nil {
				return handlers.WrapError(err, "error retrieving orderID", http.StatusInternalServerError)
			}

			// If the invoice is paid set order status to paid, otherwise
			if invoice.Paid {
				err = service.Datastore.UpdateOrder(orderID, "paid")
				if err != nil {
					return handlers.WrapError(err, "error updating order status", http.StatusInternalServerError)
				}
				err = service.Datastore.UpdateOrderMetadata(orderID, "stripeSubscriptionId", subscription.ID)
				if err != nil {
					return handlers.WrapError(err, "error updating order metadata", http.StatusInternalServerError)
				}
				return handlers.RenderContent(r.Context(), "payment successful", w, http.StatusOK)
			} else {
				err = service.Datastore.UpdateOrder(orderID, "pending")
				if err != nil {
					return handlers.WrapError(err, "error updating order status", http.StatusInternalServerError)
				}
				err = service.Datastore.UpdateOrderMetadata(orderID, "stripeSubscriptionId", subscription.ID)
				if err != nil {
					return handlers.WrapError(err, "error updating order metadata", http.StatusInternalServerError)
				}
				return handlers.RenderContent(r.Context(), "payment failed", w, http.StatusOK)
			}

		}

		// Handle subscription cancellations
		if event.Type == "customer.subscription.deleted" {
			var subscription stripe.Subscription
			err := json.Unmarshal(event.Data.Raw, &subscription)
			if err != nil {
				return handlers.WrapError(err, "error parsing webhook JSON", http.StatusBadRequest)
			}
			orderID, err := uuid.FromString(subscription.Metadata["orderID"])
			if err != nil {
				return handlers.WrapError(err, "error retrieving orderID", http.StatusInternalServerError)
			}
			err = service.Datastore.UpdateOrder(orderID, "canceled")
			if err != nil {
				return handlers.WrapError(err, "error updating order status", http.StatusInternalServerError)
			}
			return handlers.RenderContent(r.Context(), "subscription canceled", w, http.StatusOK)
		}

		return handlers.RenderContent(r.Context(), "event received", w, http.StatusOK)
	})
}

// VerifyCredentialRequest includes an opaque subscription credential blob
type VerifyCredentialRequest struct {
	Type         string  `json:"type"`
	Version      float64 `json:"version"`
	SKU          string  `json:"sku"`
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

		if req.Type == "time-limited" {

			// Presentation includes a token and token metadata test test
			type Presentation struct {
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

			verified, err := timeLimitedSecret.Verify(issuedAt, expiresAt, presentation.Token)
			if err != nil {
				return handlers.WrapError(err, "Error in token verification", http.StatusBadRequest)
			}

			if verified {
				return handlers.RenderContent(r.Context(), "Credentials successfully verified", w, http.StatusOK)
			}

			return handlers.RenderContent(r.Context(), "Credentials could not be verified", w, http.StatusForbidden)

		}

		// CBP redemptions can be reserved for other credential types

		// err = service.cbClient.RedeemCredential(r.Context(), decodedCredential.Issuer, decodedCredential.TokenPreimage, decodedCredential.Signature, decodedCredential.Issuer)
		// if err != nil {
		// 	return handlers.WrapError(err, "Error verifying credentials", http.StatusInternalServerError)
		// }

		return handlers.WrapError(err, "Unknown credential type", http.StatusBadRequest)
	})
}
