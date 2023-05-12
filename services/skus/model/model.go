// Package model provides data that the SKUs service operates on.
package model

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/checkout/session"
	"github.com/stripe/stripe-go/v72/customer"

	"github.com/brave-intl/bat-go/libs/datastore"
)

const (
	ErrOrderNotFound     Error = "model: order not found"
	ErrOrderItemNotFound Error = "model: order item not found"
)

const (
	StripePaymentMethod = "stripe"

	// OrderStatus* represent order statuses at runtime and in db.
	OrderStatusCanceled = "canceled"
	OrderStatusPaid     = "paid"
	OrderStatusPending  = "pending"
)

var (
	emptyCreateCheckoutSessionResp CreateCheckoutSessionResponse
)

// Order represents an individual order.
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

// IsStripePayable returns true if every item is payable by Stripe.
func (o *Order) IsStripePayable() bool {
	// TODO: if not we need to look into subscription trials:
	// -> https://stripe.com/docs/billing/subscriptions/trials

	return strings.Contains(strings.Join(o.AllowedPaymentMethods, ","), StripePaymentMethod)
}

// CreateStripeCheckoutSession creats a Stripe checkout session for the order.
func (o *Order) CreateStripeCheckoutSession(
	email, successURI, cancelURI string,
	freeTrialDays int64,
) (CreateCheckoutSessionResponse, error) {
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

	sd := &stripe.CheckoutSessionSubscriptionDataParams{}
	// If a free trial is set, apply it.
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
		ClientReferenceID: stripe.String(o.ID.String()),
		SubscriptionData:  sd,
		LineItems:         OrderItemList(o.Items).stripeLineItems(),
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

	params.SubscriptionData.AddMetadata("orderID", o.ID.String())
	params.AddExtra("allow_promotion_codes", "true")

	session, err := session.New(params)
	if err != nil {
		return EmptyCreateCheckoutSessionResponse(), fmt.Errorf("failed to create stripe session: %w", err)
	}

	return CreateCheckoutSessionResponse{SessionID: session.ID}, nil
}

// IsPaid returns true if the order is paid.
func (o *Order) IsPaid() bool {
	switch o.Status {
	case OrderStatusPaid:
		// The order is paid if the status is paid.
		return true
	case OrderStatusCanceled:
		// Check to make sure that expires_a is after now, if order is cancelled.
		if o.ExpiresAt == nil {
			return false
		}

		return o.ExpiresAt.After(time.Now())
	default:
		return false
	}
}

func (o *Order) GetTrialDays() int64 {
	if o.TrialDays == nil {
		return 0
	}

	return *o.TrialDays
}

// OrderItem represents a particular order item.
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

// Methods represents payment methods.
type Methods []string

// Equal checks if m equals m2.
func (m *Methods) Equal(m2 *Methods) bool {
	s1 := []string(*m)
	s2 := []string(*m2)
	sort.Strings(s1)
	sort.Strings(s2)

	return reflect.DeepEqual(s1, s2)
}

// Scan scans the raw src value into m as JSONStringArray.
func (m *Methods) Scan(src interface{}) error {
	var x []sql.NullString
	if err := pq.Array(&x).Scan(src); err != nil {
		return err
	}

	for i := range x {
		if x[i].Valid {
			*m = append(*m, x[i].String)
		}
	}

	return nil
}

// Value satisifies the drive.Valuer interface.
func (m *Methods) Value() (driver.Value, error) {
	return pq.Array(m), nil
}

// CreateCheckoutSessionResponse represents a checkout session response.
type CreateCheckoutSessionResponse struct {
	SessionID string `json:"checkoutSessionId"`
}

func EmptyCreateCheckoutSessionResponse() CreateCheckoutSessionResponse {
	return emptyCreateCheckoutSessionResp
}

type OrderItemList []OrderItem

func (l OrderItemList) SetOrderID(orderID uuid.UUID) {
	for i := range l {
		l[i].OrderID = orderID
	}
}

func (l OrderItemList) stripeLineItems() []*stripe.CheckoutSessionLineItemParams {
	result := make([]*stripe.CheckoutSessionLineItemParams, 0, len(l))

	for _, item := range l {
		// Obtain the item id from the metadata.
		priceID, ok := item.Metadata["stripe_item_id"].(string)
		if !ok {
			continue
		}

		// Assume that the stripe product is embedded in macaroon as metadata
		// because a stripe line item is being created.
		result = append(result, &stripe.CheckoutSessionLineItemParams{
			Price:    stripe.String(priceID),
			Quantity: stripe.Int64(int64(item.Quantity)),
		})
	}

	return result
}

type Error string

func (e Error) Error() string {
	return string(e)
}
