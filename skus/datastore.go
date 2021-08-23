package skus

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"

	"github.com/brave-intl/bat-go/datastore/grantserver"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/datastore"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/brave-intl/bat-go/utils/jsonutils"
	"github.com/brave-intl/bat-go/utils/logging"

	// needed for magic migration
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// Datastore abstracts over the underlying datastore
type Datastore interface {
	grantserver.Datastore
	// CreateOrder is used to create an order for payments
	CreateOrder(totalPrice decimal.Decimal, merchantID string, status string, currency string, location string, validFor *time.Duration, orderItems []OrderItem, allowedPaymentMethods *Methods) (*Order, error)
	// GetOrder by ID
	GetOrder(orderID uuid.UUID) (*Order, error)
	// RenewOrder - renew the order with this id
	RenewOrder(ctx context.Context, orderID uuid.UUID) error
	// UpdateOrder updates an order when it has been paid
	UpdateOrder(orderID uuid.UUID, status string) error
	// UpdateOrderMetadata adds a key value pair to an order's metadata
	UpdateOrderMetadata(orderID uuid.UUID, key string, value string) error
	// CreateTransaction creates a transaction
	CreateTransaction(orderID uuid.UUID, externalTransactionID string, status string, currency string, kind string, amount decimal.Decimal) (*Transaction, error)
	// GetTransaction returns a transaction given an external transaction id
	GetTransaction(externalTransactionID string) (*Transaction, error)
	// GetTransactions returns all the transactions for a specific order
	GetTransactions(orderID uuid.UUID) (*[]Transaction, error)
	// GetPagedMerchantTransactions returns all the transactions for a specific order
	GetPagedMerchantTransactions(ctx context.Context, merchantID uuid.UUID, pagination *inputs.Pagination) (*[]Transaction, int, error)
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
	GetOrderCreds(orderID uuid.UUID, isSigned bool) (*[]OrderCreds, error)
	// DeleteOrderCreds
	DeleteOrderCreds(orderID uuid.UUID, isSigned bool) error
	// GetOrderCredsByItemID retrieves an order credential by item id
	GetOrderCredsByItemID(orderID uuid.UUID, itemID uuid.UUID, isSigned bool) (*OrderCreds, error)
	// RunNextOrderJob
	RunNextOrderJob(ctx context.Context, worker OrderWorker) (bool, error)

	// GetKeys ret
	GetKeys(merchant string, showExpired bool) (*[]Key, error)
	// CreateKey
	CreateKey(merchant string, name string, encryptedSecretKey string, nonce string) (*Key, error)
	// DeleteKey
	DeleteKey(id uuid.UUID, delaySeconds int) (*Key, error)

	// Votes
	GetUncommittedVotesForUpdate(ctx context.Context) (*sqlx.Tx, []*VoteRecord, error)
	CommitVote(ctx context.Context, vr VoteRecord, tx *sqlx.Tx) error
	MarkVoteErrored(ctx context.Context, vr VoteRecord, tx *sqlx.Tx) error
	InsertVote(ctx context.Context, vr VoteRecord) error

	CheckExpiredCheckoutSession(uuid.UUID) (bool, string, error)
	IsStripeSub(uuid.UUID) (bool, string, error)
}

// VoteRecord - how the ac votes are stored in the queue
type VoteRecord struct {
	ID                 uuid.UUID
	RequestCredentials string
	VoteText           string
	VoteEventBinary    []byte
	Erred              bool
	Processed          bool
}

// Postgres is a Datastore wrapper around a postgres database
type Postgres struct {
	grantserver.Postgres
}

// NewPostgres creates a new Postgres Datastore
func NewPostgres(databaseURL string, performMigration bool, migrationTrack string, dbStatsPrefix ...string) (Datastore, error) {
	pg, err := grantserver.NewPostgres(databaseURL, performMigration, migrationTrack, dbStatsPrefix...)
	if pg != nil {
		return &DatastoreWithPrometheus{
			base: &Postgres{*pg}, instanceName: "payment_datastore",
		}, err
	}
	return nil, err
}

