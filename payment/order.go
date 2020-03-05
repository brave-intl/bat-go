package payment

import (
	"strconv"
	"strings"
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	macaroon "gopkg.in/macaroon.v2"
)

// Order includes information about a particular order
type Order struct {
	ID         uuid.UUID       `json:"id" db:"id"`
	CreatedAt  time.Time       `json:"createdAt" db:"created_at"`
	Currency   string          `json:"currency" db:"currency"`
	UpdatedAt  time.Time       `json:"updatedAt" db:"updated_at"`
	TotalPrice decimal.Decimal `json:"totalPrice" db:"total_price"`
	MerchantID string          `json:"merchantId" db:"merchant_id"`
	Status     string          `json:"status" db:"status"`
	Items      []OrderItem     `json:"items"`
}

// OrderItem includes information about a particular order item
type OrderItem struct {
	ID          uuid.UUID       `json:"id" db:"id"`
	OrderID     uuid.UUID       `json:"order_id" db:"order_id"`
	CreatedAt   *time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt   *time.Time      `json:"updatedAt" db:"updated_at"`
	Currency    string          `json:"currency" db:"currency"`
	Quantity    int             `json:"quantity" db:"quantity"`
	Price       decimal.Decimal `json:"price" db:"price"`
	Subtotal    decimal.Decimal `json:"subtotal"`
	Description string          `json:"description"`
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

	// FIXME Replace this with a lookup for the merchant's secret.
	secret := "secret"
	var discharges []*macaroon.Macaroon
	err = mac.Verify([]byte(secret), CheckCaveat, discharges)
	// Macaroon is signed with the wrong secret for the merchant. Fishy! 🐟
	if err != nil {
		return nil, err
	}

	caveats := mac.Caveats()
	orderItem := OrderItem{}
	orderItem.Quantity = quantity

	for i := 0; i < len(caveats); i++ {
		caveat := mac.Caveats()[i]
		values := strings.Split(string(caveat.Id), "=")
		key := strings.TrimSpace(values[0])
		value := strings.TrimSpace(values[1])

		switch key {
		case "id":
			uuid, err := uuid.FromString(value)
			if err != nil {
				return nil, err
			}
			orderItem.ID = uuid
		case "price":
			orderItem.Price, err = decimal.NewFromString(value)
			if err != nil {
				return nil, err
			}
		case "currency":
			orderItem.Currency = value
		case "description":
			orderItem.Description = value
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
