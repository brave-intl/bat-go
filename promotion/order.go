package promotion

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	uuid "github.com/satori/go.uuid"
	"gopkg.in/macaroon.v2"
)

// Order includes information about a particular order
type Order struct {
	ID         uuid.UUID `json:"id" db:"id"`
	CreatedAt  time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt  time.Time `json:"updatedAt" db:"updated_at"`
	TotalPrice string    `json:"totalPrice" db:"total_price"`
	MerchantID string    `json:"merchantId" db:"merchant_id"`
	Status     string    `json:"status" db:"status"`
}

// OrderItem includes information about a particular order item
type OrderItem struct {
	ID          uuid.UUID    `json:"id" db:"id"`
	OrderID     uuid.UUID    `json:"order_id" db:"order_id"`
	CreatedAt   sql.NullTime `json:"createdAt" db:"created_at"`
	UpdatedAt   sql.NullTime `json:"updatedAt" db:"updated_at"`
	Currency    string       `json:"currency" db:"currency"`
	Quantity    int          `json:"quantity" db:"quantity"`
	Price       string       `json:"price" db:"price"`
	Subtotal    string       `json:"subtotal"`
	Description string       `json:"description" db:"description"`
	Image       string       `json:"image" db:"image"`
	Kind        string       `json:"kind" db:"kind"`
}

// CreateOrderItemFromMacaroon creates an order item from a macaroon
func CreateOrderItemFromMacaroon(sku string, quantity int) OrderItem {
	macBytes, err := macaroon.Base64Decode([]byte(sku))
	if err != nil {
		panic(err)
	}
	mac := &macaroon.Macaroon{}
	err = mac.UnmarshalBinary(macBytes)
	if err != nil {
		panic(err)
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
				panic(err)
			}
			orderItem.ID = uuid
		case "price":
			orderItem.Price = value
		case "currency":
			orderItem.Currency = value
		}
	}
	price, err := strconv.ParseFloat(orderItem.Price, 64)
	orderItem.Subtotal = fmt.Sprintf("%f", price*float64(quantity))

	return orderItem
}
