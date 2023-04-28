package payments

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/qldb"
	"github.com/awslabs/amazon-qldb-driver-go/v3/qldbdriver"
)

type IdempotentObject interface {
	getIdempotencyKey() string
}

type TxStateMachine interface {
	setVersion(int)
	setTransaction(*Transaction)
	setConnection(wrappedQldbDriverAPI)
	Initialized() (TransactionState, error)
	Prepared() (TransactionState, error)
	Authorized() (TransactionState, error)
	Pending() (TransactionState, error)
	Paid() (TransactionState, error)
	Failed() (TransactionState, error)
}

// wrappedQldbDriverAPI defines the API for QLDB methods that we'll be using
type wrappedQldbDriverAPI interface {
	Execute(ctx context.Context, fn func(txn qldbdriver.Transaction) (interface{}, error)) (interface{}, error)
	Shutdown(ctx context.Context)
}

type wrappedQldbSdkClient interface {
	New() *wrappedQldbSdkClient
	GetDigest(
		ctx context.Context,
		params *qldb.GetDigestInput,
		optFns ...func(*qldb.Options),
	) (*qldb.GetDigestOutput, error)
	GetRevision(
		ctx context.Context,
		params *qldb.GetRevisionInput,
		optFns ...func(*qldb.Options),
	) (*qldb.GetRevisionOutput, error)
}

// wrappedQldbTxnAPI defines the API for QLDB methods that we'll be using
type wrappedQldbTxnAPI interface {
	Execute(statement string, parameters ...interface{}) (qldbdriver.Result, error)
	Abort() error
	BufferResult(qldbdriver.Result) (qldbdriver.BufferedResult, error)
	ID() string
}

// wrappedQldbResult defines the Result characteristics for QLDB methods that we'll be using
type wrappedQldbResult interface {
	Next(wrappedQldbTxnAPI) bool
	GetCurrentData() []byte
}

type wrappedKMSClient interface {
	Sign(ctx context.Context, params *kms.SignInput, optFns ...func(*kms.Options)) (*kms.SignOutput, error)
}