// CreateKey creates an encrypted key in the database based on the merchant
func (pg *Postgres) CreateKey(merchant string, name string, encryptedSecretKey string, nonce string) (*Key, error) {
	// interface and create an api key
	var key Key
	err := pg.RawDB().Get(&key, `
			INSERT INTO api_keys (merchant_id, name, encrypted_secret_key, nonce)
			VALUES ($1, $2, $3, $4)
			RETURNING id, name, merchant_id, encrypted_secret_key, nonce, created_at, expiry
		`,
		merchant, name, encryptedSecretKey, nonce)

	if err != nil {
		return nil, fmt.Errorf("failed to create key for merchant: %w", err)
	}
	err = key.SetSecretKey()
	if err != nil {
		return nil, fmt.Errorf("failed to set secret key for merchant: %w", err)
	}

	return &key, nil
}

// DeleteKey updates a key with an expiration time based on the id
func (pg *Postgres) DeleteKey(id uuid.UUID, delaySeconds int) (*Key, error) {
	var key Key
	err := pg.RawDB().Get(&key, `
			UPDATE api_keys
			SET expiry=(current_timestamp + $2)
			WHERE id=$1
			RETURNING id, name, merchant_id, created_at, expiry
		`, id.String(), fmt.Sprintf("%vs", delaySeconds))

	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to update key for merchant: %w", err)
	}

	return &key, nil
}

// GetKeys returns a list of active API keys
func (pg *Postgres) GetKeys(merchant string, showExpired bool) (*[]Key, error) {
	expiredQuery := "AND (expiry IS NULL or expiry > CURRENT_TIMESTAMP)"
	if showExpired {
		expiredQuery = ""
	}

	var keys []Key
	err := pg.RawDB().Select(&keys, `
			select
				id, name, merchant_id, created_at, expiry,
				encrypted_secret_key, nonce
			from api_keys
			where
			merchant_id = $1`+expiredQuery+" ORDER BY name, created_at",
		merchant)

	if err != nil {
		return nil, fmt.Errorf("failed to get keys for merchant: %w", err)
	}

	return &keys, nil
}

// CreateOrder creates orders given the total price, merchant ID, status and items of the order
func (pg *Postgres) CreateOrder(totalPrice decimal.Decimal, merchantID, status, currency, location string, validFor *time.Duration, orderItems []OrderItem, allowedPaymentMethods *Methods) (*Order, error) {
	tx := pg.RawDB().MustBegin()

	var order Order
	err := tx.Get(&order, `
			INSERT INTO orders (total_price, merchant_id, status, currency, location, allowed_payment_methods, valid_for)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, created_at, currency, updated_at, total_price, merchant_id, location, status, allowed_payment_methods, valid_for
		`,
		totalPrice, merchantID, status, currency, location, pq.Array(*allowedPaymentMethods), validFor)

	if err != nil {
		return nil, err
	}

	if status == OrderStatusPaid {
		// record the order payment
		if err := recordOrderPayment(context.Background(), tx, order.ID, time.Now()); err != nil {
			return nil, fmt.Errorf("failed to record order payment: %w", err)
		}
	}

	// TODO: We should make a generalized helper to handle bulk inserts
	query := `
		insert into order_items 
			(order_id, sku, quantity, price, currency, subtotal, location, description, credential_type, metadata, valid_for)
		values `
	params := []interface{}{}
	for i := 0; i < len(orderItems); i++ {
		// put all our params together
		params = append(params,
			order.ID, orderItems[i].SKU, orderItems[i].Quantity,
			orderItems[i].Price, orderItems[i].Currency, orderItems[i].Subtotal,
			orderItems[i].Location, orderItems[i].Description,
			orderItems[i].CredentialType, orderItems[i].Metadata, orderItems[i].ValidFor)
		numFields := 11 // the number of fields you are inserting
		n := i * numFields

		query += `(`
		for j := 0; j < numFields; j++ {
			query += `$` + strconv.Itoa(n+j+1) + `,`
		}
		query = query[:len(query)-1] + `),`
	}
	query = query[:len(query)-1] // remove the trailing comma
	query += ` RETURNING id, order_id, sku, created_at, updated_at, currency, quantity, price, location, description, credential_type, (quantity * price) as subtotal, metadata, valid_for`

	order.Items = []OrderItem{}

	err = tx.Select(&order.Items, query, params...)
	if err != nil {
		return nil, err
	}
	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	return &order, nil
}

