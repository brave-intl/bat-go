package payment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"

	"github.com/brave-intl/bat-go/datastore/grantserver"
	"github.com/brave-intl/bat-go/utils/jsonutils"
	walletservice "github.com/brave-intl/bat-go/wallet/service"

	// needed for magic migration
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// Datastore abstracts over the underlying datastore
type Datastore interface {
	walletservice.Datastore
	// CreateOrder is used to create an order for payments
	CreateOrder(totalPrice decimal.Decimal, merchantID string, status string, orderItems []OrderItem) (*Order, error)
	// GetOrder by ID
	GetOrder(orderID uuid.UUID) (*Order, error)
	// UpdateOrder updates an order when it has been paid
	UpdateOrder(orderID uuid.UUID, status string) error
	// CreateTransaction creates a transaction
	CreateTransaction(orderID uuid.UUID, externalTransactionID string, status string, currency string, kind string, amount decimal.Decimal) (*Transaction, error)
	// GetTransaction returns a transaction given an external transaction id
	GetTransaction(externalTransactionID string) (*Transaction, error)
	// GetTransactions returns all the transactions for a specific order
	GetTransactions(orderID uuid.UUID) (*[]Transaction, error)
	// GetSumForTransactions gets a decimal sum of for transactions for an order
	GetSumForTransactions(orderID uuid.UUID) (decimal.Decimal, error)
	// InsertIssuer
	InsertIssuer(issuer *Issuer) (*Issuer, error)
	// GetIssuer
	GetIssuer(merchantID string) (*Issuer, error)
	// GetIssuerByPublicKey
	GetIssuerByPublicKey(publicKey string) (*Issuer, error)
	// InsertOrderCreds
	InsertOrderCreds(creds *OrderCreds) error
	// GetOrderCreds
	GetOrderCreds(orderID uuid.UUID) (*[]OrderCreds, error)
	// RunNextOrderJob
	RunNextOrderJob(ctx context.Context, worker OrderWorker) (bool, error)
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
	statement := "SELECT * FROM orders WHERE id = $1"
	order := Order{}
	err := pg.DB.Get(&order, statement, orderID)
	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	foundOrderItems := []OrderItem{}
	statement = "SELECT * FROM order_items WHERE order_id = $1"
	err = pg.DB.Select(&foundOrderItems, statement, orderID)

	order.Items = foundOrderItems
	if err != nil {
		return nil, err
	}

	return &order, nil
}

// GetTransactions returns the list of transactions given an orderID
func (pg *Postgres) GetTransactions(orderID uuid.UUID) (*[]Transaction, error) {
	statement := "SELECT * FROM transactions WHERE order_id = $1"
	transactions := []Transaction{}
	err := pg.DB.Select(&transactions, statement, orderID)

	if err != nil {
		return nil, err
	}

	return &transactions, nil
}

// GetTransaction returns a single of transaction given an external transactin Id
func (pg *Postgres) GetTransaction(externalTransactionID string) (*Transaction, error) {
	statement := "SELECT * FROM transactions WHERE external_transaction_id = $1"
	transaction := Transaction{}
	err := pg.DB.Get(&transaction, statement, externalTransactionID)

	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return &transaction, nil
}

