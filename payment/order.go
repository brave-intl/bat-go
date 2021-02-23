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
	Type        string               `json:"type"`
}

const (
	PROD_USER_WALLET_VOTE       = "AgEJYnJhdmUuY29tAiNicmF2ZSB1c2VyLXdhbGxldC12b3RlIHNrdSB0b2tlbiB2MQACFHNrdT11c2VyLXdhbGxldC12b3RlAAIKcHJpY2U9MC4yNQACDGN1cnJlbmN5PUJBVAACDGRlc2NyaXB0aW9uPQACGmNyZWRlbnRpYWxfdHlwZT1zaW5nbGUtdXNlAAAGIOaNAUCBMKm0IaLqxefhvxOtAKB0OfoiPn0NPVfI602J"
	PROD_ANON_CARD_VOTE         = "AgEJYnJhdmUuY29tAiFicmF2ZSBhbm9uLWNhcmQtdm90ZSBza3UgdG9rZW4gdjEAAhJza3U9YW5vbi1jYXJkLXZvdGUAAgpwcmljZT0wLjI1AAIMY3VycmVuY3k9QkFUAAIMZGVzY3JpcHRpb249AAIaY3JlZGVudGlhbF90eXBlPXNpbmdsZS11c2UAAAYgrMZm85YYwnmjPXcegy5pBM5C+ZLfrySZfYiSe13yp8o="
	PROD_BRAVE_TOGETHER_FREE    = "MDAyNWxvY2F0aW9uIHRvZ2V0aGVyLmJyYXZlLnNvZnR3YXJlCjAwMzBpZGVudGlmaWVyIGJyYXZlLXRvZ2V0aGVyLWZyZWUgc2t1IHRva2VuIHYxCjAwMjBjaWQgc2t1PWJyYXZlLXRvZ2V0aGVyLWZyZWUKMDAxMGNpZCBwcmljZT0wCjAwMTVjaWQgY3VycmVuY3k9QkFUCjAwM2NjaWQgZGVzY3JpcHRpb249T25lIG1vbnRoIGZyZWUgdHJpYWwgZm9yIEJyYXZlIFRvZ2V0aGVyCjAwMjVjaWQgY3JlZGVudGlhbF90eXBlPXRpbWUtbGltaXRlZAowMDI2Y2lkIGNyZWRlbnRpYWxfdmFsaWRfZHVyYXRpb249UDFNCjAwMmZzaWduYXR1cmUgEyHMOlzoMiUqfKGY/npECUsLh+p0czZJqiRHWcm67x0K"
	PROD_BRAVE_TOGETHER_PAID    = "MDAyMGxvY2F0aW9uIHRvZ2V0aGVyLmJyYXZlLmNvbQowMDMwaWRlbnRpZmllciBicmF2ZS10b2dldGhlci1wYWlkIHNrdSB0b2tlbiB2MQowMDIwY2lkIHNrdT1icmF2ZS10b2dldGhlci1wYWlkCjAwMTBjaWQgcHJpY2U9NQowMDE1Y2lkIGN1cnJlbmN5PVVTRAowMDQzY2lkIGRlc2NyaXB0aW9uPU9uZSBtb250aCBwYWlkIHN1YnNjcmlwdGlvbiBmb3IgQnJhdmUgVG9nZXRoZXIKMDAyNWNpZCBjcmVkZW50aWFsX3R5cGU9dGltZS1saW1pdGVkCjAwMjZjaWQgY3JlZGVudGlhbF92YWxpZF9kdXJhdGlvbj1QMU0KMDAyZnNpZ25hdHVyZSAl/eGfP93lrklACcFClNPvkP3Go0HCtfYVQMs5n/NJpgo="
	STAGING_USER_WALLET_VOTE    = "AgEJYnJhdmUuY29tAiNicmF2ZSB1c2VyLXdhbGxldC12b3RlIHNrdSB0b2tlbiB2MQACFHNrdT11c2VyLXdhbGxldC12b3RlAAIKcHJpY2U9MC4yNQACDGN1cnJlbmN5PUJBVAACDGRlc2NyaXB0aW9uPQACGmNyZWRlbnRpYWxfdHlwZT1zaW5nbGUtdXNlAAAGIOH4Li+rduCtFOfV8Lfa2o8h4SQjN5CuIwxmeQFjOk4W"
	STAGING_ANON_CARD_VOTE      = "AgEJYnJhdmUuY29tAiFicmF2ZSBhbm9uLWNhcmQtdm90ZSBza3UgdG9rZW4gdjEAAhJza3U9YW5vbi1jYXJkLXZvdGUAAgpwcmljZT0wLjI1AAIMY3VycmVuY3k9QkFUAAIMZGVzY3JpcHRpb249AAIaY3JlZGVudGlhbF90eXBlPXNpbmdsZS11c2UAAAYgPV/WYY5pXhodMPvsilnrLzNH6MA8nFXwyg0qSWX477M="
	STAGING_BRAVE_TOGETHER_FREE = "MDAyOGxvY2F0aW9uIHRvZ2V0aGVyLmJyYXZlc29mdHdhcmUuY29tCjAwMzBpZGVudGlmaWVyIGJyYXZlLXRvZ2V0aGVyLWZyZWUgc2t1IHRva2VuIHYxCjAwMjBjaWQgc2t1PWJyYXZlLXRvZ2V0aGVyLWZyZWUKMDAxMGNpZCBwcmljZT0wCjAwMTVjaWQgY3VycmVuY3k9QkFUCjAwM2NjaWQgZGVzY3JpcHRpb249T25lIG1vbnRoIGZyZWUgdHJpYWwgZm9yIEJyYXZlIFRvZ2V0aGVyCjAwMjVjaWQgY3JlZGVudGlhbF90eXBlPXRpbWUtbGltaXRlZAowMDI2Y2lkIGNyZWRlbnRpYWxfdmFsaWRfZHVyYXRpb249UDFNCjAwMmZzaWduYXR1cmUg3cCMuN3F1wVhDvPmV9kA7JuvAgzedifNj2KzUNMLgMIK"
	STAGING_BRAVE_TOGETHER_PAID = "MDAyNWxvY2F0aW9uIHRvZ2V0aGVyLmJyYXZlLnNvZnR3YXJlCjAwMzBpZGVudGlmaWVyIGJyYXZlLXRvZ2V0aGVyLXBhaWQgc2t1IHRva2VuIHYxCjAwMjBjaWQgc2t1PWJyYXZlLXRvZ2V0aGVyLXBhaWQKMDAxMGNpZCBwcmljZT01CjAwMTVjaWQgY3VycmVuY3k9VVNECjAwNDNjaWQgZGVzY3JpcHRpb249T25lIG1vbnRoIHBhaWQgc3Vic2NyaXB0aW9uIGZvciBCcmF2ZSBUb2dldGhlcgowMDI1Y2lkIGNyZWRlbnRpYWxfdHlwZT10aW1lLWxpbWl0ZWQKMDAyNmNpZCBjcmVkZW50aWFsX3ZhbGlkX2R1cmF0aW9uPVAxTQowMDJmc2lnbmF0dXJlIBBaYgRlOpoFKqpcnEzOJFKbLzul3DzLEbQbiJCxd9x3Cg=="
	DEV_USER_WALLET_VOTE        = "AgEJYnJhdmUuY29tAiNicmF2ZSB1c2VyLXdhbGxldC12b3RlIHNrdSB0b2tlbiB2MQACFHNrdT11c2VyLXdhbGxldC12b3RlAAIKcHJpY2U9MC4yNQACDGN1cnJlbmN5PUJBVAACDGRlc2NyaXB0aW9uPQACGmNyZWRlbnRpYWxfdHlwZT1zaW5nbGUtdXNlAAAGINiB9dUmpqLyeSEdZ23E4dPXwIBOUNJCFN9d5toIME2M"
	DEV_ANON_CARD_VOTE          = "AgEJYnJhdmUuY29tAiFicmF2ZSBhbm9uLWNhcmQtdm90ZSBza3UgdG9rZW4gdjEAAhJza3U9YW5vbi1jYXJkLXZvdGUAAgpwcmljZT0wLjI1AAIMY3VycmVuY3k9QkFUAAIMZGVzY3JpcHRpb249AAIaY3JlZGVudGlhbF90eXBlPXNpbmdsZS11c2UAAAYgPpv+Al9jRgVCaR49/AoRrsjQqXGqkwaNfqVka00SJxQ="
	DEV_BRAVE_TOGETHER_FREE     = "MDAyOWxvY2F0aW9uIHRvZ2V0aGVyLmJzZy5icmF2ZS5zb2Z0d2FyZQowMDMwaWRlbnRpZmllciBicmF2ZS10b2dldGhlci1mcmVlIHNrdSB0b2tlbiB2MQowMDIwY2lkIHNrdT1icmF2ZS10b2dldGhlci1mcmVlCjAwMTBjaWQgcHJpY2U9MAowMDE1Y2lkIGN1cnJlbmN5PUJBVAowMDNjY2lkIGRlc2NyaXB0aW9uPU9uZSBtb250aCBmcmVlIHRyaWFsIGZvciBCcmF2ZSBUb2dldGhlcgowMDI1Y2lkIGNyZWRlbnRpYWxfdHlwZT10aW1lLWxpbWl0ZWQKMDAyNmNpZCBjcmVkZW50aWFsX3ZhbGlkX2R1cmF0aW9uPVAxTQowMDJmc2lnbmF0dXJlIGebBXoPnj06tvlJkPEDLp9nfWo6Wfc1Txj6jTlgxjrQCg=="
	DEV_BRAVE_TOGETHER_PAID     = "MDAyOWxvY2F0aW9uIHRvZ2V0aGVyLmJzZy5icmF2ZS5zb2Z0d2FyZQowMDMwaWRlbnRpZmllciBicmF2ZS10b2dldGhlci1wYWlkIHNrdSB0b2tlbiB2MQowMDIwY2lkIHNrdT1icmF2ZS10b2dldGhlci1wYWlkCjAwMTBjaWQgcHJpY2U9NQowMDE1Y2lkIGN1cnJlbmN5PVVTRAowMDQzY2lkIGRlc2NyaXB0aW9uPU9uZSBtb250aCBwYWlkIHN1YnNjcmlwdGlvbiBmb3IgQnJhdmUgVG9nZXRoZXIKMDAyNWNpZCBjcmVkZW50aWFsX3R5cGU9dGltZS1saW1pdGVkCjAwMjZjaWQgY3JlZGVudGlhbF92YWxpZF9kdXJhdGlvbj1QMU0KMDAyZnNpZ25hdHVyZSDKLJ7NuuzP3KdmTdVnn0dI3JmIfNblQKmY+WBJOqnQJAo="
	STAGING_WEBTEST_PJ_SKU_DEMO = "AgEYd2VidGVzdC1wai5oZXJva3VhcHAuY29tAih3ZWJ0ZXN0LXBqLmhlcm9rdWFwcC5jb20gYnJhdmUtdHNoaXJ0IHYxAAIQc2t1PWJyYXZlLXRzaGlydAACCnByaWNlPTAuMjUAAgxjdXJyZW5jeT1CQVQAAgxkZXNjcmlwdGlvbj0AAhpjcmVkZW50aWFsX3R5cGU9c2luZ2xlLXVzZQAABiCcJ0zXGbSg+s3vsClkci44QQQTzWJb9UPyJASMVU11jw=="
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
			DEV_BRAVE_TOGETHER_PAID,
			STAGING_WEBTEST_PJ_SKU_DEMO:
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
// FIXME: Use accepted payment types from SKU
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
func (order Order) CreateStripeCheckoutSession(email string) CreateCheckoutSessionResponse {
	// os.Getenv("STRIPE_SECRET")
	stripe.Key = "sk_test_51HlmudHof20bphG6m8eJi9BvbPMLkMX4HPqLIiHmjdKAX21oJeO3S6izMrYTmiJm3NORBzUK1oM8STqClDRT3xQ700vyUyabNo"

	// Create customer if not already created
	i := customer.List(&stripe.CustomerListParams{
		Email: stripe.String(email),
	})

	matchingCustomers := 0
	for i.Next() {
		matchingCustomers++
	}

	var customerID string
	if matchingCustomers > 0 {
		customerID = i.Customer().ID
	} else {
		customer, _ := customer.New(&stripe.CustomerParams{
			Email: stripe.String(email),
		})
		customerID = customer.ID
	}

	// os.Getenv("STRIPE_SUCCESS_URL")
	successUrl := stripe.String("https://together.bsg.brave.software/")
	// os.Getenv("STRIPE_CANCEL_URL")
	cancelUrl := stripe.String("https://together.bsg.brave.software/")

	params := &stripe.CheckoutSessionParams{
		Customer: stripe.String(customerID),
		PaymentMethodTypes: stripe.StringSlice([]string{
			"card",
		}),
		Mode:              stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		SuccessURL:        successUrl,
		CancelURL:         cancelUrl,
		ClientReferenceID: stripe.String(order.ID.String()),
		SubscriptionData:  &stripe.CheckoutSessionSubscriptionDataParams{},
		LineItems:         order.CreateStripeLineItems(),
	}

	params.SubscriptionData.AddMetadata("orderID", order.ID.String())

	session, _ := session.New(params)

	data := CreateCheckoutSessionResponse{
		SessionID: session.ID,
	}
	return data
}

func (order Order) CreateStripeLineItems() []*stripe.CheckoutSessionLineItemParams {
	lineItems := make([]*stripe.CheckoutSessionLineItemParams, len(order.Items))
	for index, item := range order.Items {

		var stripeProduct string
		if item.SKU == "brave-together-paid" {
			stripeProduct = "price_1Hpg8nHof20bphG6X4eQ6Dit"
		}

		lineItems[index] = &stripe.CheckoutSessionLineItemParams{
			Price:    stripe.String(stripeProduct),
			Quantity: stripe.Int64(int64(item.Quantity)),
		}
	}
	return lineItems
}
