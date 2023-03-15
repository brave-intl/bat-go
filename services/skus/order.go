package skus

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

	"github.com/brave-intl/bat-go/libs/datastore"
	"github.com/brave-intl/bat-go/libs/logging"
	timeutils "github.com/brave-intl/bat-go/libs/time"
	"github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/checkout/session"
	"github.com/stripe/stripe-go/v72/customer"
	"gopkg.in/macaroon.v2"
)

const (
	paymentProcessor = "paymentProcessor"
	// IOSPaymentMethod - indicating this used an ios payment method
	IOSPaymentMethod = "ios"
	// AndroidPaymentMethod - indicating this used an android payment method
	AndroidPaymentMethod = "android"
)

//StripePaymentMethod - the label for stripe payment method
const (
	StripePaymentMethod               = "stripe"
	StripeInvoiceUpdated              = "invoice.updated"
	StripeInvoicePaid                 = "invoice.paid"
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
	LastPaidAt            *time.Time           `json:"lastPaidAt" db:"last_paid_at"`
	ExpiresAt             *time.Time           `json:"expiresAt" db:"expires_at"`
	ValidFor              *time.Duration       `json:"validFor" db:"valid_for"`
	TrialDays             *int64               `json:"-" db:"trial_days"`
}

func (order *Order) getTrialDays() int64 {
	if order.TrialDays == nil {
		return 0
	}
	return *order.TrialDays
}

// OrderItem includes information about a particular order item
type OrderItem struct {
	ID                        uuid.UUID            `json:"id" db:"id"`
	OrderID                   uuid.UUID            `json:"orderId" db:"order_id"`
	SKU                       string               `json:"sku" db:"sku"`
	CreatedAt                 *time.Time           `json:"createdAt" db:"created_at"`
	UpdatedAt                 *time.Time           `json:"updatedAt" db:"updated_at"`
	Currency                  string               `json:"currency" db:"currency"`
	Quantity                  int                  `json:"quantity" db:"quantity"`
	Price                     decimal.Decimal      `json:"price" db:"price"`
	Subtotal                  decimal.Decimal      `json:"subtotal" db:"subtotal"`
	Location                  datastore.NullString `json:"location" db:"location"`
	Description               datastore.NullString `json:"description" db:"description"`
	CredentialType            string               `json:"credentialType" db:"credential_type"`
	ValidFor                  *time.Duration       `json:"validFor" db:"valid_for"`
	ValidForISO               *string              `json:"validForIso" db:"valid_for_iso"`
	EachCredentialValidForISO *string              `json:"-" db:"each_credential_valid_for_iso"`
	Metadata                  datastore.Metadata   `json:"metadata" db:"metadata"`
	IssuanceIntervalISO       *string              `json:"issuanceInterval" db:"issuance_interval"`
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

// IssuerConfig - the configuration of an issuer
type IssuerConfig struct {
	buffer  int
	overlap int
}

// CreateOrderItemFromMacaroon creates an order item from a macaroon
func (s *Service) CreateOrderItemFromMacaroon(ctx context.Context, sku string, quantity int) (*OrderItem, *Methods, *IssuerConfig, error) {
	sublogger := logging.Logger(ctx, "CreateOrderItemFromMacaroon")

	// validation prior to decoding/unmarshalling
	valid, err := validateHardcodedSku(ctx, sku)
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to validate sku")
		return nil, nil, nil, fmt.Errorf("failed to validate sku: %w", err)
	}

	// perform validation
	if !valid {
		sublogger.Error().Err(err).Msg("invalid sku")
		return nil, nil, nil, ErrInvalidSKU
	}

	// read the macaroon, its valid
	mac, err := decodeAndUnmarshalSku(sku)
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to decode sku")
		return nil, nil, nil, fmt.Errorf("failed to create order item from macaroon: %w", err)
	}

	caveats := mac.Caveats()
	allowedPaymentMethods := new(Methods)
	orderItem := OrderItem{}
	orderItem.Quantity = quantity

	orderItem.Location.String = mac.Location()
	orderItem.Location.Valid = true

	issuerConfig := &IssuerConfig{
		buffer:  defaultBuffer,
		overlap: defaultOverlap,
	}

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
				return nil, nil, nil, err
			}
		case "description":
			orderItem.Description.String = value
			orderItem.Description.Valid = true
			if err != nil {
				return nil, nil, nil, err
			}
		case "currency":
			orderItem.Currency = value
		case "credential_type":
			orderItem.CredentialType = value
		case "issuance_interval":
			orderItem.IssuanceIntervalISO = &value
		case "credential_valid_duration":
			// actually the expires time
			orderItem.ValidFor = new(time.Duration)
			id, err := timeutils.ParseDuration(value)
			if err != nil {
				sublogger.Error().Err(err).Msg("failed to decode sku credential_valid_duration")
				return nil, nil, nil, fmt.Errorf("failed to unmarshal macaroon metadata: %w", err)
			}
			t, err := id.FromNow()
			if err != nil {
				sublogger.Error().Err(err).Msg("failed to decode sku credential_valid_duration")
				return nil, nil, nil, fmt.Errorf("failed to unmarshal macaroon metadata: %w", err)
			}
			*orderItem.ValidFor = time.Until(*t)
			orderItem.ValidForISO = &value
		case "each_credential_valid_duration":
			// for time aware issuers we need to explain per order item
			// what the duration of each credential is
			_, err := timeutils.ParseDuration(value) // parse the duration
			if err != nil {
				sublogger.Error().Err(err).Msg("failed to decode sku each_credential_valid_duration")
				return nil, nil, nil, fmt.Errorf("failed to unmarshal macaroon metadata: %w", err)
			}
			// set the duration iso for the order item
			orderItem.EachCredentialValidForISO = &value
		case "issuer_token_buffer":
			buffer, err := strconv.Atoi(value)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("error converting buffer for order item %s: %w", orderItem.ID, err)
			}
			issuerConfig.buffer = buffer
		case "issuer_token_overlap":
			overlap, err := strconv.Atoi(value)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("error converting overlap for order item %s: %w", orderItem.ID, err)
			}
			issuerConfig.overlap = overlap
		case "allowed_payment_methods":
			*allowedPaymentMethods = Methods(strings.Split(value, ","))
		case "metadata":
			err := json.Unmarshal([]byte(value), &orderItem.Metadata)
			sublogger.Debug().Str("value", value).Msg("metadata string")
			sublogger.Debug().Str("metadata", fmt.Sprintf("%+v", orderItem.Metadata)).Msg("metadata structure")
			if err != nil {
				sublogger.Error().Err(err).Msg("failed to decode sku metadata")
				return nil, nil, nil, fmt.Errorf("failed to unmarshal macaroon metadata: %w", err)
			}
		}
	}
	newQuantity, err := decimal.NewFromString(strconv.Itoa(orderItem.Quantity))
	if err != nil {
		return nil, nil, nil, err
	}

	orderItem.Subtotal = orderItem.Price.Mul(newQuantity)

	return &orderItem, allowedPaymentMethods, issuerConfig, nil
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

