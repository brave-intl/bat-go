// Code generated by MockGen. DO NOT EDIT.
// Source: ./promotion/drain.go

// Package promotion is a generated GoMock package.
package promotion

import (
	context "context"
	reflect "reflect"

	cbr "github.com/brave-intl/bat-go/utils/clients/cbr"
	wallet "github.com/brave-intl/bat-go/utils/wallet"
	gomock "github.com/golang/mock/gomock"
	uuid "github.com/satori/go.uuid"
	decimal "github.com/shopspring/decimal"
)

// MockDrainWorker is a mock of DrainWorker interface.
type MockDrainWorker struct {
	ctrl     *gomock.Controller
	recorder *MockDrainWorkerMockRecorder
}

// MockDrainWorkerMockRecorder is the mock recorder for MockDrainWorker.
type MockDrainWorkerMockRecorder struct {
	mock *MockDrainWorker
}

// NewMockDrainWorker creates a new mock instance.
func NewMockDrainWorker(ctrl *gomock.Controller) *MockDrainWorker {
	mock := &MockDrainWorker{ctrl: ctrl}
	mock.recorder = &MockDrainWorkerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockDrainWorker) EXPECT() *MockDrainWorkerMockRecorder {
	return m.recorder
}

// RedeemAndTransferFunds mocks base method.
func (m *MockDrainWorker) RedeemAndTransferFunds(ctx context.Context, credentials []cbr.CredentialRedemption, walletID uuid.UUID, total decimal.Decimal) (*wallet.TransactionInfo, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RedeemAndTransferFunds", ctx, credentials, walletID, total)
	ret0, _ := ret[0].(*wallet.TransactionInfo)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// RedeemAndTransferFunds indicates an expected call of RedeemAndTransferFunds.
func (mr *MockDrainWorkerMockRecorder) RedeemAndTransferFunds(ctx, credentials, walletID, total interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RedeemAndTransferFunds", reflect.TypeOf((*MockDrainWorker)(nil).RedeemAndTransferFunds), ctx, credentials, walletID, total)
}

// MockDrainRetryWorker is a mock of DrainRetryWorker interface.
type MockDrainRetryWorker struct {
	ctrl     *gomock.Controller
	recorder *MockDrainRetryWorkerMockRecorder
}

// MockDrainRetryWorkerMockRecorder is the mock recorder for MockDrainRetryWorker.
type MockDrainRetryWorkerMockRecorder struct {
	mock *MockDrainRetryWorker
}

// NewMockDrainRetryWorker creates a new mock instance.
func NewMockDrainRetryWorker(ctrl *gomock.Controller) *MockDrainRetryWorker {
	mock := &MockDrainRetryWorker{ctrl: ctrl}
	mock.recorder = &MockDrainRetryWorkerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockDrainRetryWorker) EXPECT() *MockDrainRetryWorkerMockRecorder {
	return m.recorder
}

// FetchAdminAttestationWalletID mocks base method.
func (m *MockDrainRetryWorker) FetchAdminAttestationWalletID(ctx context.Context) (*uuid.UUID, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FetchAdminAttestationWalletID", ctx)
	ret0, _ := ret[0].(*uuid.UUID)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// FetchAdminAttestationWalletID indicates an expected call of FetchAdminAttestationWalletID.
func (mr *MockDrainRetryWorkerMockRecorder) FetchAdminAttestationWalletID(ctx interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FetchAdminAttestationWalletID", reflect.TypeOf((*MockDrainRetryWorker)(nil).FetchAdminAttestationWalletID), ctx)
}

// MockMintWorker is a mock of MintWorker interface.
type MockMintWorker struct {
	ctrl     *gomock.Controller
	recorder *MockMintWorkerMockRecorder
}

// MockMintWorkerMockRecorder is the mock recorder for MockMintWorker.
type MockMintWorkerMockRecorder struct {
	mock *MockMintWorker
}

// NewMockMintWorker creates a new mock instance.
func NewMockMintWorker(ctrl *gomock.Controller) *MockMintWorker {
	mock := &MockMintWorker{ctrl: ctrl}
	mock.recorder = &MockMintWorkerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockMintWorker) EXPECT() *MockMintWorkerMockRecorder {
	return m.recorder
}

// MintGrant mocks base method.
func (m *MockMintWorker) MintGrant(ctx context.Context, walletID uuid.UUID, total decimal.Decimal, promoIDs ...uuid.UUID) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, walletID, total}
	for _, a := range promoIDs {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "MintGrant", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// MintGrant indicates an expected call of MintGrant.
func (mr *MockMintWorkerMockRecorder) MintGrant(ctx, walletID, total interface{}, promoIDs ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, walletID, total}, promoIDs...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "MintGrant", reflect.TypeOf((*MockMintWorker)(nil).MintGrant), varargs...)
}