// GetOrder queries the database and returns an order
func (pg *Postgres) GetOrder(orderID uuid.UUID) (*Order, error) {
	statement := `
		SELECT 
			id, created_at, currency, updated_at, total_price, 
			merchant_id, location, status, allowed_payment_methods, 
			metadata, valid_for, last_paid_at, expires_at
		FROM orders WHERE id = $1`
	order := Order{}
	err := pg.RawDB().Get(&order, statement, orderID)
	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	foundOrderItems := []OrderItem{}
	statement = `
		SELECT id, order_id, sku, created_at, updated_at, currency, quantity, price, (quantity * price) as subtotal, location, description, credential_type,metadata
		FROM order_items WHERE order_id = $1`
	err = pg.RawDB().Select(&foundOrderItems, statement, orderID)

	order.Items = foundOrderItems
	if err != nil {
		return nil, err
	}

	return &order, nil
}

// GetPagedMerchantTransactions - get a paginated list of transactions for a merchant
func (pg *Postgres) GetPagedMerchantTransactions(
	ctx context.Context, merchantID uuid.UUID, pagination *inputs.Pagination) (*[]Transaction, int, error) {
	var (
		total int
		err   error
	)

	countStatement := `
			SELECT count(t.*) as total
			FROM transactions as t
				INNER JOIN orders as o ON o.id = t.order_id
			WHERE o.merchant_id = $1`

	// get the total count
	row := pg.RawDB().QueryRow(countStatement, merchantID)

	if err := row.Scan(&total); err != nil {
		return nil, 0, err
	}

	getStatement := `
		SELECT t.*
		FROM transactions as t
			INNER JOIN orders as o ON o.id = t.order_id
		WHERE o.merchant_id = $1
		`

	// $ numbered params for query
	params := []interface{}{
		merchantID,
	}

	orderBy := pagination.GetOrderBy(ctx)
	if orderBy != "" {
		getStatement += fmt.Sprintf(" ORDER BY %s", orderBy)
	}

	offset := pagination.Page * pagination.Items
	if offset > 0 {
		getStatement += fmt.Sprintf(" OFFSET %d", offset)
	}

	if pagination.Items > 0 {
		getStatement += fmt.Sprintf(" FETCH NEXT %d ROWS ONLY", pagination.Items)
	}

	transactions := []Transaction{}

	rows, err := pg.RawDB().Queryx(getStatement, params...)
	if err != nil {
		return nil, 0, err
	}
	for rows.Next() {
		var transaction = new(Transaction)
		err := rows.StructScan(transaction)
		if err != nil {
			return nil, 0, err
		}
		transactions = append(transactions, *transaction)
	}
	err = rows.Close()
	if err != nil {
		return nil, 0, err
	}

	return &transactions, total, nil
}

// GetTransactions returns the list of transactions given an orderID
func (pg *Postgres) GetTransactions(orderID uuid.UUID) (*[]Transaction, error) {
	statement := `
		SELECT id, order_id, created_at, updated_at, external_transaction_id, status, currency, kind, amount
		FROM transactions WHERE order_id = $1`
	transactions := []Transaction{}
	err := pg.RawDB().Select(&transactions, statement, orderID)

	if err != nil {
		return nil, err
	}

	return &transactions, nil
}

// GetTransaction returns a single of transaction given an external transaction Id
func (pg *Postgres) GetTransaction(externalTransactionID string) (*Transaction, error) {
	statement := `
		SELECT id, order_id, created_at, updated_at, external_transaction_id, status, currency, kind, amount
		FROM transactions WHERE external_transaction_id = $1`
	transaction := Transaction{}
	err := pg.RawDB().Get(&transaction, statement, externalTransactionID)

	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return &transaction, nil
}

