package payment

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/utils/datastore"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	stripe "github.com/stripe/stripe-go/v71"
	"github.com/stripe/stripe-go/v71/checkout/session"
	"github.com/stripe/stripe-go/v71/customer"
	macaroon "gopkg.in/macaroon.v2"
)

//StripePaymentMethod - the label for stripe payment method
const (
	StripePaymentMethod               = "stripe"
	StripeInvoiceUpdated              = "invoice.updated"
	StripeCustomerSubscriptionDeleted = "customer.subscription.deleted"
)

var (
	// ErrInvalidSKU - this sku is malformed or failed signature validation
	ErrInvalidSKU = errors.New("Invalid SKU Token provided in request")
)

// Methods type is a string slice holding payments
type Methods []string

// Equal - check equality
func (pm *Methods) Equal(b *Methods) bool {
	s1 := []string(*pm)
	s2 := []string(*b)
	sort.Strings(s1)
	sort.Strings(s2)
	return reflect.DeepEqual(s1, s2)
}

// Scan the src sql type into the passed JSONStringArray
func (pm *Methods) Scan(src interface{}) error {
	var x []sql.NullString
	var v = pq.Array(&x)

	if err := v.Scan(src); err != nil {
		return err
	}
	for i := 0; i < len(x); i++ {
		if x[i].Valid {
			*pm = append(*pm, x[i].String)
		}
	}

	return nil
}

// Value the driver.Value representation
func (pm *Methods) Value() (driver.Value, error) {
	return pq.Array(pm), nil
}

// Order includes information about a particular order
type Order struct {
	ID                    uuid.UUID            `json:"id" db:"id"`
	CreatedAt             time.Time            `json:"createdAt" db:"created_at"`
	Currency              string               `json:"currency" db:"currency"`
	UpdatedAt             time.Time            `json:"updatedAt" db:"updated_at"`
	TotalPrice            decimal.Decimal      `json:"totalPrice" db:"total_price"`
	MerchantID            string               `json:"merchantId" db:"merchant_id"`
	Location              datastore.NullString `json:"location" db:"location"`
	Status                string               `json:"status" db:"status"`
	Items                 []OrderItem          `json:"items"`
	AllowedPaymentMethods Methods              `json:"allowedPaymentMethods" db:"allowed_payment_methods"`
	Metadata              datastore.Metadata   `json:"metadata" db:"metadata"`
}

// OrderItem includes information about a particular order item
type OrderItem struct {
	ID             uuid.UUID            `json:"id" db:"id"`
	OrderID        uuid.UUID            `json:"orderId" db:"order_id"`
	SKU            string               `json:"sku" db:"sku"`
	CreatedAt      *time.Time           `json:"createdAt" db:"created_at"`
	UpdatedAt      *time.Time           `json:"updatedAt" db:"updated_at"`
	Currency       string               `json:"currency" db:"currency"`
	Quantity       int                  `json:"quantity" db:"quantity"`
	Price          decimal.Decimal      `json:"price" db:"price"`
	Subtotal       decimal.Decimal      `json:"subtotal" db:"subtotal"`
	Location       datastore.NullString `json:"location" db:"location"`
	Description    datastore.NullString `json:"description" db:"description"`
	CredentialType string               `json:"credentialType" db:"credential_type"`
	Metadata       datastore.Metadata   `json:"metadata" db:"metadata"`
}

func decodeAndUnmarshalSku(sku string) (*macaroon.Macaroon, error) {
	macBytes, err := macaroon.Base64Decode([]byte(sku))
	if err != nil {
		return nil, fmt.Errorf("failed to b64 decode sku token: %w", err)
	}
	mac := &macaroon.Macaroon{}
	if err = mac.UnmarshalBinary(macBytes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal sku token: %w", err)
	}

	return mac, nil
}

