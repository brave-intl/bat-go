package payments

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/qldb"
	"github.com/awslabs/amazon-qldb-driver-go/v3/qldbdriver"
	"github.com/stretchr/testify/mock"
)

// Unit testing the package code using a Mock Driver
type mockDriver struct {
	mock.Mock
}

// Unit testing the package code using a Mock QLDB SDK
type mockSDKClient struct {
	mock.Mock
}

/* TODO: unused
type mockResult struct {
	mock.Mock
}
*/

type mockKMSClient struct {
	mock.Mock
}

func (m *mockDriver) Execute(ctx context.Context, fn func(txn qldbdriver.Transaction) (interface{}, error)) (interface{}, error) {
	args := m.Called(ctx, fn)
	return args.Get(0), args.Error(1)
}

func (m *mockDriver) Shutdown(ctx context.Context) {
	return
}

/*
TODO: unused

	func (m *mockResult) GetCurrentData() []byte {
		args := m.Called()
		return args.Get(0).([]byte)
	}

	func (m *mockResult) Next(txn qldbdriver.Transaction) bool {
		args := m.Called(txn)
		return args.Get(0).(bool)
	}
*/
func (m *mockSDKClient) New() *wrappedQldbSDKClient {
	args := m.Called()
	return args.Get(0).(*wrappedQldbSDKClient)
}
func (m *mockSDKClient) GetDigest(
	ctx context.Context,
	params *qldb.GetDigestInput,
	optFns ...func(*qldb.Options),
) (*qldb.GetDigestOutput, error) {
	args := m.Called()
	return args.Get(0).(*qldb.GetDigestOutput), args.Error(1)
}

func (m *mockSDKClient) GetRevision(
	ctx context.Context,
	params *qldb.GetRevisionInput,
	optFns ...func(*qldb.Options),
) (*qldb.GetRevisionOutput, error) {
	args := m.Called()
	return args.Get(0).(*qldb.GetRevisionOutput), args.Error(1)
}

func (m *mockKMSClient) Sign(ctx context.Context, params *kms.SignInput, optFns ...func(*kms.Options)) (*kms.SignOutput, error) {
	args := m.Called(ctx, params, optFns)
	return args.Get(0).(*kms.SignOutput), args.Error(1)
}

func (m *mockKMSClient) Verify(ctx context.Context, params *kms.VerifyInput, optFns ...func(*kms.Options)) (*kms.VerifyOutput, error) {
	args := m.Called(ctx, params, optFns)
	return args.Get(0).(*kms.VerifyOutput), args.Error(1)
}

func (m *mockKMSClient) GetPublicKey(ctx context.Context, params *kms.GetPublicKeyInput, optFns ...func(*kms.Options)) (*kms.GetPublicKeyOutput, error) {
	args := m.Called(ctx, params, optFns)
	return args.Get(0).(*kms.GetPublicKeyOutput), args.Error(1)
}
