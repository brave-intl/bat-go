package payment

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/utils/datastore"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v71"
	"github.com/stripe/stripe-go/v71/checkout/session"
	"github.com/stripe/stripe-go/v71/customer"
	macaroon "gopkg.in/macaroon.v2"
)

// Order includes information about a particular order
type Order struct {
	ID         uuid.UUID            `json:"id" db:"id"`
	CreatedAt  time.Time            `json:"createdAt" db:"created_at"`
	Currency   string               `json:"currency" db:"currency"`
	UpdatedAt  time.Time            `json:"updatedAt" db:"updated_at"`
	TotalPrice decimal.Decimal      `json:"totalPrice" db:"total_price"`
	MerchantID string               `json:"-" db:"merchant_id"`
	Location   datastore.NullString `json:"location" db:"location"`
	Status     string               `json:"status" db:"status"`
	Items      []OrderItem          `json:"items"`
}

// OrderItem includes information about a particular order item
type OrderItem struct {
	ID          uuid.UUID            `json:"id" db:"id"`
	OrderID     uuid.UUID            `json:"orderId" db:"order_id"`
	SKU         string               `json:"sku" db:"sku"`
	CreatedAt   *time.Time           `json:"createdAt" db:"created_at"`
	UpdatedAt   *time.Time           `json:"updatedAt" db:"updated_at"`
	Currency    string               `json:"currency" db:"currency"`
	Quantity    int                  `json:"quantity" db:"quantity"`
	Price       decimal.Decimal      `json:"price" db:"price"`
	Subtotal    decimal.Decimal      `json:"subtotal" db:"subtotal"`
	Location    datastore.NullString `json:"location" db:"location"`
	Description datastore.NullString `json:"description" db:"description"`
}