// CheckExpiredCheckoutSession - check order metadata for an expired checkout session id
func (pg *Postgres) CheckExpiredCheckoutSession(orderID uuid.UUID) (bool, string, error) {
	var (
		expired         bool
		checkoutSession string
		err             error
	)

	err = pg.RawDB().Get(&checkoutSession, `
		SELECT metadata->>'stripeCheckoutSessionId'
		FROM orders
		WHERE id = $1 
			AND metadata is not null
			AND status='pending'
			AND updated_at<now() - interval '1 hour'
	`, orderID)

	if err == nil && checkoutSession != "" {
		expired = true
	}
	if errors.Is(err, sql.ErrNoRows) {
		// if there are no rows, then we are not expired
		// drop this error
		return expired, checkoutSession, nil
	}
	return expired, checkoutSession, err
}

// IsStripeSub - is this order related to a stripe subscription, if so, true, subscription id returned
func (pg *Postgres) IsStripeSub(orderID uuid.UUID) (bool, string, error) {
	var (
		ok  bool
		md  datastore.Metadata
		err error
	)

	err = pg.RawDB().Get(&md, `
		SELECT metadata
		FROM orders
		WHERE id = $1 AND metadata is not null
	`, orderID)

	if err == nil {
		if v, ok := md["stripeSubscriptionId"]; ok {
			return ok, v, err
		}
	}
	return ok, "", err
}

// UpdateOrderExpiresAt - set the expires_at attribute of the order, based on now (or last paid_at if exists) and valid_for from db
func (pg *Postgres) updateOrderExpiresAt(ctx context.Context, tx *sqlx.Tx, orderID uuid.UUID) error {
	if tx == nil {
		return fmt.Errorf("need to pass in tx to update order expiry")
	}

	// how long should the order be valid for?
	var orderTimeBounds = struct {
		ValidFor *time.Duration `db:"valid_for"`
		LastPaid sql.NullTime   `db:"last_paid_at"`
	}{}

	err := tx.GetContext(ctx, &orderTimeBounds, `
		SELECT valid_for, last_paid_at
		FROM orders
		WHERE id = $1
	`, orderID)
	if err != nil {
		return fmt.Errorf("unable to get order time bounds: %w", err)
	}

	// default to last paid now
	lastPaid := time.Now()

	// if there is a valid last paid, use that from the order
	if orderTimeBounds.LastPaid.Valid {
		lastPaid = orderTimeBounds.LastPaid.Time
	}

	var expiresAt time.Time

	if orderTimeBounds.ValidFor != nil {
		// compute expiry based on valid for
		expiresAt = lastPaid.Add(*orderTimeBounds.ValidFor)
	}

	// update the order with the right expires at
	result, err := tx.ExecContext(ctx, `
		UPDATE orders
		SET
			updated_at = CURRENT_TIMESTAMP,
			expires_at = $1
		WHERE 
			id = $2
	`, expiresAt, orderID)

	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if rowsAffected == 0 || err != nil {
		return errors.New("no rows updated")
	}

	return nil
}

// RenewOrder updates the orders status to paid and paid at time, inserts record of this order
// 	Status should either be one of pending, paid, fulfilled, or canceled.
func (pg *Postgres) RenewOrder(ctx context.Context, orderID uuid.UUID) error {

	// renew order is an update order with paid status
	// and an update order expires at with the new expiry time of the order
	err := pg.UpdateOrder(orderID, OrderStatusPaid) // this performs a record order payment
	if err != nil {
		return fmt.Errorf("failed to set order status to paid: %w", err)
	}

	return pg.DeleteOrderCreds(orderID, true)
}

