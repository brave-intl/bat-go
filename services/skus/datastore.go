package skus

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	// needed for magic migration
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/getsentry/sentry-go"
	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"
	"github.com/segmentio/kafka-go"
	"github.com/shopspring/decimal"

	"github.com/brave-intl/bat-go/libs/datastore"
	"github.com/brave-intl/bat-go/libs/inputs"
	"github.com/brave-intl/bat-go/libs/jsonutils"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/ptr"

	"github.com/brave-intl/bat-go/services/skus/model"
)

const (
	signingRequestBatchSize = 10

	errNotFound    = model.Error("not found")
	errNoTLV2Creds = model.Error("no unexpired time-limited-v2 credentials found")
)

// Datastore abstracts over the underlying datastore.
type Datastore interface {
	datastore.Datastore

	CreateOrder(ctx context.Context, dbi sqlx.ExtContext, oreq *model.OrderNew, items []model.OrderItem) (*model.Order, error)
	// SetOrderTrialDays - set the number of days of free trial for this order
	SetOrderTrialDays(ctx context.Context, orderID *uuid.UUID, days int64) (*Order, error)
	// GetOrder by ID
	GetOrder(orderID uuid.UUID) (*Order, error)
	// GetOrderByExternalID by the external id from the purchase vendor
	GetOrderByExternalID(externalID string) (*Order, error)
	// UpdateOrder updates an order when it has been paid
	UpdateOrder(orderID uuid.UUID, status string) error
	// UpdateOrderMetadata adds a key value pair to an order's metadata
	UpdateOrderMetadata(orderID uuid.UUID, key string, value string) error
	// CreateTransaction creates a transaction
	CreateTransaction(orderID uuid.UUID, externalTransactionID string, status string, currency string, kind string, amount decimal.Decimal) (*Transaction, error)
	// UpdateTransaction creates a transaction
	UpdateTransaction(orderID uuid.UUID, externalTransactionID string, status string, currency string, kind string, amount decimal.Decimal) (*Transaction, error)
	// GetTransaction returns a transaction given an external transaction id
	GetTransaction(externalTransactionID string) (*Transaction, error)
	// GetTransactions returns all the transactions for a specific order
	GetTransactions(orderID uuid.UUID) (*[]Transaction, error)
	// GetPagedMerchantTransactions returns all the transactions for a specific order
	GetPagedMerchantTransactions(ctx context.Context, merchantID uuid.UUID, pagination *inputs.Pagination) (*[]Transaction, int, error)
	// GetSumForTransactions gets a decimal sum of for transactions for an order
	GetSumForTransactions(orderID uuid.UUID) (decimal.Decimal, error)

	GetIssuerByPublicKey(publicKey string) (*Issuer, error)
	DeleteSingleUseOrderCredsByOrderTx(ctx context.Context, tx *sqlx.Tx, orderID uuid.UUID, isSigned bool) error
	// GetOrderCredsByItemID retrieves an order credential by item id
	GetOrderCredsByItemID(orderID uuid.UUID, itemID uuid.UUID, isSigned bool) (*OrderCreds, error)
	GetKeysByMerchant(merchant string, showExpired bool) (*[]Key, error)
	GetKey(id uuid.UUID, showExpired bool) (*Key, error)
	CreateKey(merchant string, name string, encryptedSecretKey string, nonce string) (*Key, error)
	DeleteKey(id uuid.UUID, delaySeconds int) (*Key, error)
	GetUncommittedVotesForUpdate(ctx context.Context) (*sqlx.Tx, []*VoteRecord, error)
	CommitVote(ctx context.Context, vr VoteRecord, tx *sqlx.Tx) error
	MarkVoteErrored(ctx context.Context, vr VoteRecord, tx *sqlx.Tx) error
	InsertVote(ctx context.Context, vr VoteRecord) error
	CheckExpiredCheckoutSession(uuid.UUID) (bool, string, error)
	IsStripeSub(uuid.UUID) (bool, string, error)
	GetOrderItem(ctx context.Context, itemID uuid.UUID) (*OrderItem, error)
	InsertOrderCredsTx(ctx context.Context, tx *sqlx.Tx, creds *OrderCreds) error
	GetOrderCreds(orderID uuid.UUID, isSigned bool) ([]OrderCreds, error)
	SendSigningRequest(ctx context.Context, signingRequestWriter SigningRequestWriter) error
	InsertSignedOrderCredentialsTx(ctx context.Context, tx *sqlx.Tx, signedOrderResult *SigningOrderResult) error
	AreTimeLimitedV2CredsSubmitted(ctx context.Context, requestID uuid.UUID, blindedCreds ...string) (AreTimeLimitedV2CredsSubmittedResult, error)
	GetTimeLimitedV2OrderCredsByOrder(orderID uuid.UUID) (*TimeLimitedV2Creds, error)
	GetTLV2Creds(ctx context.Context, dbi sqlx.QueryerContext, ordID, itemID, reqID uuid.UUID) (*TimeLimitedV2Creds, error)
	DeleteTimeLimitedV2OrderCredsByOrderTx(ctx context.Context, tx *sqlx.Tx, orderID uuid.UUID) error
	GetTimeLimitedV2OrderCredsByOrderItem(itemID uuid.UUID) (*TimeLimitedV2Creds, error)
	InsertTimeLimitedV2OrderCredsTx(ctx context.Context, tx *sqlx.Tx, tlv2 TimeAwareSubIssuedCreds) error
	InsertSigningOrderRequestOutbox(ctx context.Context, requestID uuid.UUID, orderID uuid.UUID, itemID uuid.UUID, signingOrderRequest SigningOrderRequest) error
	GetSigningOrderRequestOutboxByRequestID(ctx context.Context, dbi sqlx.QueryerContext, reqID uuid.UUID) (*SigningOrderRequestOutbox, error)
	GetSigningOrderRequestOutboxByOrder(ctx context.Context, orderID uuid.UUID) ([]SigningOrderRequestOutbox, error)
	GetSigningOrderRequestOutboxByOrderItem(ctx context.Context, itemID uuid.UUID) ([]SigningOrderRequestOutbox, error)
	DeleteSigningOrderRequestOutboxByOrderTx(ctx context.Context, tx *sqlx.Tx, orderID uuid.UUID) error
	UpdateSigningOrderRequestOutboxTx(ctx context.Context, tx *sqlx.Tx, requestID uuid.UUID, completedAt time.Time) error
	SetOrderPaid(context.Context, *uuid.UUID) error
	AppendOrderMetadata(context.Context, *uuid.UUID, string, string) error
	AppendOrderMetadataInt(context.Context, *uuid.UUID, string, int) error
	AppendOrderMetadataInt64(context.Context, *uuid.UUID, string, int64) error
	GetOutboxMovAvgDurationSeconds() (int64, error)
	ExternalIDExists(context.Context, string) (bool, error)
}