const (
	PROD_USER_WALLET_VOTE       = "AgEJYnJhdmUuY29tAiNicmF2ZSB1c2VyLXdhbGxldC12b3RlIHNrdSB0b2tlbiB2MQACFHNrdT11c2VyLXdhbGxldC12b3RlAAIKcHJpY2U9MC4yNQACDGN1cnJlbmN5PUJBVAACDGRlc2NyaXB0aW9uPQACGmNyZWRlbnRpYWxfdHlwZT1zaW5nbGUtdXNlAAAGIOaNAUCBMKm0IaLqxefhvxOtAKB0OfoiPn0NPVfI602J"
	PROD_ANON_CARD_VOTE         = "AgEJYnJhdmUuY29tAiFicmF2ZSBhbm9uLWNhcmQtdm90ZSBza3UgdG9rZW4gdjEAAhJza3U9YW5vbi1jYXJkLXZvdGUAAgpwcmljZT0wLjI1AAIMY3VycmVuY3k9QkFUAAIMZGVzY3JpcHRpb249AAIaY3JlZGVudGlhbF90eXBlPXNpbmdsZS11c2UAAAYgrMZm85YYwnmjPXcegy5pBM5C+ZLfrySZfYiSe13yp8o="
	PROD_BRAVE_TOGETHER_FREE    = "MDAxN2xvY2F0aW9uIGJyYXZlLmNvbQowMDJkaWRlbnRpZmllciBicmF2ZSBmcmVlLXRyaWFsIHNrdSB0b2tlbiB2MQowMDE3Y2lkIHNrdT1mcmVlLXRyaWFsCjAwMTBjaWQgcHJpY2U9MAowMDE1Y2lkIGN1cnJlbmN5PUJBVAowMDM0Y2lkIGRlc2NyaXB0aW9uPUdyYW50cyByZWNpcGllbnQgb25lIGZyZWUgdHJpYWwKMDAyM2NpZCBjcmVkZW50aWFsX3R5cGU9c2luZ2xlLXVzZQowMDJmc2lnbmF0dXJlILeuqgF6G9nPczv/CLyEtAQB/evX8RGFqXAxjga4++3HCg=="
	PROD_BRAVE_TOGETHER_PAID    = "MDAxN2xvY2F0aW9uIGJyYXZlLmNvbQowMDM2aWRlbnRpZmllciBicmF2ZSBicmF2ZS10b2dldGhlci1wYWlkIHNrdSB0b2tlbiB2MQowMDIwY2lkIHNrdT1icmF2ZS10b2dldGhlci1wYWlkCjAwMTBjaWQgcHJpY2U9NQowMDE1Y2lkIGN1cnJlbmN5PVVTRAowMDM5Y2lkIGRlc2NyaXB0aW9uPXBhaWQgc3Vic2NyaXB0aW9uIGZvciBicmF2ZSB0b2dldGhlcgowMDIzY2lkIGNyZWRlbnRpYWxfdHlwZT1zaW5nbGUtdXNlCjAwMmZzaWduYXR1cmUgod5rH3D4XQsvXHz65EZGLGv7HQFNtN4SJBzaWdAocdkK"
	STAGING_USER_WALLET_VOTE    = "AgEJYnJhdmUuY29tAiNicmF2ZSB1c2VyLXdhbGxldC12b3RlIHNrdSB0b2tlbiB2MQACFHNrdT11c2VyLXdhbGxldC12b3RlAAIKcHJpY2U9MC4yNQACDGN1cnJlbmN5PUJBVAACDGRlc2NyaXB0aW9uPQACGmNyZWRlbnRpYWxfdHlwZT1zaW5nbGUtdXNlAAAGIOH4Li+rduCtFOfV8Lfa2o8h4SQjN5CuIwxmeQFjOk4W"
	STAGING_ANON_CARD_VOTE      = "AgEJYnJhdmUuY29tAiFicmF2ZSBhbm9uLWNhcmQtdm90ZSBza3UgdG9rZW4gdjEAAhJza3U9YW5vbi1jYXJkLXZvdGUAAgpwcmljZT0wLjI1AAIMY3VycmVuY3k9QkFUAAIMZGVzY3JpcHRpb249AAIaY3JlZGVudGlhbF90eXBlPXNpbmdsZS11c2UAAAYgPV/WYY5pXhodMPvsilnrLzNH6MA8nFXwyg0qSWX477M="
	STAGING_BRAVE_TOGETHER_FREE = "MDAxN2xvY2F0aW9uIGJyYXZlLmNvbQowMDJkaWRlbnRpZmllciBicmF2ZSBmcmVlLXRyaWFsIHNrdSB0b2tlbiB2MQowMDE3Y2lkIHNrdT1mcmVlLXRyaWFsCjAwMTBjaWQgcHJpY2U9MAowMDE1Y2lkIGN1cnJlbmN5PUJBVAowMDM0Y2lkIGRlc2NyaXB0aW9uPUdyYW50cyByZWNpcGllbnQgb25lIGZyZWUgdHJpYWwKMDAyM2NpZCBjcmVkZW50aWFsX3R5cGU9c2luZ2xlLXVzZQowMDJmc2lnbmF0dXJlIGfeOulgTyOWVP1Qiszt8lfPnppPJQhoi8xTfI6bzqO4Cg=="
	STAGING_BRAVE_TOGETHER_PAID = "MDAxN2xvY2F0aW9uIGJyYXZlLmNvbQowMDM2aWRlbnRpZmllciBicmF2ZSBicmF2ZS10b2dldGhlci1wYWlkIHNrdSB0b2tlbiB2MQowMDIwY2lkIHNrdT1icmF2ZS10b2dldGhlci1wYWlkCjAwMTBjaWQgcHJpY2U9NQowMDE1Y2lkIGN1cnJlbmN5PVVTRAowMDM5Y2lkIGRlc2NyaXB0aW9uPXBhaWQgc3Vic2NyaXB0aW9uIGZvciBicmF2ZSB0b2dldGhlcgowMDIzY2lkIGNyZWRlbnRpYWxfdHlwZT1zaW5nbGUtdXNlCjAwMmZzaWduYXR1cmUgks5MSsT0v9cpb0MamU5blzHb+CxRO3WYAENbSkZ3bDAK"
	DEV_USER_WALLET_VOTE        = "AgEJYnJhdmUuY29tAiNicmF2ZSB1c2VyLXdhbGxldC12b3RlIHNrdSB0b2tlbiB2MQACFHNrdT11c2VyLXdhbGxldC12b3RlAAIKcHJpY2U9MC4yNQACDGN1cnJlbmN5PUJBVAACDGRlc2NyaXB0aW9uPQACGmNyZWRlbnRpYWxfdHlwZT1zaW5nbGUtdXNlAAAGINiB9dUmpqLyeSEdZ23E4dPXwIBOUNJCFN9d5toIME2M"
	DEV_ANON_CARD_VOTE          = "AgEJYnJhdmUuY29tAiFicmF2ZSBhbm9uLWNhcmQtdm90ZSBza3UgdG9rZW4gdjEAAhJza3U9YW5vbi1jYXJkLXZvdGUAAgpwcmljZT0wLjI1AAIMY3VycmVuY3k9QkFUAAIMZGVzY3JpcHRpb249AAIaY3JlZGVudGlhbF90eXBlPXNpbmdsZS11c2UAAAYgPpv+Al9jRgVCaR49/AoRrsjQqXGqkwaNfqVka00SJxQ="
	DEV_BRAVE_TOGETHER_FREE     = "MDAxN2xvY2F0aW9uIGJyYXZlLmNvbQowMDJkaWRlbnRpZmllciBicmF2ZSBmcmVlLXRyaWFsIHNrdSB0b2tlbiB2MQowMDE3Y2lkIHNrdT1mcmVlLXRyaWFsCjAwMTBjaWQgcHJpY2U9MAowMDE1Y2lkIGN1cnJlbmN5PUJBVAowMDM0Y2lkIGRlc2NyaXB0aW9uPUdyYW50cyByZWNpcGllbnQgb25lIGZyZWUgdHJpYWwKMDAyM2NpZCBjcmVkZW50aWFsX3R5cGU9c2luZ2xlLXVzZQowMDJmc2lnbmF0dXJlIAs+/paWWm0Kxm/do/8bPGga5ETPVRx1w6J8SPq0mzBFCg=="
	DEV_BRAVE_TOGETHER_PAID     = "MDAxN2xvY2F0aW9uIGJyYXZlLmNvbQowMDM2aWRlbnRpZmllciBicmF2ZSBicmF2ZS10b2dldGhlci1wYWlkIHNrdSB0b2tlbiB2MQowMDIwY2lkIHNrdT1icmF2ZS10b2dldGhlci1wYWlkCjAwMTBjaWQgcHJpY2U9NQowMDE1Y2lkIGN1cnJlbmN5PVVTRAowMDM5Y2lkIGRlc2NyaXB0aW9uPXBhaWQgc3Vic2NyaXB0aW9uIGZvciBicmF2ZSB0b2dldGhlcgowMDIzY2lkIGNyZWRlbnRpYWxfdHlwZT1zaW5nbGUtdXNlCjAwMmZzaWduYXR1cmUgpkP6KyaaKg6UmrqtbuRR16xtQ3TyQcDY6G33GmNLWAsK"
)

