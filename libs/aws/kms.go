package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/kms"
)

// KMSCreateKeyAPI defines the interface for the CreateKey function.
// We use this interface to test the function using a mocked service.
type KMSCreateKeyAPI interface {
	CreateKey(ctx context.Context,
		params *kms.CreateKeyInput,
		optFns ...func(*kms.Options)) (*kms.CreateKeyOutput, error)
}

// MakeKey creates an AWS Key Management Service (AWS KMS) key (KMS key).
// Inputs:
//
//	c is the context of the method call, which includes the AWS Region.
//	api is the interface that defines the method call.
//	input defines the input arguments to the service call.
//
// Output:
//
//	If success, a CreateKeyOutput object containing the result of the service call and nil.
//	Otherwise, nil and an error from the call to CreateKey.
func MakeKey(c context.Context, api KMSCreateKeyAPI, input *kms.CreateKeyInput) (*kms.CreateKeyOutput, error) {
	return api.CreateKey(c, input)
}
