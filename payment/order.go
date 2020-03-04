package payment

import (
	"fmt"
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
	ID        uuid.UUID       `json:"id" db:"id"`
	OrderID   uuid.UUID       `json:"order_id" db:"order_id"`
	CreatedAt *time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt *time.Time      `json:"updatedAt" db:"updated_at"`
	Currency  string          `json:"currency" db:"currency"`
	Quantity  int             `json:"quantity" db:"quantity"`
	Price     decimal.Decimal `json:"price" db:"price"`
	Subtotal  decimal.Decimal `json:"subtotal"`
}

func checkMacaroon(caveat string) error {
	fmt.Println("Caveat: " + caveat)

	return nil
}

// CreateOrderItemFromMacaroon creates an order item from a macaroon
func CreateOrderItemFromMacaroon(sku string, quantity int) (*OrderItem, error) {
	fmt.Println("Creating SKU")
	macBytes, err := macaroon.Base64Decode([]byte(sku))
	if err != nil {
		return nil, err
	}
	fmt.Println("Creating Macaroon")
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
	fmt.Println("YEs?")
	secret := "secret"
	var discharges []*macaroon.Macaroon
	err = mac.Verify([]byte(secret), checkMacaroon, discharges)
	if err != nil {
		return nil, err
	}
	fmt.Println("no error?")

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