func recordOrderPayment(ctx context.Context, tx *sqlx.Tx, id uuid.UUID, t time.Time) error {

	// record the order payment
	// on renewal and initial payment
	result, err := tx.ExecContext(ctx, `
		INSERT INTO order_payment_history
			(order_id, last_paid)
		VALUES
			( $1, $2 )
	`, id, t)

	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if rowsAffected == 0 || err != nil {
		return errors.New("no rows updated")
	}

	if err != nil {
		return err
	}

	// record on order as well
	result, err = tx.ExecContext(ctx, `
		update orders set last_paid_at = $1
		where id = $2
	`, t, id)

	if err != nil {
		return err
	}

	rowsAffected, err = result.RowsAffected()
	if rowsAffected == 0 || err != nil {
		return errors.New("no rows updated")
	}

	if err != nil {
		return err
	}
	return nil
}

// UpdateOrder updates the orders status.
// 	Status should either be one of pending, paid, fulfilled, or canceled.
func (pg *Postgres) UpdateOrder(orderID uuid.UUID, status string) error {
	ctx := context.Background()
	// create tx
	tx, err := pg.RawDB().BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer pg.RollbackTx(tx)

	result, err := tx.Exec(`UPDATE orders set status = $1, updated_at = CURRENT_TIMESTAMP where id = $2`, status, orderID)

	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if rowsAffected == 0 || err != nil {
		return errors.New("no rows updated")
	}

	if status == OrderStatusPaid {
		// record the order payment
		if err := recordOrderPayment(ctx, tx, orderID, time.Now()); err != nil {
			return fmt.Errorf("failed to record order payment: %w", err)
		}

		// set the expires at value
		err = pg.updateOrderExpiresAt(ctx, tx, orderID)
		if err != nil {
			return fmt.Errorf("failed to set order expires_at: %w", err)
		}
	}

	return tx.Commit()
}

// CreateTransaction creates a transaction given an orderID, externalTransactionID, currency, and a kind of transaction
func (pg *Postgres) CreateTransaction(orderID uuid.UUID, externalTransactionID string, status string, currency string, kind string, amount decimal.Decimal) (*Transaction, error) {
	tx := pg.RawDB().MustBegin()
	defer pg.RollbackTx(tx)

	var transaction Transaction
	err := tx.Get(&transaction,
		`
			INSERT INTO transactions (order_id, external_transaction_id, status, currency, kind, amount)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, order_id, created_at, updated_at, external_transaction_id, status, currency, kind, amount
	`, orderID, externalTransactionID, status, currency, kind, amount)

	if err != nil {
		return nil, err
	}

	err = tx.Commit()

	if err != nil {
		return nil, err
	}

	return &transaction, nil
}

// GetSumForTransactions returns the calculated sum
func (pg *Postgres) GetSumForTransactions(orderID uuid.UUID) (decimal.Decimal, error) {
	var sum decimal.Decimal

	err := pg.RawDB().Get(&sum, `
		SELECT COALESCE(SUM(amount), 0.0) as sum
		FROM transactions
		WHERE order_id = $1 AND status = 'completed'
	`, orderID)

	return sum, err
}

// InsertIssuer inserts the given issuer
func (pg *Postgres) InsertIssuer(issuer *Issuer) (*Issuer, error) {
	statement := `
	INSERT INTO order_cred_issuers (merchant_id, public_key)
	VALUES ($1, $2)
	RETURNING id, created_at, merchant_id, public_key`
	var issuers []Issuer
	err := pg.RawDB().Select(&issuers, statement, issuer.MerchantID, issuer.PublicKey)
	if err != nil {
		return nil, err
	}

	if len(issuers) != 1 {
		return nil, errors.New("unexpected number of issuers returned")
	}

	return &issuers[0], nil
}

// GetIssuer retrieves the given issuer
func (pg *Postgres) GetIssuer(merchantID string) (*Issuer, error) {
	statement := "select id, created_at, merchant_id, public_key from order_cred_issuers where merchant_id = $1"
	var issuer Issuer
	err := pg.RawDB().Get(&issuer, statement, merchantID)
	if err != nil {
		return nil, err
	}

	return &issuer, nil
}