func getEmailFromCheckoutSession(stripeSession *stripe.CheckoutSession) string {
	// has an existing checkout session
	var email string
	if stripeSession == nil {
		// stripe session does not exist
		return email
	}
	if stripeSession.CustomerEmail != "" {
		// if the email was stored on the stripe session customer email, use it
		email = stripeSession.CustomerEmail
	} else if stripeSession.Customer != nil && stripeSession.Customer.Email != "" {
		// if the stripe session has a customer record, with an email, use it
		email = stripeSession.Customer.Email
	}
	// if there is no record of an email, stripe will ask for it and make a new customer
	return email
}

// CreateStripeCheckoutSession - Create a Stripe Checkout Session for an Order
func (order Order) CreateStripeCheckoutSession(email, successURI, cancelURI string, freeTrialDays int64) (CreateCheckoutSessionResponse, error) {

	var custID string

	if email != "" {
		// find the existing customer by email
		// so we can use the customer id instead of a customer email
		i := customer.List(&stripe.CustomerListParams{
			Email: stripe.String(email),
		})

		for i.Next() {
			custID = i.Customer().ID
		}
	}

	var sd = &stripe.CheckoutSessionSubscriptionDataParams{}

	// if a free trial is set, apply it
	if freeTrialDays > 0 {
		sd.TrialPeriodDays = &freeTrialDays
	}

	params := &stripe.CheckoutSessionParams{
		PaymentMethodTypes: stripe.StringSlice([]string{
			"card",
		}),
		Mode:              stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		SuccessURL:        stripe.String(successURI),
		CancelURL:         stripe.String(cancelURI),
		ClientReferenceID: stripe.String(order.ID.String()),
		SubscriptionData:  sd,
		LineItems:         order.CreateStripeLineItems(),
	}

	if custID != "" {
		// try to use existing customer we found by email
		params.Customer = stripe.String(custID)
	} else if email != "" {
		// if we dont have an existing customer, this CustomerEmail param will create a new one
		params.CustomerEmail = stripe.String(email)
	}
	// else we have no record of this email for this checkout session
	// the user will be asked for the email, we cannot send an empty customer email as a param

	params.SubscriptionData.AddMetadata("orderID", order.ID.String())
	params.AddExtra("allow_promotion_codes", "true")
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
		// get the item id from the metadata
		priceID, ok := item.Metadata["stripe_item_id"].(string)
		if !ok {
			continue
		}
		// since we are creating stripe line item, we can assume
		// that the stripe product is embedded in macaroon as metadata
		lineItems[index] = &stripe.CheckoutSessionLineItemParams{
			Price:    stripe.String(priceID),
			Quantity: stripe.Int64(int64(item.Quantity)),
		}
	}
	return lineItems
}

// IsPaid returns true if the order is paid
func (order Order) IsPaid() bool {
	// if the order status is paid it is paid.
	// if the order is cancelled, check to make sure that expires at is after now
	if order.Status == OrderStatusPaid {
		return true
	} else if order.Status == OrderStatusCanceled && order.ExpiresAt != nil {
		expires := *order.ExpiresAt
		return expires.After(time.Now())
	}
	return false
}

// RenewOrder updates the orders status to paid and paid at time, inserts record of this order
// Status should either be one of pending, paid, fulfilled, or canceled.
func (s *Service) RenewOrder(ctx context.Context, orderID uuid.UUID) error {

	// renew order is an update order with paid status
	// and an update order expires at with the new expiry time of the order
	err := s.Datastore.UpdateOrder(orderID, OrderStatusPaid) // this performs a record order payment
	if err != nil {
		return fmt.Errorf("failed to set order status to paid: %w", err)
	}

	return s.DeleteOrderCreds(ctx, orderID, true)
}
