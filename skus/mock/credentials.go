// Code generated by MockGen. DO NOT EDIT.
// Source: ./skus/credentials.go

// Package mockskus is a generated GoMock package.
package mockskus

import (
	context "context"
	reflect "reflect"

	skus "github.com/brave-intl/bat-go/skus"
	gomock "github.com/golang/mock/gomock"
	uuid "github.com/satori/go.uuid"
)

// MockOrderWorker is a mock of OrderWorker interface.
type MockOrderWorker struct {
	ctrl     *gomock.Controller
	recorder *MockOrderWorkerMockRecorder
}

// MockOrderWorkerMockRecorder is the mock recorder for MockOrderWorker.
type MockOrderWorkerMockRecorder struct {
	mock *MockOrderWorker
}

// NewMockOrderWorker creates a new mock instance.
func NewMockOrderWorker(ctrl *gomock.Controller) *MockOrderWorker {
	mock := &MockOrderWorker{ctrl: ctrl}
	mock.recorder = &MockOrderWorkerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockOrderWorker) EXPECT() *MockOrderWorkerMockRecorder {
	return m.recorder
}

// SignOrderCreds mocks base method.
func (m *MockOrderWorker) SignOrderCreds(ctx context.Context, orderID uuid.UUID, issuer skus.Issuer, blindedCreds []string) (*skus.OrderCreds, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SignOrderCreds", ctx, orderID, issuer, blindedCreds)
	ret0, _ := ret[0].(*skus.OrderCreds)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// SignOrderCreds indicates an expected call of SignOrderCreds.
func (mr *MockOrderWorkerMockRecorder) SignOrderCreds(ctx, orderID, issuer, blindedCreds interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SignOrderCreds", reflect.TypeOf((*MockOrderWorker)(nil).SignOrderCreds), ctx, orderID, issuer, blindedCreds)
}

// MockOrderCredentialsWorker is a mock of OrderCredentialsWorker interface.
type MockOrderCredentialsWorker struct {
	ctrl     *gomock.Controller
	recorder *MockOrderCredentialsWorkerMockRecorder
}

// MockOrderCredentialsWorkerMockRecorder is the mock recorder for MockOrderCredentialsWorker.
type MockOrderCredentialsWorkerMockRecorder struct {
	mock *MockOrderCredentialsWorker
}

// NewMockOrderCredentialsWorker creates a new mock instance.
func NewMockOrderCredentialsWorker(ctrl *gomock.Controller) *MockOrderCredentialsWorker {
	mock := &MockOrderCredentialsWorker{ctrl: ctrl}
	mock.recorder = &MockOrderCredentialsWorkerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockOrderCredentialsWorker) EXPECT() *MockOrderCredentialsWorkerMockRecorder {
	return m.recorder
}

// FetchSignedOrderCredentials mocks base method.
func (m *MockOrderCredentialsWorker) FetchSignedOrderCredentials(ctx context.Context) (*skus.SigningOrderResult, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FetchSignedOrderCredentials", ctx)
	ret0, _ := ret[0].(*skus.SigningOrderResult)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// FetchSignedOrderCredentials indicates an expected call of FetchSignedOrderCredentials.
func (mr *MockOrderCredentialsWorkerMockRecorder) FetchSignedOrderCredentials(ctx interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FetchSignedOrderCredentials", reflect.TypeOf((*MockOrderCredentialsWorker)(nil).FetchSignedOrderCredentials), ctx)
}