// GetIssuerByPublicKey or return an error
func (pg *Postgres) GetIssuerByPublicKey(publicKey string) (*Issuer, error) {
	statement := "select id, created_at, merchant_id, public_key from order_cred_issuers where public_key = $1"
	var issuer Issuer
	err := pg.RawDB().Get(&issuer, statement, publicKey)
	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return &issuer, nil
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
	_, err = pg.RawDB().Exec(statement, creds.ID, creds.OrderID, creds.IssuerID, blindedCredsJSON)
	return err
}

// GetOrderCreds returns the order credentials for a OrderID
func (pg *Postgres) GetOrderCreds(orderID uuid.UUID, isSigned bool) (*[]OrderCreds, error) {
	orderCreds := []OrderCreds{}

	query := `
		select item_id, order_id, issuer_id, blinded_creds, signed_creds, batch_proof, public_key
		from order_creds
		where order_id = $1`
	if isSigned {
		query += " and signed_creds is not null"
	}

	err := pg.RawDB().Select(&orderCreds, query, orderID)
	if err != nil {
		return nil, err
	}

	if len(orderCreds) > 0 {
		return &orderCreds, nil
	}

	return nil, nil
}

// DeleteOrderCreds deletes the order credentials for a OrderID
func (pg *Postgres) DeleteOrderCreds(orderID uuid.UUID, isSigned bool) error {

	query := `
		delete
		from order_creds
		where order_id = $1`

	if isSigned {
		query += " and signed_creds is not null"
	}

	_, err := pg.RawDB().Exec(query, orderID)
	if err != nil {
		return err
	}

	return nil
}

// GetOrderCredsByItemID returns the order credentials for a OrderID by the itemID
func (pg *Postgres) GetOrderCredsByItemID(orderID uuid.UUID, itemID uuid.UUID, isSigned bool) (*OrderCreds, error) {
	orderCreds := OrderCreds{}

	query := `
		SELECT item_id, order_id, issuer_id, blinded_creds, signed_creds, batch_proof, public_key
		FROM order_creds
		WHERE order_id = $1 AND item_id = $2`
	if isSigned {
		query += " and signed_creds is not null"
	}

	err := pg.RawDB().Get(&orderCreds, query, orderID, itemID)
	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return &orderCreds, nil
}

// GetUncommittedVotesForUpdate - row locking on number of votes we will be pulling
// returns a transaction to commit, the vote records, and an error
func (pg *Postgres) GetUncommittedVotesForUpdate(ctx context.Context) (*sqlx.Tx, []*VoteRecord, error) {
	var (
		results = make([]*VoteRecord, 100)
		tx, err = pg.RawDB().Beginx()
	)

	if err != nil {
		return tx, nil, fmt.Errorf("failed to aquire transaction: %w", err)
	}

	statement := `
select
	id, credentials, vote_text, vote_event, erred, processed
from
	vote_drain
where
	processed = false AND
	erred = false
limit 100
FOR UPDATE
`
	rows, err := tx.QueryContext(ctx, statement)
	if err != nil {
		return tx, nil, fmt.Errorf("failed to perform query for vote drain: %w", err)
	}

	for rows.Next() {
		var vr = new(VoteRecord)
		if err := rows.Scan(&vr.ID, &vr.RequestCredentials, &vr.VoteText,
			&vr.VoteEventBinary, &vr.Erred, &vr.Processed); err != nil {
			return tx, nil, fmt.Errorf("failed to scan vote drain record: %w", err)
		}
		// add to results
		results = append(results, vr)
	}
	if err := rows.Err(); err != nil {
		return tx, nil, fmt.Errorf("row errors after scanning vote drain: %w", err)
	}

	if err := rows.Close(); err != nil {
		return tx, results, fmt.Errorf("error closing rows: %w", err)
	}

	return tx, results, err
}

// MarkVoteErrored - Update a vote to show it has errored, designed to run on a transaction so
// a batch number of votes can be processed.
func (pg *Postgres) MarkVoteErrored(ctx context.Context, vr VoteRecord, tx *sqlx.Tx) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		ctx, logger = logging.SetupLogger(ctx)
	}
	logger.Debug().Msg("about to set errored to true for this vote")

	var statement = `update vote_drain set erred=true where id=$1`
	_, err = tx.ExecContext(ctx, statement, vr.ID)

	if err != nil {
		logger.Error().Err(err).Msg("failed to update vote_drain")
		return fmt.Errorf("failed to commit vote from drain: %w", err)
	}
	return nil
}

