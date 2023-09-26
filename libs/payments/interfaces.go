package payments

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/service/kms"
)
// wrappedKMSClient defines the characteristics for KMS methods that we'll be using.
type wrappedKMSClient interface {
	Sign(ctx context.Context, params *kms.SignInput, optFns ...func(*kms.Options)) (*kms.SignOutput, error)
	Verify(ctx context.Context, params *kms.VerifyInput, optFns ...func(*kms.Options)) (*kms.VerifyOutput, error)
	GetPublicKey(ctx context.Context, params *kms.GetPublicKeyInput, optFns ...func(*kms.Options)) (*kms.GetPublicKeyOutput, error)
}