// CreateOrderItemFromMacaroon creates an order item from a macaroon
func (s *Service) CreateOrderItemFromMacaroon(ctx context.Context, sku string, quantity int) (*OrderItem, *Methods, error) {
	sublogger := logging.Logger(ctx, "CreateOrderItemFromMacaroon")

	// validation prior to decoding/unmarshaling
	valid, err := validateHardcodedSku(ctx, sku)
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to validate sku")
		return nil, nil, fmt.Errorf("failed to validate sku: %w", err)
	}

	// perform validation
	if !valid {
		sublogger.Error().Err(err).Msg("invalid sku")
		return nil, nil, ErrInvalidSKU
	}

	// read the macaroon, its valid
	mac, err := decodeAndUnmarshalSku(sku)
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to decode sku")
		return nil, nil, fmt.Errorf("failed to create order item from macaroon: %w", err)
	}

	/*
		 * TODO: macaroon library to validate macaroons
		 * don't delete the below, when we have a good macaroon lib it will be applicable

		// get the merchant's keys
		keys, err := s.Datastore.GetKeys(mac.Location(), false)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get keys for merchant to validate macaroon: %w", err)
		}

		// check if any of the keys for the merchant will validate the mac
		var valid bool
		for _, k := range *keys {
			// decrypt the merchant's secret key from db
			if err := k.SetSecretKey(); err != nil {
				return nil, nil, fmt.Errorf("unable to decrypt merchant key from db: %w", err)
			}
			// perform verify
			if _, err := mac.VerifySignature([]byte(k.SecretKey), nil); err == nil {
				// valid
				valid = true
				break
			}
		}
	*/

	caveats := mac.Caveats()
	allowedPaymentMethods := new(Methods)
	orderItem := OrderItem{}
	orderItem.Quantity = quantity
	orderItem.Location.String = mac.Location()
	orderItem.Location.Valid = true

	for i := 0; i < len(caveats); i++ {
		caveat := mac.Caveats()[i]
		values := strings.Split(string(caveat.Id), "=")
		key := strings.TrimSpace(values[0])
		value := strings.TrimSpace(strings.Join(values[1:], "="))

		switch key {
		case "sku":
			orderItem.SKU = value
		case "price", "amount":
			orderItem.Price, err = decimal.NewFromString(value)
			if err != nil {
				return nil, nil, err
			}
		case "description":
			orderItem.Description.String = value
			orderItem.Description.Valid = true
			if err != nil {
				return nil, nil, err
			}
		case "currency":
			orderItem.Currency = value
		case "credential_type":
			orderItem.CredentialType = value
		case "allowed_payment_methods":
			*allowedPaymentMethods = Methods(strings.Split(value, ","))
		case "metadata":
			err := json.Unmarshal([]byte(value), &orderItem.Metadata)
			sublogger.Debug().Str("value", value).Msg("metadata string")
			sublogger.Debug().Str("metadata", fmt.Sprintf("%+v", orderItem.Metadata)).Msg("metadata structure")
			if err != nil {
				sublogger.Error().Err(err).Msg("failed to decode sku metadata")
				return nil, nil, fmt.Errorf("failed to unmarshal macaroon metadata: %w", err)
			}
		}
	}
	newQuantity, err := decimal.NewFromString(strconv.Itoa(orderItem.Quantity))
	if err != nil {
		return nil, nil, err
	}

	orderItem.Subtotal = orderItem.Price.Mul(newQuantity)

	return &orderItem, allowedPaymentMethods, nil
}

// IsStripePayable returns true if every item is payable by Stripe
func (order Order) IsStripePayable() bool {
	// TODO: if not we need to look into subscription trials:
	/// -> https://stripe.com/docs/billing/subscriptions/trials
	return strings.Contains(strings.Join(order.AllowedPaymentMethods, ","), StripePaymentMethod)
}

// CreateCheckoutSessionResponse - the structure of a checkout session response
type CreateCheckoutSessionResponse struct {
	SessionID string `json:"checkoutSessionId"`
}

// CreateStripeCheckoutSession - Create a Stripe Checkout Session for an Order
func (order Order) CreateStripeCheckoutSession(email, successURI, cancelURI string) (CreateCheckoutSessionResponse, error) {
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
		customer, err := customer.New(&stripe.CustomerParams{
			Email: stripe.String(email),
		})
		if err != nil {
			return CreateCheckoutSessionResponse{}, fmt.Errorf("failed to create stripe customer: %w", err)
		}
		customerID = customer.ID
	}

	params := &stripe.CheckoutSessionParams{
		Customer: stripe.String(customerID),
		PaymentMethodTypes: stripe.StringSlice([]string{
			"card",
		}),
		Mode:              stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		SuccessURL:        stripe.String(successURI),
		CancelURL:         stripe.String(cancelURI),
		ClientReferenceID: stripe.String(order.ID.String()),
		SubscriptionData:  &stripe.CheckoutSessionSubscriptionDataParams{},
		LineItems:         order.CreateStripeLineItems(),
	}

	params.SubscriptionData.AddMetadata("orderID", order.ID.String())

	session, err := session.New(params)
	if err != nil {
		return CreateCheckoutSessionResponse{}, fmt.Errorf("failed to create stripe session: %w", err)
	}

	data := CreateCheckoutSessionResponse{
		SessionID: session.ID,
	}
	return data, nil
}

// CreateStripeLineItems - create line items for a checkout session with stripe
func (order Order) CreateStripeLineItems() []*stripe.CheckoutSessionLineItemParams {
	lineItems := make([]*stripe.CheckoutSessionLineItemParams, len(order.Items))
	for index, item := range order.Items {
		// since we are creating stripe line item, we can assume
		// that the stripe product is embedded in macaroon as metadata
		lineItems[index] = &stripe.CheckoutSessionLineItemParams{
			Price:    stripe.String(item.Metadata["stripe_item_id"]),
			Quantity: stripe.Int64(int64(item.Quantity)),
		}
	}
	return lineItems
}

// IsPaid returns true if the order is paid
func (order Order) IsPaid() bool {
	return order.Status == "paid"
}