// IsValidSKU checks to see if the token provided is one that we've previously created
func IsValidSKU(sku string) bool {
	env := os.Getenv("ENV")
	if env == "production" {
		switch sku {
		case
			PROD_USER_WALLET_VOTE,
			PROD_ANON_CARD_VOTE,
			PROD_BRAVE_TOGETHER_FREE,
			PROD_BRAVE_TOGETHER_PAID:
			return true
		}
	} else {
		switch sku {
		case
			STAGING_USER_WALLET_VOTE,
			STAGING_ANON_CARD_VOTE,
			STAGING_BRAVE_TOGETHER_FREE,
			STAGING_BRAVE_TOGETHER_PAID,
			DEV_USER_WALLET_VOTE,
			DEV_ANON_CARD_VOTE,
			DEV_BRAVE_TOGETHER_FREE,
			DEV_BRAVE_TOGETHER_PAID:
			return true
		}
	}

	return false
}

// CreateOrderItemFromMacaroon creates an order item from a macaroon
func CreateOrderItemFromMacaroon(sku string, quantity int) (*OrderItem, error) {
	macBytes, err := macaroon.Base64Decode([]byte(sku))
	if err != nil {
		return nil, err
	}
	mac := &macaroon.Macaroon{}
	err = mac.UnmarshalBinary(macBytes)
	if err != nil {
		return nil, err
	}

	// TODO Figure out how to verify macaroon using library
	// I think we have to call .Bind()
	// https://github.com/go-macaroon/macaroon#func-macaroon-bind
	// I think we simply want to verify the signature and not the caveats?
	// SO maybe VerifySignature
	// https://github.com/go-macaroon/macaroon#func-macaroon-verifysignature

	caveats := mac.Caveats()
	orderItem := OrderItem{}
	orderItem.Quantity = quantity
	orderItem.Location.String = mac.Location()
	orderItem.Location.Valid = true

	for i := 0; i < len(caveats); i++ {
		caveat := mac.Caveats()[i]
		values := strings.Split(string(caveat.Id), "=")
		key := strings.TrimSpace(values[0])
		value := strings.TrimSpace(values[1])

		switch key {
		case "sku":
			orderItem.SKU = value
		case "price", "amount":
			orderItem.Price, err = decimal.NewFromString(value)
			if err != nil {
				return nil, err
			}
		case "description":
			orderItem.Description.String = value
			orderItem.Description.Valid = true
			if err != nil {
				return nil, err
			}
		case "currency":
			orderItem.Currency = value
		}

	}
	newQuantity, err := decimal.NewFromString(strconv.Itoa(orderItem.Quantity))
	if err != nil {
		return nil, err
	}

	orderItem.Subtotal = orderItem.Price.Mul(newQuantity)

	return &orderItem, nil
}

