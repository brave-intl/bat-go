// Package model provides data that the SKUs service operates on.
package model

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"sort"
	"time"

	"github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/checkout/session"
	"github.com/stripe/stripe-go/v72/customer"

	"github.com/brave-intl/bat-go/libs/clients/radom"
	"github.com/brave-intl/bat-go/libs/datastore"
)

const (
	ErrOrderNotFound                          Error = "model: order not found"
	ErrOrderItemNotFound                      Error = "model: order item not found"
	ErrIssuerNotFound                         Error = "model: issuer not found"
	ErrNoRowsChangedOrder                     Error = "model: no rows changed in orders"
	ErrNoRowsChangedOrderPayHistory           Error = "model: no rows changed in order_payment_history"
	ErrExpiredStripeCheckoutSessionIDNotFound Error = "model: expired stripeCheckoutSessionId not found"
	ErrInvalidOrderNoItems                    Error = "model: invalid order: no items"
	ErrInvalidOrderNoSuccessURL               Error = "model: invalid order: no success url"
	ErrInvalidOrderNoCancelURL                Error = "model: invalid order: no cancel url"
	ErrInvalidOrderNoProductID                Error = "model: invalid order: no product id"

	// The text of the following errors is preserved as is, in case anything depends on them.
	ErrInvalidSKU              Error = "Invalid SKU Token provided in request"
	ErrDifferentPaymentMethods Error = "all order items must have the same allowed payment methods"
	ErrInvalidOrderRequest     Error = "model: no items to be created"
)

const (
	StripePaymentMethod = "stripe"
	RadomPaymentMethod  = "radom"

	// OrderStatus* represent order statuses at runtime and in db.
	OrderStatusCanceled = "canceled"
	OrderStatusPaid     = "paid"
	OrderStatusPending  = "pending"

	issuerBufferDefault  = 30
	issuerOverlapDefault = 5
)

var (
	emptyCreateCheckoutSessionResp CreateCheckoutSessionResponse
	emptyOrderTimeBounds           OrderTimeBounds
)

type radomClient interface {
	CreateCheckoutSession(ctx context.Context, req *radom.CheckoutSessionRequest) (*radom.CheckoutSessionResponse, error)
}

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
	AllowedPaymentMethods pq.StringArray       `json:"allowedPaymentMethods" db:"allowed_payment_methods"`
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

	return Slice[string](o.AllowedPaymentMethods).Contains(StripePaymentMethod)
}

// IsRadomPayable indicates whether the order is payable by Radom.
func (o *Order) IsRadomPayable() bool {
	return Slice[string](o.AllowedPaymentMethods).Contains(RadomPaymentMethod)
}

// CreateStripeCheckoutSession creates a Stripe checkout session for the order.
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

// CreateRadomCheckoutSession creates a Radom checkout session for o.
func (o *Order) CreateRadomCheckoutSession(
	ctx context.Context,
	client radomClient,
	sellerAddr string,
) (CreateCheckoutSessionResponse, error) {
	return o.CreateRadomCheckoutSessionWithTime(ctx, client, sellerAddr, time.Now().Add(24*time.Hour))
}

