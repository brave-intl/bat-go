package payments

import (
	"context"

	"github.com/google/uuid"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/qldb"
	"github.com/awslabs/amazon-qldb-driver-go/v3/qldbdriver"
)

// idempotentObject is anything that can generate an idempotency key.
type idempotentObject interface {
	getIdempotencyKey() *uuid.UUID
	generateIdempotencyKey(uuid.UUID) uuid.UUID
}

// TxStateMachine is anything that be progressed through states by the
// Drive function.
type TxStateMachine interface {
	setTransaction(*Transaction)
	setService(*Service)
	GetState() TransactionState
	GetTransaction() *Transaction
	GetService() *Service
	GetTransactionID() *uuid.UUID
	GenerateTransactionID(namespace uuid.UUID) (*uuid.UUID, error)
	Prepare(context.Context) (*Transaction, error)
	Authorize(context.Context) (*Transaction, error)
	Pay(context.Context) (*Transaction, error)
	Fail(context.Context) (*Transaction, error)
}

// wrappedQldbDriverAPI defines the API for QLDB methods that we'll be using.
type wrappedQldbDriverAPI interface {
	Execute(ctx context.Context, fn func(txn qldbdriver.Transaction) (interface{}, error)) (interface{}, error)
	Shutdown(ctx context.Context)
}

type wrappedQldbSdkClient interface {
	New() *wrappedQldbSdkClient
	GetDigest(ctx context.Context, params *qldb.GetDigestInput, optFns ...func(*qldb.Options)) (*qldb.GetDigestOutput, error)
	GetRevision(ctx context.Context, params *qldb.GetRevisionInput, optFns ...func(*qldb.Options)) (*qldb.GetRevisionOutput, error)
}

// wrappedQldbTxnAPI defines the API for QLDB methods that we'll be using.
type wrappedQldbTxnAPI interface {
	Execute(statement string, parameters ...interface{}) (qldbdriver.Result, error)
	Abort() error
	BufferResult(qldbdriver.Result) (qldbdriver.BufferedResult, error)
	ID() string
}

// wrappedKMSClient defines the characteristics for KMS methods that we'll be using.
type wrappedKMSClient interface {
	Sign(ctx context.Context, params *kms.SignInput, optFns ...func(*kms.Options)) (*kms.SignOutput, error)
	Verify(ctx context.Context, params *kms.VerifyInput, optFns ...func(*kms.Options)) (*kms.VerifyOutput, error)
	GetPublicKey(ctx context.Context, params *kms.GetPublicKeyInput, optFns ...func(*kms.Options)) (*kms.GetPublicKeyOutput, error)
}
