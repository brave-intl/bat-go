package payments

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/qldb"
	"github.com/awslabs/amazon-qldb-driver-go/v3/qldbdriver"
)

// wrappedQldbDriverAPI defines the API for QLDB methods that we'll be using.
type wrappedQldbDriverAPI interface {
	Execute(ctx context.Context, fn func(txn qldbdriver.Transaction) (interface{}, error)) (interface{}, error)
	Shutdown(ctx context.Context)
}

type wrappedQldbSDKClient interface {
	New() *wrappedQldbSDKClient
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
