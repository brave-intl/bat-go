// Package model provides data that the SKUs service operates on.
package model

import (
	"database/sql"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"

	"github.com/brave-intl/bat-go/libs/datastore"
)

const (
	ErrSomethingWentWrong                     Error = "something went wrong"
	ErrOrderNotFound                          Error = "model: order not found"
	ErrOrderItemNotFound                      Error = "model: order item not found"
	ErrOrderNotPaid                           Error = "order not paid"
	ErrIssuerNotFound                         Error = "model: issuer not found"
	ErrNoRowsChangedOrder                     Error = "model: no rows changed in orders"
	ErrExpiredStripeCheckoutSessionIDNotFound Error = "model: expired stripeCheckoutSessionId not found"
	ErrInvalidOrderNoItems                    Error = "model: invalid order: no items"
	ErrNoStripeCheckoutSessID                 Error = "model: order: no stripe checkout session id"
	ErrInvalidOrderMetadataType               Error = "model: order: invalid metadata type"
	ErrInvalidUUID                            Error = "model: invalid uuid"

	ErrNumPerIntervalNotSet  Error = "model: invalid order: numPerInterval must be set"
	ErrNumIntervalsNotSet    Error = "model: invalid order: numIntervals must be set"
	ErrInvalidNumPerInterval Error = "model: invalid order: invalid numPerInterval"
	ErrInvalidNumIntervals   Error = "model: invalid order: invalid numIntervals"
	ErrInvalidMobileProduct  Error = "model: invalid mobile product"
	ErrNoMatchOrderReceipt   Error = "model: order_id does not match receipt order"
	ErrOrderExistsForReceipt Error = "model: order already exists for receipt"

	// The text of the following errors is preserved as is, in case anything depends on them.
	ErrInvalidSKU              Error = "Invalid SKU Token provided in request"
	ErrDifferentPaymentMethods Error = "all order items must have the same allowed payment methods"
	ErrInvalidOrderRequest     Error = "model: no items to be created"
	ErrReceiptAlreadyLinked    Error = "model: receipt already linked"
	ErrInvalidVendor           Error = "model: invalid receipt vendor"

	ErrTLV2InvalidCredNum Error = "model: invalid number of creds"

	// ErrInvalidCredType is returned when an invalid cred type has been detected.
	ErrInvalidCredType Error = "invalid credential type on order"

	// ErrUnsupportedCredType is returned when requested operation is not supported for the cred type.
	ErrUnsupportedCredType Error = "unsupported credential type"

	ErrNoRadomCheckoutSessionID Error = "model: no radom checkout session id"

	ErrRadomInvalidNumAssocSubs Error = "model: invalid number of associated subs"
	ErrRadomSubNotActive        Error = "model: sub not active"

	errInvalidNumConversion Error = "model: invalid numeric conversion"
)

const (
	// StatusClientClosedConn is not declared in net/http.
	StatusClientClosedConn = 499

	MerchID             = "brave.com"
	StripePaymentMethod = "stripe"
	RadomPaymentMethod  = "radom"

	// OrderStatus* represent order statuses at runtime and in db.
	OrderStatusCanceled = "canceled"
	OrderStatusPaid     = "paid"
	OrderStatusPending  = "pending"

	issuerBufferDefault  = 30
	issuerOverlapDefault = 5
)

const (
	VendorUnknown Vendor = "unknown"
	VendorApple   Vendor = "ios"
	VendorGoogle  Vendor = "android"
)

var emptyOrderTimeBounds OrderTimeBounds

// Vendor represents an app store vendor.
type Vendor string