// CreateRadomCheckoutSessionWithTime creates a Radom checkout session for o.
func (o *Order) CreateRadomCheckoutSessionWithTime(
	ctx context.Context,
	client radomClient,
	sellerAddr string,
	expiresAt time.Time,
) (CreateCheckoutSessionResponse, error) {
	if len(o.Items) < 1 {
		return EmptyCreateCheckoutSessionResponse(), ErrInvalidOrderNoItems
	}

	successURI, ok := o.Items[0].Metadata["radom_success_uri"].(string)
	if !ok {
		return EmptyCreateCheckoutSessionResponse(), ErrInvalidOrderNoSuccessURL
	}

	cancelURI, ok := o.Items[0].Metadata["radom_cancel_uri"].(string)
	if !ok {
		return EmptyCreateCheckoutSessionResponse(), ErrInvalidOrderNoCancelURL
	}

	productID, ok := o.Items[0].Metadata["radom_product_id"].(string)
	if !ok {
		return EmptyCreateCheckoutSessionResponse(), ErrInvalidOrderNoProductID
	}

	resp, err := client.CreateCheckoutSession(ctx, &radom.CheckoutSessionRequest{
		SuccessURL: successURI,
		CancelURL:  cancelURI,
		// Gateway will be set by the client.
		Metadata: radom.Metadata([]radom.KeyValue{
			{
				Key:   "braveOrderId",
				Value: o.ID.String(),
			},
		}),
		LineItems: []radom.LineItem{
			{
				ProductID: productID,
			},
		},
		ExpiresAt: expiresAt.Unix(),
		Customizations: map[string]interface{}{
			"leftPanelColor":     "linear-gradient(125deg, rgba(0,0,128,1) 0%, RGBA(196,22,196,1) 100%)",
			"primaryButtonColor": "#000000",
			"slantedEdge":        true,
		},
	})
	if err != nil {
		return EmptyCreateCheckoutSessionResponse(), fmt.Errorf("failed to get checkout session response: %w", err)
	}

	return CreateCheckoutSessionResponse{SessionID: resp.SessionID}, nil
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

	// TODO: Remove this when products & issuers have been reworked.
	// The issuer for a product must be created when the product is created.
	IssuerConfig *IssuerConfig `json:"-" db:"-"`
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

func (l OrderItemList) TotalCost() decimal.Decimal {
	var result decimal.Decimal

	for i := range l {
		result = result.Add(l[i].Subtotal)
	}

	return result
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

type OrderTimeBounds struct {
	ValidFor *time.Duration `db:"valid_for"`
	LastPaid sql.NullTime   `db:"last_paid_at"`
}

func EmptyOrderTimeBounds() OrderTimeBounds {
	return emptyOrderTimeBounds
}

// ExpiresAt computes expiry time, and uses now if last paid was not set before.
func (x *OrderTimeBounds) ExpiresAt() time.Time {
	// Default to last paid now.
	return x.ExpiresAtWithFallback(time.Now())
}

// ExpiresAtWithFallback computes expiry time, and uses fallback for last paid, if it was not set before.
func (x *OrderTimeBounds) ExpiresAtWithFallback(fallback time.Time) time.Time {
	// Default to fallback.
	// Use valid last paid from order, if available.
	lastPaid := fallback
	if x.LastPaid.Valid {
		lastPaid = x.LastPaid.Time
	}

	var expiresAt time.Time
	if x.ValidFor != nil {
		// Compute expiry based on valid for.
		expiresAt = lastPaid.Add(*x.ValidFor)
	}

	return expiresAt
}

// CreateOrderRequest includes information needed to create an order.
type CreateOrderRequest struct {
	Email string             `json:"email" valid:"-"`
	Items []OrderItemRequest `json:"items" valid:"-"`
}

// OrderItemRequest represents an item in a order request.
type OrderItemRequest struct {
	SKU      string `json:"sku" valid:"-"`
	Quantity int    `json:"quantity" valid:"int"`
}

// CreateOrderRequestNew includes information needed to create an order.
type CreateOrderRequestNew struct {
	Email          string                `json:"email" validate:"required,email"`
	Currency       string                `json:"currency" validate:"required,iso4217"`
	StripeMetadata *OrderStripeMetadata  `json:"stripe_metadata"`
	PaymentMethods []string              `json:"payment_methods" validate:"required,gt=0"`
	Items          []OrderItemRequestNew `json:"items" validate:"required,gt=0,dive"`
}

// OrderItemRequestNew represents an item in an order request.
type OrderItemRequestNew struct {
	Quantity                    int                 `json:"quantity" validate:"required,gte=1"`
	IssuerTokenBuffer           int                 `json:"issuer_token_buffer"`
	IssuerTokenOverlap          int                 `json:"issuer_token_overlap"`
	SKU                         string              `json:"sku" validate:"required"`
	Location                    string              `json:"location" validate:"required"`
	Description                 string              `json:"description" validate:"required"`
	CredentialType              string              `json:"credential_type" validate:"required"`
	CredentialValidDuration     string              `json:"credential_valid_duration" validate:"required"`
	Price                       decimal.Decimal     `json:"price"`
	CredentialValidDurationEach *string             `json:"each_credential_valid_duration"`
	IssuanceInterval            *string             `json:"issuance_interval"`
	StripeMetadata              *ItemStripeMetadata `json:"stripe_metadata"`
}

func (r *OrderItemRequestNew) TokenBufferOrDefault() int {
	if r == nil {
		return 0
	}

	if r.IssuerTokenBuffer == 0 {
		return issuerBufferDefault
	}

	return r.IssuerTokenBuffer
}

func (r *OrderItemRequestNew) TokenOverlapOrDefault() int {
	if r == nil {
		return 0
	}

	if r.IssuerTokenOverlap == 0 {
		return issuerOverlapDefault
	}

	return r.IssuerTokenOverlap
}

// OrderStripeMetadata holds data relevant to the order in Stripe.
type OrderStripeMetadata struct {
	SuccessURI string `json:"success_uri" validate:"http_url"`
	CancelURI  string `json:"cancel_uri" validate:"http_url"`
}

func (m *OrderStripeMetadata) SuccessURL(oid string) (string, error) {
	if m == nil {
		return "", nil
	}

	return addURLParam(m.SuccessURI, "order_id", oid)
}

func (m *OrderStripeMetadata) CancelURL(oid string) (string, error) {
	if m == nil {
		return "", nil
	}

	return addURLParam(m.CancelURI, "order_id", oid)
}

// ItemStripeMetadata holds data about the product in Stripe.
type ItemStripeMetadata struct {
	ProductID string `json:"product_id"`
	ItemID    string `json:"item_id"`
}

// Metadata returns the contents of m as a map for datastore.Metadata.
//
// It can be called when m is nil.
func (m *ItemStripeMetadata) Metadata() map[string]interface{} {
	if m == nil {
		return nil
	}

	result := make(map[string]interface{})
	if m.ProductID != "" {
		result["stripe_product_id"] = m.ProductID
	}

	if m.ItemID != "" {
		result["stripe_item_id"] = m.ItemID
	}

	return result
}

// EnsureEqualPaymentMethods checks if the methods list equals the incoming list.
//
// This operation may change both slices due to sorting.
func EnsureEqualPaymentMethods(methods, incoming []string) error {
	sort.Strings(methods)
	sort.Strings(incoming)

	if !Slice[string](methods).Equal(Slice[string](incoming)) {
		return ErrDifferentPaymentMethods
	}

	return nil
}

type Slice[T comparable] []T

func (s Slice[T]) Equal(target []T) bool {
	if len(s) != len(target) {
		return false
	}

	for i, v := range s {
		if v != target[i] {
			return false
		}
	}

	return true
}

func (s Slice[T]) Contains(target T) bool {
	for _, v := range s {
		if v == target {
			return true
		}
	}

	return false
}

// Issuer represents a credential issuer.
type Issuer struct {
	ID         uuid.UUID `json:"id" db:"id"`
	MerchantID string    `json:"merchantId" db:"merchant_id"`
	PublicKey  string    `json:"publicKey" db:"public_key"`
	CreatedAt  time.Time `json:"createdAt" db:"created_at"`
}

// Name returns the name of the issuer as known by the challenge bypass server.
func (x *Issuer) Name() string {
	return x.MerchantID
}

// IssuerNew is a request to create an issuer in the database.
type IssuerNew struct {
	MerchantID string `db:"merchant_id"`
	PublicKey  string `db:"public_key"`
}

// IssuerConfig holds configuration of an issuer.
type IssuerConfig struct {
	Buffer  int
	Overlap int
}

func (c *IssuerConfig) NumIntervals() int {
	return c.Buffer + c.Overlap
}

func addURLParam(src, name, val string) (string, error) {
	raw, err := url.Parse(src)
	if err != nil {
		return "", err
	}

	v := raw.Query()
	v.Add(name, val)

	raw.RawQuery = v.Encode()

	return raw.String(), nil
}