// MockBatchTransferWorker is a mock of BatchTransferWorker interface.
type MockBatchTransferWorker struct {
	ctrl     *gomock.Controller
	recorder *MockBatchTransferWorkerMockRecorder
}

// MockBatchTransferWorkerMockRecorder is the mock recorder for MockBatchTransferWorker.
type MockBatchTransferWorkerMockRecorder struct {
	mock *MockBatchTransferWorker
}

// NewMockBatchTransferWorker creates a new mock instance.
func NewMockBatchTransferWorker(ctrl *gomock.Controller) *MockBatchTransferWorker {
	mock := &MockBatchTransferWorker{ctrl: ctrl}
	mock.recorder = &MockBatchTransferWorkerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockBatchTransferWorker) EXPECT() *MockBatchTransferWorkerMockRecorder {
	return m.recorder
}

// SubmitBatchTransfer mocks base method.
func (m *MockBatchTransferWorker) SubmitBatchTransfer(ctx context.Context, batchID *uuid.UUID) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SubmitBatchTransfer", ctx, batchID)
	ret0, _ := ret[0].(error)
	return ret0
}

// SubmitBatchTransfer indicates an expected call of SubmitBatchTransfer.
func (mr *MockBatchTransferWorkerMockRecorder) SubmitBatchTransfer(ctx, batchID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SubmitBatchTransfer", reflect.TypeOf((*MockBatchTransferWorker)(nil).SubmitBatchTransfer), ctx, batchID)
}

// MockGeminiTxnStatusWorker is a mock of GeminiTxnStatusWorker interface.
type MockGeminiTxnStatusWorker struct {
	ctrl     *gomock.Controller
	recorder *MockGeminiTxnStatusWorkerMockRecorder
}

// MockGeminiTxnStatusWorkerMockRecorder is the mock recorder for MockGeminiTxnStatusWorker.
type MockGeminiTxnStatusWorkerMockRecorder struct {
	mock *MockGeminiTxnStatusWorker
}

// NewMockGeminiTxnStatusWorker creates a new mock instance.
func NewMockGeminiTxnStatusWorker(ctrl *gomock.Controller) *MockGeminiTxnStatusWorker {
	mock := &MockGeminiTxnStatusWorker{ctrl: ctrl}
	mock.recorder = &MockGeminiTxnStatusWorkerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockGeminiTxnStatusWorker) EXPECT() *MockGeminiTxnStatusWorkerMockRecorder {
	return m.recorder
}

// GetGeminiTxnStatus mocks base method.
func (m *MockGeminiTxnStatusWorker) GetGeminiTxnStatus(ctx context.Context, transactionID string) (*string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetGeminiTxnStatus", ctx, transactionID)
	ret0, _ := ret[0].(*string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetGeminiTxnStatus indicates an expected call of GetGeminiTxnStatus.
func (mr *MockGeminiTxnStatusWorkerMockRecorder) GetGeminiTxnStatus(ctx, transactionID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetGeminiTxnStatus", reflect.TypeOf((*MockGeminiTxnStatusWorker)(nil).GetGeminiTxnStatus), ctx, transactionID)
}
