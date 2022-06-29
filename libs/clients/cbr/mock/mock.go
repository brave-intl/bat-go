// Code generated by MockGen. DO NOT EDIT.
// Source: ./libs/clients/cbr/client.go

// Package mock_cbr is a generated GoMock package.
package mock_cbr

import (
	context "context"
	reflect "reflect"

	cbr "github.com/brave-intl/bat-go/libs/clients/cbr"
	gomock "github.com/golang/mock/gomock"
)

// MockClient is a mock of Client interface.
type MockClient struct {
	ctrl     *gomock.Controller
	recorder *MockClientMockRecorder
}

// MockClientMockRecorder is the mock recorder for MockClient.
type MockClientMockRecorder struct {
	mock *MockClient
}

// NewMockClient creates a new mock instance.
func NewMockClient(ctrl *gomock.Controller) *MockClient {
	mock := &MockClient{ctrl: ctrl}
	mock.recorder = &MockClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockClient) EXPECT() *MockClientMockRecorder {
	return m.recorder
}

// CreateIssuer mocks base method.
func (m *MockClient) CreateIssuer(ctx context.Context, issuer string, maxTokens int) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateIssuer", ctx, issuer, maxTokens)
	ret0, _ := ret[0].(error)
	return ret0
}

// CreateIssuer indicates an expected call of CreateIssuer.
func (mr *MockClientMockRecorder) CreateIssuer(ctx, issuer, maxTokens interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateIssuer", reflect.TypeOf((*MockClient)(nil).CreateIssuer), ctx, issuer, maxTokens)
}

// CreateIssuerV3 mocks base method.
func (m *MockClient) CreateIssuerV3(ctx context.Context, createIssuerV3 cbr.CreateIssuerV3) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateIssuerV3", ctx, createIssuerV3)
	ret0, _ := ret[0].(error)
	return ret0
}

// CreateIssuerV3 indicates an expected call of CreateIssuerV3.
func (mr *MockClientMockRecorder) CreateIssuerV3(ctx, createIssuerV3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateIssuerV3", reflect.TypeOf((*MockClient)(nil).CreateIssuerV3), ctx, createIssuerV3)
}

// GetIssuer mocks base method.
func (m *MockClient) GetIssuer(ctx context.Context, issuer string) (*cbr.IssuerResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetIssuer", ctx, issuer)
	ret0, _ := ret[0].(*cbr.IssuerResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetIssuer indicates an expected call of GetIssuer.
func (mr *MockClientMockRecorder) GetIssuer(ctx, issuer interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetIssuer", reflect.TypeOf((*MockClient)(nil).GetIssuer), ctx, issuer)
}

// RedeemCredential mocks base method.
func (m *MockClient) RedeemCredential(ctx context.Context, issuer, preimage, signature, payload string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RedeemCredential", ctx, issuer, preimage, signature, payload)
	ret0, _ := ret[0].(error)
	return ret0
}

// RedeemCredential indicates an expected call of RedeemCredential.
func (mr *MockClientMockRecorder) RedeemCredential(ctx, issuer, preimage, signature, payload interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RedeemCredential", reflect.TypeOf((*MockClient)(nil).RedeemCredential), ctx, issuer, preimage, signature, payload)
}

// RedeemCredentials mocks base method.
func (m *MockClient) RedeemCredentials(ctx context.Context, credentials []cbr.CredentialRedemption, payload string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RedeemCredentials", ctx, credentials, payload)
	ret0, _ := ret[0].(error)
	return ret0
}

// RedeemCredentials indicates an expected call of RedeemCredentials.
func (mr *MockClientMockRecorder) RedeemCredentials(ctx, credentials, payload interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RedeemCredentials", reflect.TypeOf((*MockClient)(nil).RedeemCredentials), ctx, credentials, payload)
}

// SignCredentials mocks base method.
func (m *MockClient) SignCredentials(ctx context.Context, issuer string, creds []string) (*cbr.CredentialsIssueResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SignCredentials", ctx, issuer, creds)
	ret0, _ := ret[0].(*cbr.CredentialsIssueResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// SignCredentials indicates an expected call of SignCredentials.
func (mr *MockClientMockRecorder) SignCredentials(ctx, issuer, creds interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SignCredentials", reflect.TypeOf((*MockClient)(nil).SignCredentials), ctx, issuer, creds)
}
