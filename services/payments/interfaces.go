package payments

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/qldb"
)

type wrappedQldbSDKClient interface {
	GetDigest(ctx context.Context, params *qldb.GetDigestInput, optFns ...func(*qldb.Options)) (*qldb.GetDigestOutput, error)
	GetRevision(ctx context.Context, params *qldb.GetRevisionInput, optFns ...func(*qldb.Options)) (*qldb.GetRevisionOutput, error)
}