func (v Vendor) String() string {
	return string(v)
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

func (o *Order) ShouldCreateTrialSessionStripe(now time.Time) bool {
	return !o.IsPaidAt(now) && o.IsStripePayable()
}

// IsPaid returns true if the order is paid.
//
// TODO: Update all callers of the method to pass time explicitly.
func (o *Order) IsPaid() bool {
	return o.IsPaidAt(time.Now())
}

// IsPaidAt returns true if the order is paid.
//
// If canceled, it checks if expires_at is in the future.
func (o *Order) IsPaidAt(now time.Time) bool {
	switch o.Status {
	case OrderStatusPaid:
		// The order is paid if the status is paid.
		return true
	case OrderStatusCanceled:
		// Check to make sure that expires_a is after now, if order is cancelled.
		if o.ExpiresAt == nil {
			return false
		}

		return o.ExpiresAt.After(now)
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

func (o *Order) NumPerInterval() (int, error) {
	numRaw, ok := o.Metadata["numPerInterval"]
	if !ok {
		return 0, ErrNumPerIntervalNotSet
	}

	result, err := numFromAny(numRaw)
	if err != nil {
		return 0, ErrInvalidNumPerInterval
	}

	return result, nil
}

func (o *Order) NumIntervals() (int, error) {
	numRaw, ok := o.Metadata["numIntervals"]
	if !ok {
		return 0, ErrNumIntervalsNotSet
	}

	result, err := numFromAny(numRaw)
	if err != nil {
		return 0, ErrInvalidNumIntervals
	}

	return result, nil
}

func (o *Order) NumPaymentFailed() int {
	numRaw, ok := o.Metadata["numPaymentFailed"]
	if !ok {
		return 0
	}

	result, _ := numFromAny(numRaw)

	return result
}

// HasItem returns the item if found.
//
// It exposes a comma, ok API similar to a map.
// Today items are stored in a slice, but it might change to a map in the future.
func (o *Order) HasItem(id uuid.UUID) (*OrderItem, bool) {
	return OrderItemList(o.Items).HasItem(id)
}

func (o *Order) StripeSubID() (string, bool) {
	sid, ok := o.Metadata["stripeSubscriptionId"].(string)

	return sid, ok
}

func (o *Order) StripeSessID() (string, bool) {
	sessID, ok := o.Metadata["stripeCheckoutSessionId"].(string)

	return sessID, ok
}

func (o *Order) RadomSubID() (string, bool) {
	sid, ok := o.Metadata["radomSubscriptionId"].(string)

	return sid, ok
}

func (o *Order) RadomSessID() (string, bool) {
	sessID, ok := o.Metadata["radomCheckoutSessionId"].(string)

	return sessID, ok
}

func (o *Order) IsIOS() bool {
	pp, ok := o.PaymentProc()
	if !ok {
		return false
	}

	vn, ok := o.Vendor()
	if !ok {
		return false
	}

	return pp == "ios" && vn == VendorApple
}

func (o *Order) IsAndroid() bool {
	pp, ok := o.PaymentProc()
	if !ok {
		return false
	}

	vn, ok := o.Vendor()
	if !ok {
		return false
	}

	return pp == "android" && vn == VendorGoogle
}

func (o *Order) IsStripe() bool {
	pp, ok := o.PaymentProc()
	if !ok {
		return false
	}

	return pp == StripePaymentMethod
}

func (o *Order) PaymentProc() (string, bool) {
	pp, ok := o.Metadata["paymentProcessor"].(string)

	return pp, ok
}

func (o *Order) Vendor() (Vendor, bool) {
	vn, ok := o.Metadata["vendor"].(string)
	if !ok {
		return VendorUnknown, false
	}

	return Vendor(vn), true
}

func (o *Order) UpdateCheckoutSessionID(id string) {
	o.Metadata["stripeCheckoutSessionId"] = id
}

// OrderItem represents a particular order item.
type OrderItem struct {
	ID                        uuid.UUID            `json:"id" db:"id"`
	OrderID                   uuid.UUID            `json:"orderId" db:"order_id"`
	SKU                       string               `json:"sku" db:"sku"`
	SKUVnt                    string               `json:"sku_variant" db:"sku_variant"`
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

func (x *OrderItem) IsLeo() bool {
	return x.SKUVnt == "brave-leo-premium" || x.SKUVnt == "brave-leo-premium-year"
}

func (x *OrderItem) StripeItemID() (string, bool) {
	itemID, ok := x.Metadata["stripe_item_id"].(string)

	return itemID, ok
}

func (x *OrderItem) RadomProductID() (string, bool) {
	itemID, ok := x.Metadata["radom_product_id"].(string)

	return itemID, ok
}

func (x *OrderItem) Issuer() string {
	return x.SKU
}

func (x *OrderItem) IsTalkAnnual() bool {
	if x == nil {
		return false
	}

	return x.SKUVnt == "brave-talk-premium-year"
}

func (x *OrderItem) IsSearchAnnual() bool {
	if x == nil {
		return false
	}

	return x.SKUVnt == "brave-search-premium-year"
}

// OrderNew represents a request to create an order in the database.
type OrderNew struct {
	MerchantID            string          `db:"merchant_id"`
	Currency              string          `db:"currency"`
	Status                string          `db:"status"`
	Location              sql.NullString  `db:"location"`
	TotalPrice            decimal.Decimal `db:"total_price"`
	AllowedPaymentMethods pq.StringArray  `db:"allowed_payment_methods"`
	ValidFor              *time.Duration  `db:"valid_for"`
}

// CreateCheckoutSessionResponse represents a checkout session response.
type CreateCheckoutSessionResponse struct {
	SessionID string `json:"checkoutSessionId"`
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

func (l OrderItemList) HasItem(id uuid.UUID) (*OrderItem, bool) {
	for i := range l {
		if uuid.Equal(l[i].ID, id) {
			return &l[i], true
		}
	}

	return nil, false

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
	SKU      string `json:"sku" valid:"-"` // This is old legacy field representing an encoded macaroon.
	Quantity int    `json:"quantity" valid:"int"`
}

// CreateOrderRequestNew includes information needed to create an order.
type CreateOrderRequestNew struct {
	Email           string                `json:"email" validate:"required,email"`
	CustomerID      string                `json:"customer_id"` // Optional.
	Currency        string                `json:"currency" validate:"required,iso4217"`
	StripeMetadata  *OrderStripeMetadata  `json:"stripe_metadata"`
	RadomMetadata   *OrderRadomMetadata   `json:"radom_metadata"`
	PaymentMethods  []string              `json:"payment_methods"`
	Discounts       []string              `json:"discounts"`
	Items           []OrderItemRequestNew `json:"items" validate:"required,gt=0,dive"`
	Metadata        map[string]string     `json:"metadata"`
	Locale          string                `json:"locale" validate:"omitempty,bcp47_language_tag"`
	PricingInterval string                `json:"pricing_interval" validate:"omitempty,oneof=one-off"`
}

func (r *CreateOrderRequestNew) IsOneOffPayment() bool {
	return r.PricingInterval == "one-off"
}

// OrderItemRequestNew represents an item in an order request.
type OrderItemRequestNew struct {
	Quantity                    int                 `json:"quantity" validate:"required,gte=1"`
	SKU                         string              `json:"sku" validate:"required"`
	SKUVnt                      string              `json:"sku_variant" validate:"required"`
	Period                      string              `json:"period"` // Not used yet.
	Location                    string              `json:"location" validate:"required"`
	Description                 string              `json:"description" validate:"required"`
	CredentialType              string              `json:"credential_type" validate:"required"`
	CredentialValidDuration     string              `json:"credential_valid_duration" validate:"required"`
	Price                       decimal.Decimal     `json:"price"`
	IssuerTokenBuffer           *int                `json:"issuer_token_buffer"`
	IssuerTokenOverlap          *int                `json:"issuer_token_overlap"`
	CredentialValidDurationEach *string             `json:"each_credential_valid_duration"`
	IssuanceInterval            *string             `json:"issuance_interval"`
	StripeMetadata              *ItemStripeMetadata `json:"stripe_metadata"`
	RadomMetadata               *ItemRadomMetadata  `json:"radom_metadata"`
}

func (r *OrderItemRequestNew) TokenBufferOrDefault() int {
	if r == nil {
		return 0
	}

	if !r.IsTLV2() {
		return 1
	}

	if r.IssuerTokenBuffer == nil {
		return issuerBufferDefault
	}

	return *r.IssuerTokenBuffer
}

func (r *OrderItemRequestNew) TokenOverlapOrDefault() int {
	if r == nil {
		return 0
	}

	if !r.IsTLV2() {
		return 0
	}

	if r.IssuerTokenOverlap == nil {
		return issuerOverlapDefault
	}

	return *r.IssuerTokenOverlap
}

func (r *OrderItemRequestNew) IsTLV2() bool {
	if r == nil {
		return false
	}

	return r.CredentialType == "time-limited-v2"
}

func (r *OrderItemRequestNew) Metadata() map[string]interface{} {
	if r == nil {
		return nil
	}

	if r.StripeMetadata != nil {
		return r.StripeMetadata.Metadata()
	}

	if r.RadomMetadata != nil {
		return r.RadomMetadata.Metadata()
	}

	return nil
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

// OrderRadomMetadata holds data relevant to the order in Radom.
type OrderRadomMetadata struct {
	SuccessURI    string `json:"success_uri" validate:"http_url"`
	CancelURI     string `json:"cancel_uri" validate:"http_url"`
	SubBackBtnURL string `json:"sub_back_button_url" validate:"omitempty,http_url"`
}

func (m *OrderRadomMetadata) SuccessURL(oid string) (string, error) {
	if m == nil {
		return "", nil
	}

	return addURLParam(m.SuccessURI, "order_id", oid)
}

func (m *OrderRadomMetadata) CancelURL(oid string) (string, error) {
	if m == nil {
		return "", nil
	}

	return addURLParam(m.CancelURI, "order_id", oid)
}

// ItemRadomMetadata holds data about the product in Radom.
type ItemRadomMetadata struct {
	ProductID string `json:"product_id"`
}

// Metadata returns the contents of m as a map for datastore.Metadata.
//
// It can be called when m is nil.
func (m *ItemRadomMetadata) Metadata() map[string]interface{} {
	if m == nil {
		return nil
	}

	result := make(map[string]interface{})
	if m.ProductID != "" {
		result["radom_product_id"] = m.ProductID
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

type TLV2CredSubmissionReport struct {
	Submitted     bool `db:"submitted"`
	ReqIDMismatch bool `db:"req_id_mismatch"`
}

// ReceiptRequest represents a receipt submitted by a mobile or web client.
type ReceiptRequest struct {
	Type           Vendor `json:"type" validate:"required,oneof=ios android"`
	Blob           string `json:"raw_receipt" validate:"required"`
	Package        string `json:"package" validate:"-"`
	SubscriptionID string `json:"subscription_id" validate:"-"`
}

type ReceiptData struct {
	Type      Vendor
	ProductID string
	ExtID     string
	ExpiresAt time.Time
}

type CreateOrderWithReceiptResponse struct {
	ID string `json:"orderId"`
}

type VerifyCredentialRequestV1 struct {
	Type         string  `json:"type" validate:"oneof=single-use time-limited time-limited-v2"`
	SKU          string  `json:"sku" validate:"-"`
	MerchantID   string  `json:"merchantId" validate:"-"`
	Presentation string  `json:"presentation" validate:"base64"`
	Version      float64 `json:"version" validate:"-"`
}

func (r *VerifyCredentialRequestV1) GetSKU() string {
	return r.SKU
}

func (r *VerifyCredentialRequestV1) GetType() string {
	return r.Type
}

func (r *VerifyCredentialRequestV1) GetMerchantID() string {
	return r.MerchantID
}

func (r *VerifyCredentialRequestV1) GetPresentation() string {
	return r.Presentation
}

type VerifyCredentialRequestV2 struct {
	SKU              string                  `json:"sku" validate:"-"`
	MerchantID       string                  `json:"merchantId" validate:"-"`
	Credential       string                  `json:"credential" validate:"base64"`
	CredentialOpaque *VerifyCredentialOpaque `json:"-" validate:"-"`
}

func (r *VerifyCredentialRequestV2) GetSKU() string {
	return r.SKU
}

func (r *VerifyCredentialRequestV2) GetType() string {
	if r.CredentialOpaque == nil {
		return ""
	}

	return r.CredentialOpaque.Type
}

func (r *VerifyCredentialRequestV2) GetMerchantID() string {
	return r.MerchantID
}

func (r *VerifyCredentialRequestV2) GetPresentation() string {
	if r.CredentialOpaque == nil {
		return ""
	}

	return r.CredentialOpaque.Presentation
}

type VerifyCredentialOpaque struct {
	Type         string  `json:"type" validate:"oneof=single-use time-limited time-limited-v2"`
	Presentation string  `json:"presentation" validate:"base64"`
	Version      float64 `json:"version" validate:"-"`
}

type SetTrialDaysRequest struct {
	Email     string `json:"email"` // TODO: Make it required.
	TrialDays int64  `json:"trialDays"`
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

func numFromAny(raw any) (int, error) {
	switch v := raw.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case int32:
		return int(v), nil
	case float32:
		return int(v), nil
	case float64:
		return int(v), nil
	case string:
		return strconv.Atoi(v)
	default:
		return 0, errInvalidNumConversion
	}
}