type orderStore interface {
	Get(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error)
	GetByExternalID(ctx context.Context, dbi sqlx.QueryerContext, extID string) (*model.Order, error)
	Create(ctx context.Context, dbi sqlx.QueryerContext, oreq *model.OrderNew) (*model.Order, error)
	SetLastPaidAt(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error
	SetTrialDays(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID, ndays int64) (*model.Order, error)
	SetStatus(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, status string) error
	GetExpiresAtAfterISOPeriod(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (time.Time, error)
	SetExpiresAt(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error
	UpdateMetadata(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, data datastore.Metadata) error
	AppendMetadata(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error
	AppendMetadataInt(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key string, val int) error
	AppendMetadataInt64(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key string, val int64) error
	GetExpiredStripeCheckoutSessionID(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) (string, error)
	HasExternalID(ctx context.Context, dbi sqlx.QueryerContext, extID string) (bool, error)
	GetMetadata(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (datastore.Metadata, error)
}

type orderItemStore interface {
	Get(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.OrderItem, error)
	FindByOrderID(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) ([]model.OrderItem, error)
	InsertMany(ctx context.Context, dbi sqlx.ExtContext, items ...model.OrderItem) ([]model.OrderItem, error)
}

type orderPayHistoryStore interface {
	Insert(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error
}

type issuerStore interface {
	GetByMerchID(ctx context.Context, dbi sqlx.QueryerContext, merchID string) (*model.Issuer, error)
	GetByPubKey(ctx context.Context, dbi sqlx.QueryerContext, pubKey string) (*model.Issuer, error)
	Create(ctx context.Context, dbi sqlx.QueryerContext, req model.IssuerNew) (*model.Issuer, error)
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
	datastore.Postgres

	orderRepo       orderStore
	orderItemRepo   orderItemStore
	orderPayHistory orderPayHistoryStore
	issuerRepo      issuerStore
}

// NewPostgres creates a new Postgres Datastore.
func NewPostgres(
	orderRepo orderStore,
	orderItemRepo orderItemStore,
	orderPayHistory orderPayHistoryStore,
	issuerRepo issuerStore,
	databaseURL string,
	performMigration bool,
	migrationTrack string,
	dbStatsPrefix ...string,
) (Datastore, error) {
	pg, err := newPostgres(databaseURL, performMigration, migrationTrack, dbStatsPrefix...)
	if err != nil {
		return nil, err
	}

	pg.orderRepo = orderRepo
	pg.orderItemRepo = orderItemRepo
	pg.orderPayHistory = orderPayHistory
	pg.issuerRepo = issuerRepo

	return &DatastoreWithPrometheus{base: pg, instanceName: "payment_datastore"}, nil
}

func newPostgres(databaseURL string, performMigration bool, migrationTrack string, dbStatsPrefix ...string) (*Postgres, error) {
	pg, err := datastore.NewPostgres(databaseURL, performMigration, migrationTrack, dbStatsPrefix...)
	if err != nil {
		return nil, err
	}

	return &Postgres{Postgres: *pg}, nil
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

// GetKeysByMerchant returns a list of active API keys
func (pg *Postgres) GetKeysByMerchant(merchant string, showExpired bool) (*[]Key, error) {
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

// GetKey returns the specified key, conditionally checking if it is expired
func (pg *Postgres) GetKey(id uuid.UUID, showExpired bool) (*Key, error) {
	expiredQuery := " AND (expiry IS NULL or expiry > CURRENT_TIMESTAMP)"
	if showExpired {
		expiredQuery = ""
	}

	var key Key
	err := pg.RawDB().Get(&key, `
			select
				id, name, merchant_id, created_at, expiry,
				encrypted_secret_key, nonce
			from api_keys
			where
			id = $1`+expiredQuery,
		id.String())

	if err != nil {
		return nil, fmt.Errorf("failed to get key: %w", err)
	}

	return &key, nil
}

// SetOrderTrialDays sets the number of days of free trial for this order and returns the updated result.
func (pg *Postgres) SetOrderTrialDays(ctx context.Context, orderID *uuid.UUID, days int64) (*Order, error) {
	tx, err := pg.RawDB().BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create db tx: %w", err)
	}
	defer pg.RollbackTx(tx)

	result, err := pg.orderRepo.SetTrialDays(ctx, tx, *orderID, days)
	if err != nil {
		return nil, fmt.Errorf("failed to execute tx: %w", err)
	}

	result.Items, err = pg.orderItemRepo.FindByOrderID(ctx, tx, *orderID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return result, nil
}

// CreateOrder creates an order from the given prototype, and inserts items.
func (pg *Postgres) CreateOrder(ctx context.Context, dbi sqlx.ExtContext, oreq *model.OrderNew, items []model.OrderItem) (*model.Order, error) {
	result, err := pg.orderRepo.Create(ctx, dbi, oreq)
	if err != nil {
		return nil, err
	}

	if oreq.Status == OrderStatusPaid {
		if err := pg.recordOrderPayment(ctx, dbi, result.ID, time.Now()); err != nil {
			return nil, fmt.Errorf("failed to record order payment: %w", err)
		}
	}

	model.OrderItemList(items).SetOrderID(result.ID)

	result.Items, err = pg.orderItemRepo.InsertMany(ctx, dbi, items...)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetOrderByExternalID returns an order by the external id from the purchase vendor.
func (pg *Postgres) GetOrderByExternalID(externalID string) (*Order, error) {
	ctx := context.TODO()
	dbi := pg.RawDB()

	result, err := pg.orderRepo.GetByExternalID(ctx, dbi, externalID)
	if err != nil {
		// Preserve the legacy behaviour.
		// TODO: Propagate the sentinel error, and handle in the business logic properly.
		if errors.Is(err, model.ErrOrderNotFound) {
			return nil, nil
		}

		return nil, err
	}

	result.Items, err = pg.orderItemRepo.FindByOrderID(ctx, dbi, result.ID)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetOutboxMovAvgDurationSeconds - get the number of seconds it takes to clear the last 20 outbox messages
func (pg *Postgres) GetOutboxMovAvgDurationSeconds() (int64, error) {
	statement := `
		select
			coalesce(ceiling(extract(epoch from avg(completed_at-created_at))),1)
		from
			(select completed_at, created_at from signing_order_request_outbox order by completed_at desc limit 20) as q;
`
	var seconds int64
	if err := pg.RawDB().Get(&seconds, statement); err != nil {
		return 0, err
	}
	if seconds > 5 {
		// set max allowable retry after
		seconds = 5
	}
	return seconds, nil
}

// GetOrder returns an order from the database.
func (pg *Postgres) GetOrder(orderID uuid.UUID) (*Order, error) {
	ctx := context.TODO()
	dbi := pg.RawDB()

	result, err := pg.orderRepo.Get(ctx, dbi, orderID)
	if err != nil {
		// Preserve the legacy behaviour.
		// TODO: Propagate the sentinel error, and handle in the business logic properly.
		if errors.Is(err, model.ErrOrderNotFound) {
			return nil, nil
		}

		return nil, err
	}

	result.Items, err = pg.orderItemRepo.FindByOrderID(ctx, dbi, orderID)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetOrderItem retrieves the order item for the given identifier.
//
// It returns sql.ErrNoRows if the item is not found.
func (pg *Postgres) GetOrderItem(ctx context.Context, itemID uuid.UUID) (*OrderItem, error) {
	result, err := pg.orderItemRepo.Get(ctx, pg.RawDB(), itemID)
	if err != nil {
		// Preserve the legacy behaviour.
		// TODO: Propagate the sentinel error, and handle in the business logic properly.
		if errors.Is(err, model.ErrOrderItemNotFound) {
			return nil, sql.ErrNoRows
		}

		return nil, err
	}

	return result, nil
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

// CheckExpiredCheckoutSession indicates whether a Stripe checkout session is expired with its id for the given orderID.
//
// TODO(pavelb): The boolean return value is unnecessary, and can be removed.
// If there is experied session, the session id is present.
// If there is no session, or it has not expired, the result is the same – no session id.
// It's the caller's responsibility (the business logic layer) to interpret the result.
func (pg *Postgres) CheckExpiredCheckoutSession(orderID uuid.UUID) (bool, string, error) {
	ctx := context.TODO()

	sessID, err := pg.orderRepo.GetExpiredStripeCheckoutSessionID(ctx, pg.RawDB(), orderID)
	if err != nil {
		if errors.Is(err, model.ErrExpiredStripeCheckoutSessionIDNotFound) {
			return false, "", nil
		}

		return false, "", fmt.Errorf("failed to check expired state of checkout session: %w", err)
	}

	if sessID == "" {
		return false, "", nil
	}

	return true, sessID, nil
}

func (pg *Postgres) ExternalIDExists(ctx context.Context, externalID string) (bool, error) {
	return pg.orderRepo.HasExternalID(ctx, pg.RawDB(), externalID)
}

// IsStripeSub reports whether the order is associated with a stripe subscription, if true, subscription id is returned.
//
// TODO(pavelb): This is a piece of business logic that leaked to the storage layer.
// Also, it unsuccessfully mimics the Go comma, ok idiom – bool and string should be swapped.
// But that's not necessary.
// If metadata was found, but there was no stripeSubscriptionId, it's known not to be a Stripe order.
func (pg *Postgres) IsStripeSub(orderID uuid.UUID) (bool, string, error) {
	ctx := context.TODO()

	data, err := pg.orderRepo.GetMetadata(ctx, pg.RawDB(), orderID)
	if err != nil {
		return false, "", err
	}

	sid, ok := data["stripeSubscriptionId"].(string)

	return ok, sid, nil
}

// UpdateOrder updates the orders status.
//
// Status should either be one of pending, paid, fulfilled, or canceled.
//
// TODO: rename it to better reflect the behaviour.
func (pg *Postgres) UpdateOrder(orderID uuid.UUID, status string) error {
	ctx := context.Background()

	tx, err := pg.RawDB().BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer pg.RollbackTx(tx)

	if err := pg.orderRepo.SetStatus(ctx, tx, orderID, status); err != nil {
		return err
	}

	if status == OrderStatusPaid {
		if err := pg.recordOrderPayment(ctx, tx, orderID, time.Now()); err != nil {
			return fmt.Errorf("failed to record order payment: %w", err)
		}

		if err := pg.updateOrderExpiresAt(ctx, tx, orderID); err != nil {
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

// UpdateTransaction creates a transaction given an orderID, externalTransactionID, currency, and a kind of transaction
func (pg *Postgres) UpdateTransaction(orderID uuid.UUID, externalTransactionID string, status string, currency string, kind string, amount decimal.Decimal) (*Transaction, error) {
	tx := pg.RawDB().MustBegin()
	defer pg.RollbackTx(tx)

	var transaction Transaction
	err := tx.Get(&transaction,
		`
			update transactions set status=$1, currency=$2, kind=$3, amount=$4
			where external_transaction_id=$5 and order_id=$6
			RETURNING id, order_id, created_at, updated_at, external_transaction_id, status, currency, kind, amount
	`, status, currency, kind, amount, externalTransactionID, orderID)

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

// GetIssuerByPublicKey returns an issuer by the pubKey.
//
// Deprecated: Use the corresponding repository directly with GetByPubKey.
func (pg *Postgres) GetIssuerByPublicKey(pubKey string) (*Issuer, error) {
	result, err := pg.issuerRepo.GetByPubKey(context.TODO(), pg.RawDB(), pubKey)
	if err != nil {
		// Preserve the old behaviour.
		// TODO: Fix this as it defeats the purpose of multiple returns and has risks for callers.
		// Thankfully, there is only one caller, but that coller, hypothetically, might panic.
		if errors.Is(err, model.ErrIssuerNotFound) {
			return nil, nil
		}

		return nil, err
	}

	return result, nil
}

// InsertOrderCredsTx inserts the given order creds.
func (pg *Postgres) InsertOrderCredsTx(ctx context.Context, tx *sqlx.Tx, creds *OrderCreds) error {
	blindedCredsJSON, err := json.Marshal(creds.BlindedCreds)
	if err != nil {
		return err
	}

	statement := `
	insert into order_creds (item_id, order_id, issuer_id, blinded_creds, signed_creds, batch_proof, public_key)
	values ($1, $2, $3, $4, $5, $6, $7)`
	_, err = tx.ExecContext(ctx, statement, creds.ID, creds.OrderID, creds.IssuerID, blindedCredsJSON,
		creds.SignedCreds, creds.BatchProof, creds.PublicKey)

	return err
}

// GetOrderCreds returns the order credentials for a OrderID
func (pg *Postgres) GetOrderCreds(orderID uuid.UUID, isSigned bool) ([]OrderCreds, error) {
	query := `
		select item_id, order_id, issuer_id, blinded_creds, signed_creds, batch_proof, public_key
		from order_creds
		where order_id = $1`
	if isSigned {
		query += " and signed_creds is not null"
	}

	var orderCreds []OrderCreds
	err := pg.RawDB().Select(&orderCreds, query, orderID)
	if err != nil {
		return nil, err
	}

	if len(orderCreds) > 0 {
		return orderCreds, nil
	}

	return nil, nil
}

// GetOrderTimeLimitedV2Creds returns all order credentials for an order.
func (pg *Postgres) GetOrderTimeLimitedV2Creds(orderID uuid.UUID) (*[]TimeLimitedV2Creds, error) {
	// each "order item" is a different record
	var timeLimitedV2Creds []TimeLimitedV2Creds
	var timeAwareSubIssuedCreds []TimeAwareSubIssuedCreds

	// get all the credentials related to the order_id
	query := `
		select order_id, item_id, issuer_id, blinded_creds, signed_creds, batch_proof, public_key, valid_from, valid_to
		from order_creds
		where order_id = $1

	`

	err := pg.RawDB().Select(&timeAwareSubIssuedCreds, query, orderID)
	if err != nil {
		return nil, err
	}

	// convert our time aware creds into result
	itemCredMap := map[uuid.UUID][]TimeAwareSubIssuedCreds{}
	for i := range timeAwareSubIssuedCreds {
		itemCredMap[timeAwareSubIssuedCreds[i].ItemID] = append(itemCredMap[timeAwareSubIssuedCreds[i].ItemID], timeAwareSubIssuedCreds[i])
	}

	for _, v := range itemCredMap {
		if len(v) > 0 {
			timeLimitedV2Creds = append(timeLimitedV2Creds, TimeLimitedV2Creds{
				OrderID:     v[0].OrderID,
				IssuerID:    v[0].IssuerID,
				Credentials: v,
			})
		}
	}

	if len(timeLimitedV2Creds) > 0 {
		return &timeLimitedV2Creds, nil
	}

	return nil, nil
}

// GetOrderTimeLimitedV2CredsByItemID returns the order credentials by order and item.
func (pg *Postgres) GetOrderTimeLimitedV2CredsByItemID(orderID uuid.UUID, itemID uuid.UUID) (*TimeLimitedV2Creds, error) {
	query := `
		select
			order_id, item_id, issuer_id, blinded_creds, signed_creds, batch_proof, public_key, valid_from, valid_to
		from order_creds
		where
			order_id = $1
			and item_id = $2
		  	and valid_to is not null
			and valid_from is not null
	`

	var timeAwareCreds []TimeAwareSubIssuedCreds
	err := pg.RawDB().Select(&timeAwareCreds, query, orderID, itemID)
	if err != nil {
		return nil, fmt.Errorf("error getting time aware creds: %w", err)
	}

	if len(timeAwareCreds) == 0 {
		return nil, nil
	}

	return &TimeLimitedV2Creds{
		OrderID:     timeAwareCreds[0].OrderID,
		IssuerID:    timeAwareCreds[0].IssuerID,
		Credentials: timeAwareCreds,
	}, nil
}

// DeleteSingleUseOrderCredsByOrderTx performs a hard delete all single use order credentials for a given OrderID.
func (pg *Postgres) DeleteSingleUseOrderCredsByOrderTx(ctx context.Context, tx *sqlx.Tx, orderID uuid.UUID, isSigned bool) error {
	query := `delete from order_creds where order_id = $1`
	if isSigned {
		query += " and signed_creds is not null"
	}

	_, err := tx.ExecContext(ctx, query, orderID)
	if err != nil {
		return fmt.Errorf("error deleting single use order creds: %w", err)
	}

	return nil
}

// DeleteTimeLimitedV2OrderCredsByOrderTx performs a hard delete for all time limited v2 order
// credentials for a given OrderID.
func (pg *Postgres) DeleteTimeLimitedV2OrderCredsByOrderTx(ctx context.Context, tx *sqlx.Tx, orderID uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `delete from time_limited_v2_order_creds where order_id = $1`, orderID)
	if err != nil {
		return fmt.Errorf("error deleting time limited v2 order creds: %w", err)
	}
	return nil
}

// GetOrderCredsByItemID returns the order credentials for a OrderID by the itemID.
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
	logger := logging.Logger(ctx, "skus.MarkVoteErrored")
	logger.Debug().Msg("about to set errored to true for this vote")

	var statement = `update vote_drain set erred=true where id=$1`
	_, err := tx.ExecContext(ctx, statement, vr.ID)

	if err != nil {
		logger.Error().Err(err).Msg("failed to update vote_drain")
		return fmt.Errorf("failed to commit vote from drain: %w", err)
	}
	return nil
}

// CommitVote - Update a vote to show it has been processed, designed to run on a transaction so
// a batch number of votes can be processed.
func (pg *Postgres) CommitVote(ctx context.Context, vr VoteRecord, tx *sqlx.Tx) error {
	logger := logging.Logger(ctx, "skus.CommitVote")
	logger.Debug().Msg("about to set processed to true for this vote")

	var statement = `update vote_drain set processed=true where id=$1`
	_, err := tx.ExecContext(ctx, statement, vr.ID)

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

// UpdateOrderMetadata sets the order's metadata to the key and value.
//
// Deprecated: This method is no longer used and should be deleted.
//
// TODO(pavelb): Remove this method as it's dangerous and must not be used.
func (pg *Postgres) UpdateOrderMetadata(orderID uuid.UUID, key string, value string) error {
	return model.Error("UpdateOrderMetadata must not be used")
}

// TimeLimitedV2Creds represent all the
type TimeLimitedV2Creds struct {
	OrderID     uuid.UUID                 `json:"orderId"`
	IssuerID    uuid.UUID                 `json:"issuerId" `
	Credentials []TimeAwareSubIssuedCreds `json:"credentials"`
}

// TimeAwareSubIssuedCreds sub issued time aware credentials
type TimeAwareSubIssuedCreds struct {
	OrderID      uuid.UUID                 `json:"orderId" db:"order_id"`
	ItemID       uuid.UUID                 `json:"itemId" db:"item_id"`
	IssuerID     uuid.UUID                 `json:"issuerId" db:"issuer_id"`
	ValidTo      time.Time                 `json:"validTo" db:"valid_to"`
	ValidFrom    time.Time                 `json:"validFrom" db:"valid_from"`
	BlindedCreds jsonutils.JSONStringArray `json:"blindedCreds" db:"blinded_creds"`
	SignedCreds  jsonutils.JSONStringArray `json:"signedCreds" db:"signed_creds"`
	BatchProof   string                    `json:"batchProof" db:"batch_proof"`
	PublicKey    string                    `json:"publicKey" db:"public_key"`
	RequestID    string                    `json:"-" db:"request_id"`
}

type AreTimeLimitedV2CredsSubmittedResult struct {
	AlreadySubmitted bool `db:"already_submitted"`
	Mismatch         bool `db:"mismatch"`
}

func (pg *Postgres) AreTimeLimitedV2CredsSubmitted(ctx context.Context, requestID uuid.UUID, blindedCreds ...string) (AreTimeLimitedV2CredsSubmittedResult, error) {
	return areTimeLimitedV2CredsSubmitted(ctx, pg.RawDB(), requestID, blindedCreds...)
}

type getContext interface {
	GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
}

func areTimeLimitedV2CredsSubmitted(ctx context.Context, dbi getContext, requestID uuid.UUID, blindedCreds ...string) (AreTimeLimitedV2CredsSubmittedResult, error) {
	var result = AreTimeLimitedV2CredsSubmittedResult{}

	if len(blindedCreds) < 1 {
		return result, errors.New("invalid parameter to tlv2 creds signed")
	}

	const query = `
		select exists(
			select 1 from time_limited_v2_order_creds where blinded_creds->>0 = $1
		) as already_submitted,
		exists(
			select 1 from time_limited_v2_order_creds where blinded_creds->>0 != $1 and request_id = $2
		) as mismatch
	`
	err := dbi.GetContext(ctx, &result, query, blindedCreds[0], requestID)
	if err != nil {
		return result, err
	}

	return result, nil
}

// GetTimeLimitedV2OrderCredsByOrder returns all the non expired time limited v2 order credentials for a given order.
func (pg *Postgres) GetTimeLimitedV2OrderCredsByOrder(orderID uuid.UUID) (*TimeLimitedV2Creds, error) {
	query := `
		select order_id, item_id, issuer_id, blinded_creds, signed_creds, batch_proof, public_key,
		valid_from, valid_to
		from time_limited_v2_order_creds
		where order_id = $1 and valid_to > now()
	`

	var timeAwareSubIssuedCreds []TimeAwareSubIssuedCreds
	err := pg.RawDB().Select(&timeAwareSubIssuedCreds, query, orderID)
	if err != nil {
		return nil, err
	}

	if len(timeAwareSubIssuedCreds) == 0 {
		return nil, nil
	}

	timeLimitedV2Creds := TimeLimitedV2Creds{
		OrderID:     timeAwareSubIssuedCreds[0].OrderID,
		IssuerID:    timeAwareSubIssuedCreds[0].IssuerID,
		Credentials: timeAwareSubIssuedCreds,
	}

	return &timeLimitedV2Creds, nil
}

// GetTLV2Creds returns all the non expired tlv2 credentials for a given order, item and request ids.
//
// If no credentials have been found, the method returns errNoTLV2Creds.
func (pg *Postgres) GetTLV2Creds(ctx context.Context, dbi sqlx.QueryerContext, ordID, itemID, reqID uuid.UUID) (*TimeLimitedV2Creds, error) {
	const q = `SELECT
		order_id, item_id, issuer_id, blinded_creds, signed_creds,
		batch_proof, public_key, valid_from, valid_to
	FROM time_limited_v2_order_creds
	WHERE order_id = $1 AND item_id = $2 AND request_id = $3 AND valid_to > now()`

	creds := make([]TimeAwareSubIssuedCreds, 0)
	if err := sqlx.SelectContext(ctx, dbi, &creds, q, ordID, itemID, reqID); err != nil {
		return nil, err
	}

	if len(creds) == 0 {
		return nil, errNoTLV2Creds
	}

	result := &TimeLimitedV2Creds{
		OrderID:     creds[0].OrderID,
		IssuerID:    creds[0].IssuerID,
		Credentials: creds,
	}

	return result, nil
}

// GetTimeLimitedV2OrderCredsByOrderItem returns all the order credentials for a single order item.
func (pg *Postgres) GetTimeLimitedV2OrderCredsByOrderItem(itemID uuid.UUID) (*TimeLimitedV2Creds, error) {
	query := `
		select order_id, item_id, issuer_id, blinded_creds, signed_creds, batch_proof, public_key,
		valid_from, valid_to
		from time_limited_v2_order_creds
		where item_id = $1 and valid_to > now()
	`

	var timeAwareSubIssuedCreds []TimeAwareSubIssuedCreds
	err := pg.RawDB().Select(&timeAwareSubIssuedCreds, query, itemID)
	if err != nil {
		return nil, fmt.Errorf("error getting time aware creds: %w", err)
	}

	if len(timeAwareSubIssuedCreds) == 0 {
		return nil, nil
	}

	return &TimeLimitedV2Creds{
		OrderID:     timeAwareSubIssuedCreds[0].OrderID,
		IssuerID:    timeAwareSubIssuedCreds[0].IssuerID,
		Credentials: timeAwareSubIssuedCreds,
	}, nil
}

// InsertTimeLimitedV2OrderCredsTx inserts time limited v2 credentials.
func (pg *Postgres) InsertTimeLimitedV2OrderCredsTx(ctx context.Context, tx *sqlx.Tx, tlv2 TimeAwareSubIssuedCreds) error {
	blindedCredsJSON, err := json.Marshal(tlv2.BlindedCreds)
	if err != nil {
		return fmt.Errorf("error marshaling blinded creds: %w", err)
	}

	signedCredsJSON, err := json.Marshal(tlv2.SignedCreds)
	if err != nil {
		return fmt.Errorf("error marshaling signed creds: %w", err)
	}

	// continue to insert the credential
	query := `insert into time_limited_v2_order_creds(item_id, order_id, issuer_id, blinded_creds,
                        signed_creds, batch_proof, public_key, valid_to, valid_from, request_id)
                    values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) on conflict do nothing`

	_, err = tx.ExecContext(ctx, query, tlv2.ItemID, tlv2.OrderID, tlv2.IssuerID, blindedCredsJSON,
		signedCredsJSON, tlv2.BatchProof, tlv2.PublicKey, tlv2.ValidTo, tlv2.ValidFrom, tlv2.RequestID)
	if err != nil {
		return fmt.Errorf("error inserting row: %w", err)
	}

	return nil
}

// SigningOrderRequestOutbox - model for the signing request outbox
type SigningOrderRequestOutbox struct {
	RequestID   uuid.UUID       `db:"request_id"`
	OrderID     uuid.UUID       `db:"order_id"`
	ItemID      uuid.UUID       `db:"item_id"`
	CompletedAt *time.Time      `db:"completed_at"`
	Message     json.RawMessage `db:"message_data" json:"message"`
}

// GetSigningOrderRequestOutboxByOrder retrieves the latest signing order from the outbox.
func (pg *Postgres) GetSigningOrderRequestOutboxByOrder(ctx context.Context, orderID uuid.UUID) ([]SigningOrderRequestOutbox, error) {
	var signingRequestOutbox []SigningOrderRequestOutbox
	err := pg.RawDB().SelectContext(ctx, &signingRequestOutbox,
		`select request_id, order_id, item_id, completed_at, message_data
				from signing_order_request_outbox where order_id = $1`, orderID)
	if err != nil {
		return nil, fmt.Errorf("error retrieving signing request from outbox: %w", err)
	}
	return signingRequestOutbox, nil
}

// GetSigningOrderRequestOutboxByOrderItem retrieves the latest signing order from the outbox.
// An empty result set is returned if no rows are found.
func (pg *Postgres) GetSigningOrderRequestOutboxByOrderItem(ctx context.Context, itemID uuid.UUID) ([]SigningOrderRequestOutbox, error) {
	var signingRequestOutbox []SigningOrderRequestOutbox
	err := pg.RawDB().SelectContext(ctx, &signingRequestOutbox,
		`select request_id, order_id, item_id, completed_at, message_data
				from signing_order_request_outbox where item_id = $1`, itemID)
	if err != nil {
		return nil, fmt.Errorf("error retrieving signing requests from outbox: %w", err)
	}
	return signingRequestOutbox, nil
}

// GetSigningOrderRequestOutboxByRequestID retrieves the SigningOrderRequestOutbox by requestID.
//
// An error is returned if the result set is empty.
func (pg *Postgres) GetSigningOrderRequestOutboxByRequestID(ctx context.Context, dbi sqlx.QueryerContext, reqID uuid.UUID) (*SigningOrderRequestOutbox, error) {
	const q = `SELECT request_id, order_id, item_id, completed_at, message_data
	FROM signing_order_request_outbox
	WHERE request_id = $1 FOR UPDATE`

	result := &SigningOrderRequestOutbox{}
	if err := sqlx.GetContext(ctx, dbi, result, q, reqID); err != nil {
		return nil, fmt.Errorf("error retrieving signing request from outbox: %w", err)
	}

	return result, nil
}

// UpdateSigningOrderRequestOutboxTx updates a signing order request outbox message for the given requestID.
func (pg *Postgres) UpdateSigningOrderRequestOutboxTx(ctx context.Context, tx *sqlx.Tx, requestID uuid.UUID, completedAt time.Time) error {
	_, err := tx.ExecContext(ctx, `update signing_order_request_outbox set completed_at = $2
                                    where request_id = $1`, requestID, completedAt)
	if err != nil {
		return fmt.Errorf("error updating signing order request: %w", err)
	}
	return nil
}

// InsertSigningOrderRequestOutbox inserts the signing order request into the outbox.
func (pg *Postgres) InsertSigningOrderRequestOutbox(ctx context.Context, requestID, orderID, itemID uuid.UUID, signingOrderRequest SigningOrderRequest) error {
	message, err := json.Marshal(signingOrderRequest)
	if err != nil {
		return fmt.Errorf("error marshalling signing order request: %w", err)
	}

	const q = `INSERT INTO signing_order_request_outbox (request_id, order_id, item_id, message_data) VALUES ($1, $2, $3, $4)`
	if _, err := pg.ExecContext(ctx, q, requestID, orderID, itemID, message); err != nil {
		return fmt.Errorf("error inserting order request outbox row: %w", err)
	}

	return nil
}

// DeleteSigningOrderRequestOutboxByOrderTx performs a hard delete of all signing order request outbox
// messages for a given orderID.
func (pg *Postgres) DeleteSigningOrderRequestOutboxByOrderTx(ctx context.Context, tx *sqlx.Tx, orderID uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `delete from signing_order_request_outbox where order_id = $1`, orderID)
	if err != nil {
		return fmt.Errorf("error deleting signing order request outbox: %w", err)
	}
	return nil
}

// SendSigningRequest sends batches of signing order requests to kafka for signing.
// Failed messages are logged and can be manually retried.
// Default batch size 10.
func (pg *Postgres) SendSigningRequest(ctx context.Context, signingRequestWriter SigningRequestWriter) error {
	_, tx, rollback, commit, err := datastore.GetTx(ctx, pg)
	if err != nil {
		return fmt.Errorf("error send signing request could not begin tx: %w", err)
	}
	defer rollback()

	var soro []SigningOrderRequestOutbox
	err = tx.SelectContext(ctx, &soro, `select request_id, order_id, item_id, message_data from signing_order_request_outbox
													where submitted_at is null order by created_at asc
													for update skip locked limit $1`, signingRequestBatchSize)
	if err != nil {
		return fmt.Errorf("error could not get signing order request outbox: %w", err)
	}

	if len(soro) == 0 {
		return nil
	}

	// If there is an error writing messages to kafka we need to log the failed messages here instead of returning
	// to the job runner, we can then update the messages as processed and continue to the next batch rather than
	// retrying, these errors are likely not transient and need checked before retry.
	go func() {
		switch err := signingRequestWriter.WriteMessages(ctx, soro).(type) {
		case nil:
		case kafka.WriteErrors:
			for i := range soro {
				if err[i] != nil {
					logging.FromContext(ctx).Err(err[i]).
						Interface("message", soro[i]).
						Msg("error writing outbox message")
					sentry.CaptureException(err)
				}
			}
		default:
			logging.FromContext(ctx).Err(err).
				Interface("messages", soro).
				Msg("error writing outbox messages")
			sentry.CaptureException(err)
		}
	}()

	soroIDs := make([]uuid.UUID, len(soro))
	for i := 0; i < len(soroIDs); i++ {
		soroIDs[i] = soro[i].RequestID
	}

	qry, args, err := sqlx.In(`update signing_order_request_outbox
										set submitted_at = now() where request_id IN (?)`, soroIDs)
	if err != nil {
		return fmt.Errorf("error creating sql update statement: %w", err)
	}

	result, err := tx.ExecContext(ctx, pg.Rebind(qry), args...)
	if err != nil {
		return fmt.Errorf("error updating outbox message: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error getting updated outbox message rows: %w", err)
	}

	if rows != int64(len(soroIDs)) {
		return fmt.Errorf("error updating rows expected %d got %d", len(soroIDs), rows)
	}

	err = commit()
	if err != nil {
		return fmt.Errorf("error committing signing order request outbox: %w", err)
	}

	return nil
}

// InsertSignedOrderCredentialsTx inserts a signed order request. It handles both TimeLimitedV2Creds and
// SingleUse credentials. All SigningOrder's in the SigningOrderResult must be successful to persist the overall result.
func (pg *Postgres) InsertSignedOrderCredentialsTx(ctx context.Context, tx *sqlx.Tx, signedOrderResult *SigningOrderResult) error {
	var requestID = signedOrderResult.RequestID
	if len(signedOrderResult.Data) == 0 {
		return fmt.Errorf("error no signing order result is empty for requestID %s",
			signedOrderResult.RequestID)
	}

	for _, so := range signedOrderResult.Data {

		var metadata Metadata
		err := json.Unmarshal(so.AssociatedData, &metadata)
		if err != nil {
			return fmt.Errorf("error desearializing associated data for requestID %s: %w",
				signedOrderResult.RequestID, err)
		}

		if so.Status != SignedOrderStatusOk {
			return fmt.Errorf("error signing order creds for orderID %s itemID %s issuerID %s status %s",
				metadata.OrderID, metadata.ItemID, metadata.IssuerID, so.Status.String())
		}

		blindedCreds := jsonutils.JSONStringArray(so.BlindedTokens)
		if len(blindedCreds) == 0 {
			return fmt.Errorf("error blinded tokens is empty order creds orderID %s itemID %s: %w",
				metadata.OrderID, metadata.ItemID, err)
		}

		signedTokens := jsonutils.JSONStringArray(so.SignedTokens)
		if len(signedTokens) == 0 {
			return fmt.Errorf("error signed tokens is empty order creds orderID %s itemID %s: %w",
				metadata.OrderID, metadata.ItemID, err)
		}

		switch metadata.CredentialType {

		case singleUse:

			cred := &OrderCreds{
				ID:           metadata.ItemID,
				OrderID:      metadata.OrderID,
				IssuerID:     metadata.IssuerID,
				BlindedCreds: blindedCreds,
				SignedCreds:  &signedTokens,
				BatchProof:   ptr.FromString(so.Proof),
				PublicKey:    ptr.FromString(so.PublicKey),
			}

			err = pg.InsertOrderCredsTx(ctx, tx, cred)
			if err != nil {
				return fmt.Errorf("error inserting single use order credential orderID %s itemID %s: %w",
					metadata.OrderID, metadata.ItemID, err)
			}

		case timeLimitedV2:
			if so.ValidTo.Value() == nil {
				return fmt.Errorf("error validTo for order creds orderID %s itemID %s is null: %w",
					metadata.OrderID, metadata.ItemID, err)
			}

			validTo, err := time.Parse(time.RFC3339, *so.ValidTo.Value())
			if err != nil {
				return fmt.Errorf("error parsing validTo for order creds orderID %s itemID %s: %w",
					metadata.OrderID, metadata.ItemID, err)
			}

			if so.ValidFrom.Value() == nil {
				return fmt.Errorf("error validFrom for order creds orderID %s itemID %s is null: %w",
					metadata.OrderID, metadata.ItemID, err)
			}

			validFrom, err := time.Parse(time.RFC3339, *so.ValidFrom.Value())
			if err != nil {
				return fmt.Errorf("error parsing validFrom for order creds orderID %s itemID %s: %w",
					metadata.OrderID, metadata.ItemID, err)
			}

			o, err := pg.GetOrder(metadata.OrderID)
			if err != nil {
				return fmt.Errorf("failed to get the order %s: %w", metadata.OrderID, err)
			}
			if o.ExpiresAt == nil || validFrom.After(*o.ExpiresAt) {
				// filter out creds after the order expires
				continue
			}

			cred := TimeAwareSubIssuedCreds{
				ItemID:       metadata.ItemID,
				OrderID:      metadata.OrderID,
				IssuerID:     metadata.IssuerID,
				BlindedCreds: blindedCreds,
				SignedCreds:  signedTokens,
				BatchProof:   so.Proof,
				PublicKey:    so.PublicKey,
				ValidTo:      validTo,
				ValidFrom:    validFrom,
				RequestID:    requestID,
			}

			err = pg.InsertTimeLimitedV2OrderCredsTx(ctx, tx, cred)
			if err != nil {
				return fmt.Errorf("error inserting time limited order credential orderID %s itemID %s: %w",
					metadata.OrderID, metadata.ItemID, err)
			}

		default:
			return fmt.Errorf("error unknown credential type %s for order credential orderID %s itemID %s",
				metadata.CredentialType, metadata.OrderID, metadata.ItemID)
		}
	}

	return nil
}

// AppendOrderMetadataInt64 appends the key and int64 value to an order's metadata.
func (pg *Postgres) AppendOrderMetadataInt64(ctx context.Context, orderID *uuid.UUID, key string, value int64) error {
	_, tx, rollback, commit, err := datastore.GetTx(ctx, pg)
	if err != nil {
		return err
	}
	defer rollback()

	if err := pg.orderRepo.AppendMetadataInt64(ctx, tx, *orderID, key, value); err != nil {
		return fmt.Errorf("error updating order metadata %s: %w", orderID, err)
	}

	return commit()
}

// AppendOrderMetadataInt appends the key and int value to an order's metadata.
func (pg *Postgres) AppendOrderMetadataInt(ctx context.Context, orderID *uuid.UUID, key string, value int) error {
	_, tx, rollback, commit, err := datastore.GetTx(ctx, pg)
	if err != nil {
		return err
	}
	defer rollback()

	if err := pg.orderRepo.AppendMetadataInt(ctx, tx, *orderID, key, value); err != nil {
		return fmt.Errorf("error updating order metadata %s: %w", orderID, err)
	}

	return commit()
}

// AppendOrderMetadata appends the key and string value to an order's metadata.
func (pg *Postgres) AppendOrderMetadata(ctx context.Context, orderID *uuid.UUID, key, value string) error {
	_, tx, rollback, commit, err := datastore.GetTx(ctx, pg)
	if err != nil {
		return err
	}
	defer rollback()

	if err := pg.orderRepo.AppendMetadata(ctx, tx, *orderID, key, value); err != nil {
		return fmt.Errorf("error updating order metadata %s: %w", orderID, err)
	}

	return commit()
}

// SetOrderPaid sets status to paid for the order, updates last paid and expiration.
func (pg *Postgres) SetOrderPaid(ctx context.Context, orderID *uuid.UUID) error {
	_, tx, rollback, commit, err := datastore.GetTx(ctx, pg)
	if err != nil {
		return fmt.Errorf("failed to get db transaction: %w", err)
	}
	defer rollback()

	if err := pg.orderRepo.SetStatus(ctx, tx, *orderID, OrderStatusPaid); err != nil {
		return fmt.Errorf("error updating order %s: %w", orderID, err)
	}

	if err := pg.recordOrderPayment(ctx, tx, *orderID, time.Now()); err != nil {
		return fmt.Errorf("failed to record order payment: %w", err)
	}

	if err := pg.updateOrderExpiresAt(ctx, tx, *orderID); err != nil {
		return fmt.Errorf("failed to set order expires_at: %w", err)
	}

	return commit()
}

func (pg *Postgres) recordOrderPayment(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
	if err := pg.orderPayHistory.Insert(ctx, dbi, id, when); err != nil {
		return err
	}

	return pg.orderRepo.SetLastPaidAt(ctx, dbi, id, when)
}

func (pg *Postgres) updateOrderExpiresAt(ctx context.Context, dbi sqlx.ExtContext, orderID uuid.UUID) error {
	expiresAt, err := pg.orderRepo.GetExpiresAtAfterISOPeriod(ctx, dbi, orderID)
	if err != nil {
		return fmt.Errorf("unable to get order time bounds: %w", err)
	}

	return pg.orderRepo.SetExpiresAt(ctx, dbi, orderID, expiresAt)
}
