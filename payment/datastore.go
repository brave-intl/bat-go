package payment

import (
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"

	"github.com/brave-intl/bat-go/datastore/grantserver"
	// needed for magic migration
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// Datastore abstracts over the underlying datastore
type Datastore interface {
	// CreateOrder is used to create an order for payments
	CreateOrder(totalPrice decimal.Decimal, merchantID string, status string, orderItems []OrderItem) (*Order, error)
	// GetOrder by ID
	GetOrder(orderID uuid.UUID) (*Order, error)
	// CreateTransaction
	CreateTransaction(orderID uuid.UUID, externalTransactionID string, status string, currency string, kind string, amount decimal.Decimal) (*Transaction, error)
}

// Postgres is a Datastore wrapper around a postgres database
type Postgres struct {
	grantserver.Postgres
}

// NewPostgres creates a new Postgres Datastore
func NewPostgres(databaseURL string, performMigration bool) (*Postgres, error) {
	pg, err := grantserver.NewPostgres(databaseURL, performMigration)
	if pg != nil {
		return &Postgres{*pg}, err
	}
	return nil, err
}

// CreateOrder creates orders given the total price, merchant ID, status and items of the order
func (pg *Postgres) CreateOrder(totalPrice decimal.Decimal, merchantID string, status string, orderItems []OrderItem) (*Order, error) {
	tx := pg.DB.MustBegin()

	var order Order
	err := tx.Get(&order, `
			INSERT INTO orders (total_price, merchant_id, status)
			VALUES ($1, $2, $3)
			RETURNING *
		`,
		totalPrice, merchantID, status)

	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	for i := 0; i < len(orderItems); i++ {
		orderItems[i].OrderID = order.ID

		nstmt, _ := tx.PrepareNamed(`
			INSERT INTO order_items (order_id, quantity, price, currency, subtotal)
			VALUES (:order_id, :quantity, :price, :currency, :subtotal)
			RETURNING *
		`)
		err = nstmt.Get(&orderItems[i], orderItems[i])

		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	err = tx.Commit()
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	order.Items = orderItems

	return &order, nil
}

// GetOrder queries the database and returns an order
func (pg *Postgres) GetOrder(orderID uuid.UUID) (*Order, error) {
	statement := "select * from orders where id = $1"
	order := Order{}
	err := pg.DB.Get(&order, statement, orderID)
	if err != nil {
		return nil, err
	}

	foundOrderItems := []OrderItem{}
	statement = "select * from order_items where order_id = $1"
	err = pg.DB.Select(&foundOrderItems, statement, orderID)

	order.Items = foundOrderItems
	if err != nil {
		return nil, err
	}

	return &order, nil
}

// CreateTransaction creates a transaction given an orderID, externalTransactionID, currency, and a kind of transaction
func (pg *Postgres) CreateTransaction(orderID uuid.UUID, externalTransactionID string, status string, currency string, kind string, amount decimal.Decimal) (*Transaction, error) {
	tx := pg.DB.MustBegin()

	var transaction Transaction
	err := tx.Get(&transaction,
		`
			INSERT INTO transactions (order_id, external_transaction_id, status, currency, kind, amount)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING *
	`, orderID, externalTransactionID, status, currency, kind, amount)

	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	err = tx.Commit()

	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	return &transaction, nil
}