// CommitVote - Update a vote to show it has been processed, designed to run on a transaction so
// a batch number of votes can be processed.
func (pg *Postgres) CommitVote(ctx context.Context, vr VoteRecord, tx *sqlx.Tx) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		ctx, logger = logging.SetupLogger(ctx)
	}
	logger.Debug().Msg("about to set processed to true for this vote")

	var statement = `update vote_drain set processed=true where id=$1`
	_, err = tx.ExecContext(ctx, statement, vr.ID)

	if err != nil {
		logger.Error().Err(err).Msg("unable to update processed=true for vote drain job")
		return fmt.Errorf("failed to commit vote from drain: %w", err)
	}
	return nil
}

// InsertVote - Add a vote to our "queue" to be processed
func (pg *Postgres) InsertVote(ctx context.Context, vr VoteRecord) error {
	var (
		statement = `
	insert into vote_drain (credentials, vote_text, vote_event)
	values ($1, $2, $3)`
		_, err = pg.RawDB().ExecContext(ctx, statement, vr.RequestCredentials, vr.VoteText, vr.VoteEventBinary)
	)
	if err != nil {
		return fmt.Errorf("failed to insert vote to drain: %w", err)
	}
	return nil
}

// RunNextOrderJob to sign order credentials if there is a order waiting, returning true if a job was attempted
func (pg *Postgres) RunNextOrderJob(ctx context.Context, worker OrderWorker) (bool, error) {
	tx, err := pg.RawDB().Beginx()
	attempted := false
	if err != nil {
		return attempted, err
	}
	defer pg.RollbackTx(tx)

	type SigningJob struct {
		Issuer
		OrderID      uuid.UUID                 `db:"order_id"`
		BlindedCreds jsonutils.JSONStringArray `db:"blinded_creds"`
	}

	statement := `
SELECT
	order_cred_issuers.id,
	order_cred_issuers.created_at,
	order_cred_issuers.merchant_id,
	order_cred_issuers.public_key,
	order_cred.order_id,
	order_cred.blinded_creds
FROM
	(
		SELECT item_id, order_id, issuer_id, blinded_creds, signed_creds, batch_proof, public_key
		FROM order_creds
		WHERE batch_proof is null
		FOR UPDATE skip locked
		limit 1
	) order_cred
INNER JOIN order_cred_issuers
ON order_cred.issuer_id = order_cred_issuers.id`

	jobs := []SigningJob{}
	err = tx.Select(&jobs, statement)
	if err != nil {
		return attempted, err
	}

	if len(jobs) != 1 {
		return attempted, nil
	}

	job := jobs[0]

	attempted = true
	creds, err := worker.SignOrderCreds(ctx, job.OrderID, job.Issuer, job.BlindedCreds)
	if err != nil {
		// FIXME certain errors are not recoverable
		return attempted, err
	}

	_, err = tx.Exec(`update order_creds set signed_creds = $1, batch_proof = $2, public_key = $3 where order_id = $4`, creds.SignedCreds, creds.BatchProof, creds.PublicKey, creds.ID)
	if err != nil {
		return attempted, err
	}

	err = tx.Commit()
	if err != nil {
		return attempted, err
	}

	return attempted, nil
}

// UpdateOrderMetadata adds a key value pair to an order's metadata
func (pg *Postgres) UpdateOrderMetadata(orderID uuid.UUID, key string, value string) error {
	// create order
	om := datastore.Metadata{
		key: value,
	}

	stmt := `update orders set metadata = $1, updated_at = current_timestamp where id = $2`

	result, err := pg.RawDB().Exec(stmt, om, orderID.String())

	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if rowsAffected == 0 || err != nil {
		return errors.New("No rows updated")
	}

	return nil
}
