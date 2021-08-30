package payments

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/brave-intl/bat-go/datastore/grantserver"
	"github.com/brave-intl/bat-go/payments/pb"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

var (
	// PreparedStatus - status for prepared state
	PreparedStatus = "prepared"
)

// Datastore - interface describing the datastore
type Datastore interface {
	grantserver.Datastore
	// PrepareBatchedTXs - prepare a set of transactions
	PrepareBatchedTXs(context.Context, pb.Custodian, []*pb.Transaction) (*string, error)
	// RecordAuthorization - record authorization
	RecordAuthorization(context.Context, *Authorization, string) error
	// SubmitBatchedTXs - submit the batch
	SubmitBatchedTXs(context.Context, string) error
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
			base: &Postgres{*pg}, instanceName: "payments_datastore",
		}, err
	}
	return nil, err
}

func bulkInsert(query string, params [][]interface{}) (string, []interface{}, error) {
	if query == "" {
		return "", nil, fmt.Errorf("bulkInsert failed, needs a query to execute")
	}
	if len(params) < 1 {
		return "", nil, fmt.Errorf("bulkInsert failed, needs more than one set of params")
	}

	p := []interface{}{}
	for i := 0; i < len(params); i++ {
		// put all our params together
		p = append(p, params[i]...)

		numFields := len(params[i]) // the number of fields you are inserting
		n := i * numFields

		query += `(`
		for j := 0; j < numFields; j++ {
			query += `$` + strconv.Itoa(n+j+1) + `,`
		}
		query = query[:len(query)-1] + `),`
	}
	query = query[:len(query)-1] // remove the trailing comma
	return query, p, nil
}

// Authorization - representation of an authorization in QLDB
type Authorization struct {
	ID        *uuid.UUID        // id for the authorization
	DocID     *uuid.UUID        // document id in QLDB
	PublicKey string            // hex encoded public key
	Signature string            // hex encoded signature (maybe over the document id?)
	Meta      map[string]string // extra context
}

// transactionData - db repr for pb.Transaction
/*
type transactionData struct {
	BatchID            uuid.UUID       `db:"batch_id"`
	TransactionID      uuid.UUID       `db:"tx_id"`
	Destination        *string         `db:"destination"`
	Origin             *string         `db:"origin"`
	Currency           *string         `db:"currency"`
	ApproximateValue   decimal.Decimal `db:"approximate_value"`
	CreatedAt          *time.Time      `db:"created_at"`
	UpdatedAt          *time.Time      `db:"updated_at"`
	Status             *string         `db:"status"`
	SignedTXCiphertext []byte          `db:"signed_tx_ciphertext"`
}
*/

// SubmitBatchedTXs - Submit the batched transaction
func (pg *Postgres) SubmitBatchedTXs(ctx context.Context, documentID string) error {
	return errorutils.ErrNotImplemented
}

// RecordAuthorization - record a successful authorization on a prepared batch
func (pg *Postgres) RecordAuthorization(ctx context.Context, auth *Authorization, docID string) error {
	return errorutils.ErrNotImplemented
}

// PrepareBatchedTXs - record new transactions in QLDB in initialized state.
// Returns Document ID from QLDB
func (pg *Postgres) PrepareBatchedTXs(ctx context.Context, custodian pb.Custodian, txs []*pb.Transaction) (*string, error) {
	// insert all txs into postgres for this batch
	batchID := uuid.New()

	txData := [][]interface{}{}
	for i := range txs {
		//convert amount to decimal
		amount, err := decimal.NewFromString(txs[i].Amount)
		if err != nil {
			return nil, fmt.Errorf("failed to convert amount to decimal: %w", err)
		}
		// append transaction data
		txData = append(txData, []interface{}{
			batchID, &txs[i].Destination, &txs[i].Origin, &txs[i].Currency, amount, &PreparedStatus,
		})
	}

	q := `
		insert into transactions
			(batch_id, destination, origin, currency, approximate_value, status)
		values `

	query, values, err := bulkInsert(q, txData)
	if err != nil {
		return nil, fmt.Errorf("failed to create insert query: %w", err)
	}

	// perform insert of all transactions provided

	// create tx
	tx, err := pg.RawDB().BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to start db tx: %w", err)
	}
	defer pg.RollbackTx(tx)

	result, err := tx.ExecContext(ctx, query, values...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute insertion; %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if rowsAffected == 0 || err != nil {
		return nil, errors.New("no rows inserted")
	}

	// get the qldb session from context
	// TODO: get this from qldb
	documentID := uuid.New().String()

	// commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit db tx: %w", err)
	}

	// insert into qldb the set of transactions for the given custodian
	return &documentID, nil
}

// RecordAuthorization - Record an authorization for a qldb document, the submission will
// be in charge of implementing the logic to bound the authorizations, this merely applies
// the rubber stamp to the document in qldb stating that this record was authorized
func RecordAuthorization(ctx context.Context, auth *Authorization, docID *uuid.UUID) error {
	return errorutils.ErrNotImplemented
}

// RetrieveTransactionsByID - Record an authorization for a qldb document.  This will be used
// in submission to pull the transactions and authorizations from the qldb document
func RetrieveTransactionsByID(ctx context.Context, docID string) ([]Authorization, []*pb.Transaction, error) {
	return nil, nil, errorutils.ErrNotImplemented
}

// RecordStateChange - Record the state change for this particular document in qldb
func RecordStateChange(ctx context.Context, docID string, state pb.State) error {
	return errorutils.ErrNotImplemented
}
