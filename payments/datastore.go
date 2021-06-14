package payments

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/service/qldbsession"
	"github.com/brave-intl/bat-go/payments/pb"
	appctx "github.com/brave-intl/bat-go/utils/context"
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

// getQLDBSessionFromContext - get qldb session from the context
func getQLDBSessionFromContext(ctx context.Context) (*qldbsession.QLDBSession, error) {
	session := ctx.Value(appctx.QLDBSessionCTXKey)
	if session == nil {
		// value not on context
		return nil, appctx.ErrNotInContext
	}
	if s, ok := session.(*qldbsession.QLDBSession); ok {
		return s, nil
	}
	// value not a string
	return nil, appctx.ErrValueWrongType
}

// InitializeBatchedTXs - record new transactions in QLDB in initialized state.
// Returns Document ID from QLDB
func InitializeBatchedTXs(ctx context.Context, custodian pb.Custodian, txs []*pb.Transaction) (*uuid.UUID, error) {
	// get the qldb session from context
	_, err := getQLDBSessionFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get qldb session from context: %w", err)
	}
	// perform insert of all transactions provided

	// insert into qldb the set of transactions for the given custodian
	return nil, errorutils.ErrNotYetImplemented
}

// RecordAuthorization - Record an authorization for a qldb document, the submission will
// be in charge of implementing the logic to bound the authorizations, this merely applies
// the rubber stamp to the document in qldb stating that this record was authorized
func RecordAuthorization(ctx context.Context, auth *Authorization, docID *uuid.UUID) error {
	return errorutils.ErrNotYetImplemented
}

// RetrieveTransactionsByID - Record an authorization for a qldb document.  This will be used
// in submission to pull the transactions and authorizations from the qldb document
func RetrieveTransactionsByID(ctx context.Context, docID *uuid.UUID) ([]Authorization, []*pb.Transaction, error) {
	return nil, nil, errorutils.ErrNotYetImplemented
}

// RecordStateChange - Record the state change for this particular document in qldb
func RecordStateChange(ctx context.Context, docID *uuid.UUID, state pb.State) error {
	return errorutils.ErrNotYetImplemented
}