// UpdateOrder updates the orders status.
// 	Status should either be one of pending, paid, fulfilled, or canceled.
func (pg *Postgres) UpdateOrder(orderID uuid.UUID, status string) error {
	_, err := pg.DB.Exec(`UPDATE orders set status = $1, updated_at = CURRENT_TIMESTAMP where id = $2`, status, orderID)

	if err != nil {
		return err
	}

	return nil
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

// GetSumForTransactions returns the calculated sum
func (pg *Postgres) GetSumForTransactions(orderID uuid.UUID) (decimal.Decimal, error) {
	var sum decimal.Decimal

	err := pg.DB.Get(&sum, `
		SELECT SUM(amount) as sum
		FROM transactions
		WHERE order_id = $1 AND status = 'completed'
	`, orderID)

	return sum, err
}

// InsertIssuer inserts the given issuer
func (pg *Postgres) InsertIssuer(issuer *Issuer) (*Issuer, error) {
	statement := `
	insert into order_cred_issuers (merchant_id, public_key)
	values ($1, $2)
	returning *`
	issuers := []Issuer{}
	err := pg.DB.Select(&issuers, statement, issuer.MerchantID, issuer.PublicKey)
	if err != nil {
		return nil, err
	}

	if len(issuers) != 1 {
		return nil, errors.New("Unexpected number of issuers returned")
	}

	return &issuers[0], nil
}

// GetIssuer retrieves the given issuer
func (pg *Postgres) GetIssuer(merchantID string) (*Issuer, error) {
	statement := "select * from order_cred_issuers where merchant_id = $1"
	issuers := []Issuer{}
	err := pg.DB.Select(&issuers, statement, merchantID)
	if err != nil {
		return nil, err
	}

	if len(issuers) > 0 {
		return &issuers[0], nil
	}

	return nil, nil
}

// GetIssuerByPublicKey or return an error
func (pg *Postgres) GetIssuerByPublicKey(publicKey string) (*Issuer, error) {
	statement := "select * from order_cred_issuers where public_key = $1"
	issuers := []Issuer{}
	err := pg.DB.Select(&issuers, statement, publicKey)
	if err != nil {
		return nil, err
	}

	if len(issuers) > 0 {
		return &issuers[0], nil
	}

	return nil, nil
}

// InsertOrderCreds inserts the given order creds
func (pg *Postgres) InsertOrderCreds(creds *OrderCreds) error {
	blindedCredsJSON, err := json.Marshal(creds.BlindedCreds)
	if err != nil {
		return err
	}

	statement := `
	insert into order_creds (item_id, order_id, issuer_id, blinded_creds)
	values ($1, $2, $3, $4)`
	_, err = pg.DB.Exec(statement, creds.ID, creds.OrderID, creds.IssuerID, blindedCredsJSON)
	return err
}

// GetOrderCreds returns the order credentials for a OrderID
func (pg *Postgres) GetOrderCreds(orderID uuid.UUID) (*[]OrderCreds, error) {
	orderCreds := []OrderCreds{}
	err := pg.DB.Select(&orderCreds, "select * from order_creds where order_id = $1 and signed_creds is not null", orderID)
	if err != nil {
		return nil, err
	}

	if len(orderCreds) > 0 {
		return &orderCreds, nil
	}

	return nil, nil
}

// RunNextOrderJob to sign order credentials if there is a order waiting, returning true if a job was attempted
func (pg *Postgres) RunNextOrderJob(ctx context.Context, worker OrderWorker) (bool, error) {
	tx, err := pg.DB.Beginx()
	attempted := false
	if err != nil {
		return attempted, err
	}

	type SigningJob struct {
		Issuer
		OrderID      uuid.UUID                 `db:"order_id"`
		BlindedCreds jsonutils.JSONStringArray `db:"blinded_creds"`
	}

	statement := `
select
	order_cred_issuers.*,
	order_cred.order_id,
	order_cred.blinded_creds
from
	(select *
	from order_creds
	where batch_proof is null
	for update skip locked
	limit 1
) order_cred
inner join order_cred_issuers
on order_cred.issuer_id = order_cred_issuers.id`

	jobs := []SigningJob{}
	err = tx.Select(&jobs, statement)
	if err != nil {
		_ = tx.Rollback()
		return attempted, err
	}

	if len(jobs) != 1 {
		_ = tx.Rollback()
		return attempted, nil
	}

	job := jobs[0]

	attempted = true
	creds, err := worker.SignOrderCreds(ctx, job.OrderID, job.Issuer, job.BlindedCreds)
	if err != nil {
		// FIXME certain errors are not recoverable
		_ = tx.Rollback()
		return attempted, err
	}

	_, err = tx.Exec(`update order_creds set signed_creds = $1, batch_proof = $2, public_key = $3 where order_id = $4`, creds.SignedCreds, creds.BatchProof, creds.PublicKey, creds.ID)
	if err != nil {
		_ = tx.Rollback()
		return attempted, err
	}

	err = tx.Commit()
	if err != nil {
		return attempted, err
	}

	return attempted, nil
}