// IsPaid returns true if the order is paid
func (order Order) IsPaid() bool {
	return order.Status == "paid"
}

// IsStripePayable returns true if every item is payable by Stripe
func (order Order) IsStripePayable() bool {
	for _, item := range order.Items {
		if item.SKU != "brave-together-paid" {
			return false
		}
	}
	return true
}

type CreateCheckoutSessionResponse struct {
	SessionID string `json:"checkoutSessionId"`
}

// Create a Stripe Checkout Session for an Order
func (order Order) CreateCheckoutSession() CreateCheckoutSessionResponse {
	stripe.Key = "sk_test_51HlmudHof20bphG6m8eJi9BvbPMLkMX4HPqLIiHmjdKAX21oJeO3S6izMrYTmiJm3NORBzUK1oM8STqClDRT3xQ700vyUyabNo"

	// Create Stripe customer if one doesn't already exist
	// TODO - Email should be stored on Order at creation time.
	customerParams := &stripe.CustomerParams{
		Email: stripe.String("danlipeles@icloud.com"),
	}
	customer, _ := customer.New(customerParams)

	metadata := make(map[string]string)
	metadata["orderID"] = order.ID.String()

	// TODO - Match SKUs to Stripe Price Objects
	params := &stripe.CheckoutSessionParams{
		Customer: stripe.String(customer.ID),
		PaymentMethodTypes: stripe.StringSlice([]string{
			"card",
		}),
		Mode:              stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		SuccessURL:        stripe.String("https://example.com/success"),
		CancelURL:         stripe.String("https://example.com/cancel"),
		ClientReferenceID: stripe.String(order.ID.String()),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			&stripe.CheckoutSessionLineItemParams{
				Price:    stripe.String("price_1Hpg8nHof20bphG6X4eQ6Dit"),
				Quantity: stripe.Int64(1),
			},
		},
		SubscriptionData: &stripe.CheckoutSessionSubscriptionDataParams{},
	}

	params.SubscriptionData.AddMetadata("orderID", order.ID.String())

	session, _ := session.New(params)

	data := CreateCheckoutSessionResponse{
		SessionID: session.ID,
	}
	return data
}
