package payments

import (
	"context"

	"github.com/brave-intl/bat-go/payments/pb"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/google/uuid"
)

// Authorization - representation of an authorization in QLDB
type Authorization struct {
	ID        *uuid.UUID        // id for the authorization
	DocID     *uuid.UUID        // document id in QLDB
	PublicKey string            // hex encoded public key
	Signature string            // hex encoded signature (maybe over the document id?)
	Meta      map[string]string // extra context
}

// InitializeBatchedTXs - record new transactions in QLDB in initialized state.
// Returns Document ID from QLDB
func InitializeBatchedTXs(ctx context.Context, custodian pb.Custodian, txs []*pb.Transaction) (*uuid.UUID, error) {
	// get the qldb session from context

	// perform insert of all transactions provided

	// insert into qldb the set of transactions for the given custodian
	return nil, errorutils.ErrNotImplemented
}

// RecordAuthorization - Record an authorization for a qldb document, the submission will
// be in charge of implementing the logic to bound the authorizations, this merely applies
// the rubber stamp to the document in qldb stating that this record was authorized
func RecordAuthorization(ctx context.Context, auth *Authorization, docID *uuid.UUID) error {
	return errorutils.ErrNotImplemented
}

// RetrieveTransactionsByID - Record an authorization for a qldb document.  This will be used
// in submission to pull the transactions and authorizations from the qldb document
func RetrieveTransactionsByID(ctx context.Context, docID *uuid.UUID) ([]Authorization, []*pb.Transaction, error) {
	return nil, nil, errorutils.ErrNotImplemented
}

// RecordStateChange - Record the state change for this particular document in qldb
func RecordStateChange(ctx context.Context, docID *uuid.UUID, state pb.State) error {
	return errorutils.ErrNotImplemented
}
