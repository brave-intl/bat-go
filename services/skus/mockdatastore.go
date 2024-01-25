// Code generated by MockGen. DO NOT EDIT.
// Source: ./skus/datastore.go

// Package skus is a generated GoMock package.
package skus

import (
	context "context"
	reflect "reflect"
	time "time"

	inputs "github.com/brave-intl/bat-go/libs/inputs"
	model "github.com/brave-intl/bat-go/services/skus/model"
	v4 "github.com/golang-migrate/migrate/v4"
	gomock "github.com/golang/mock/gomock"
	sqlx "github.com/jmoiron/sqlx"
	go_uuid "github.com/satori/go.uuid"
	decimal "github.com/shopspring/decimal"
)

// MockDatastore is a mock of Datastore interface.
type MockDatastore struct {
	ctrl     *gomock.Controller
	recorder *MockDatastoreMockRecorder
}

// MockDatastoreMockRecorder is the mock recorder for MockDatastore.
type MockDatastoreMockRecorder struct {
	mock *MockDatastore
}

// NewMockDatastore creates a new mock instance.
func NewMockDatastore(ctrl *gomock.Controller) *MockDatastore {
	mock := &MockDatastore{ctrl: ctrl}
	mock.recorder = &MockDatastoreMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockDatastore) EXPECT() *MockDatastoreMockRecorder {
	return m.recorder
}

// AppendOrderMetadata mocks base method.
func (m *MockDatastore) AppendOrderMetadata(arg0 context.Context, arg1 *go_uuid.UUID, arg2, arg3 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AppendOrderMetadata", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(error)
	return ret0
}

// AppendOrderMetadata indicates an expected call of AppendOrderMetadata.
func (mr *MockDatastoreMockRecorder) AppendOrderMetadata(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AppendOrderMetadata", reflect.TypeOf((*MockDatastore)(nil).AppendOrderMetadata), arg0, arg1, arg2, arg3)
}

// AppendOrderMetadataInt mocks base method.
func (m *MockDatastore) AppendOrderMetadataInt(arg0 context.Context, arg1 *go_uuid.UUID, arg2 string, arg3 int) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AppendOrderMetadataInt", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(error)
	return ret0
}

// AppendOrderMetadataInt indicates an expected call of AppendOrderMetadataInt.
func (mr *MockDatastoreMockRecorder) AppendOrderMetadataInt(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AppendOrderMetadataInt", reflect.TypeOf((*MockDatastore)(nil).AppendOrderMetadataInt), arg0, arg1, arg2, arg3)
}

// AppendOrderMetadataInt64 mocks base method.
func (m *MockDatastore) AppendOrderMetadataInt64(arg0 context.Context, arg1 *go_uuid.UUID, arg2 string, arg3 int64) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AppendOrderMetadataInt64", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(error)
	return ret0
}

// AppendOrderMetadataInt64 indicates an expected call of AppendOrderMetadataInt64.
func (mr *MockDatastoreMockRecorder) AppendOrderMetadataInt64(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AppendOrderMetadataInt64", reflect.TypeOf((*MockDatastore)(nil).AppendOrderMetadataInt64), arg0, arg1, arg2, arg3)
}

// AreTimeLimitedV2CredsSubmitted mocks base method.
func (m *MockDatastore) AreTimeLimitedV2CredsSubmitted(ctx context.Context, blindedCreds ...string) (bool, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx}
	for _, a := range blindedCreds {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "AreTimeLimitedV2CredsSubmitted", varargs...)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// AreTimeLimitedV2CredsSubmitted indicates an expected call of AreTimeLimitedV2CredsSubmitted.
func (mr *MockDatastoreMockRecorder) AreTimeLimitedV2CredsSubmitted(ctx interface{}, blindedCreds ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx}, blindedCreds...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AreTimeLimitedV2CredsSubmitted", reflect.TypeOf((*MockDatastore)(nil).AreTimeLimitedV2CredsSubmitted), varargs...)
}

// BeginTx mocks base method.
func (m *MockDatastore) BeginTx() (*sqlx.Tx, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "BeginTx")
	ret0, _ := ret[0].(*sqlx.Tx)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// BeginTx indicates an expected call of BeginTx.
func (mr *MockDatastoreMockRecorder) BeginTx() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "BeginTx", reflect.TypeOf((*MockDatastore)(nil).BeginTx))
}

// CheckExpiredCheckoutSession mocks base method.
func (m *MockDatastore) CheckExpiredCheckoutSession(arg0 go_uuid.UUID) (bool, string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CheckExpiredCheckoutSession", arg0)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(string)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// CheckExpiredCheckoutSession indicates an expected call of CheckExpiredCheckoutSession.
func (mr *MockDatastoreMockRecorder) CheckExpiredCheckoutSession(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CheckExpiredCheckoutSession", reflect.TypeOf((*MockDatastore)(nil).CheckExpiredCheckoutSession), arg0)
}

// CommitVote mocks base method.
func (m *MockDatastore) CommitVote(ctx context.Context, vr VoteRecord, tx *sqlx.Tx) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CommitVote", ctx, vr, tx)
	ret0, _ := ret[0].(error)
	return ret0
}

// CommitVote indicates an expected call of CommitVote.
func (mr *MockDatastoreMockRecorder) CommitVote(ctx, vr, tx interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CommitVote", reflect.TypeOf((*MockDatastore)(nil).CommitVote), ctx, vr, tx)
}

// CreateKey mocks base method.
func (m *MockDatastore) CreateKey(merchant, name, encryptedSecretKey, nonce string) (*Key, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateKey", merchant, name, encryptedSecretKey, nonce)
	ret0, _ := ret[0].(*Key)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateKey indicates an expected call of CreateKey.
func (mr *MockDatastoreMockRecorder) CreateKey(merchant, name, encryptedSecretKey, nonce interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateKey", reflect.TypeOf((*MockDatastore)(nil).CreateKey), merchant, name, encryptedSecretKey, nonce)
}

// CreateOrder mocks base method.
func (m *MockDatastore) CreateOrder(ctx context.Context, dbi sqlx.ExtContext, oreq *model.OrderNew, items []model.OrderItem) (*model.Order, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateOrder", ctx, dbi, oreq, items)
	ret0, _ := ret[0].(*model.Order)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateOrder indicates an expected call of CreateOrder.
func (mr *MockDatastoreMockRecorder) CreateOrder(ctx, dbi, oreq, items interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateOrder", reflect.TypeOf((*MockDatastore)(nil).CreateOrder), ctx, dbi, oreq, items)
}

// CreateTransaction mocks base method.
func (m *MockDatastore) CreateTransaction(orderID go_uuid.UUID, externalTransactionID, status, currency, kind string, amount decimal.Decimal) (*Transaction, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateTransaction", orderID, externalTransactionID, status, currency, kind, amount)
	ret0, _ := ret[0].(*Transaction)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateTransaction indicates an expected call of CreateTransaction.
func (mr *MockDatastoreMockRecorder) CreateTransaction(orderID, externalTransactionID, status, currency, kind, amount interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateTransaction", reflect.TypeOf((*MockDatastore)(nil).CreateTransaction), orderID, externalTransactionID, status, currency, kind, amount)
}

// DeleteKey mocks base method.
func (m *MockDatastore) DeleteKey(id go_uuid.UUID, delaySeconds int) (*Key, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteKey", id, delaySeconds)
	ret0, _ := ret[0].(*Key)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DeleteKey indicates an expected call of DeleteKey.
func (mr *MockDatastoreMockRecorder) DeleteKey(id, delaySeconds interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteKey", reflect.TypeOf((*MockDatastore)(nil).DeleteKey), id, delaySeconds)
}

// DeleteSigningOrderRequestOutboxByOrderTx mocks base method.
func (m *MockDatastore) DeleteSigningOrderRequestOutboxByOrderTx(ctx context.Context, tx *sqlx.Tx, orderID go_uuid.UUID) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteSigningOrderRequestOutboxByOrderTx", ctx, tx, orderID)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteSigningOrderRequestOutboxByOrderTx indicates an expected call of DeleteSigningOrderRequestOutboxByOrderTx.
func (mr *MockDatastoreMockRecorder) DeleteSigningOrderRequestOutboxByOrderTx(ctx, tx, orderID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteSigningOrderRequestOutboxByOrderTx", reflect.TypeOf((*MockDatastore)(nil).DeleteSigningOrderRequestOutboxByOrderTx), ctx, tx, orderID)
}

// DeleteSingleUseOrderCredsByOrderTx mocks base method.
func (m *MockDatastore) DeleteSingleUseOrderCredsByOrderTx(ctx context.Context, tx *sqlx.Tx, orderID go_uuid.UUID, isSigned bool) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteSingleUseOrderCredsByOrderTx", ctx, tx, orderID, isSigned)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteSingleUseOrderCredsByOrderTx indicates an expected call of DeleteSingleUseOrderCredsByOrderTx.
func (mr *MockDatastoreMockRecorder) DeleteSingleUseOrderCredsByOrderTx(ctx, tx, orderID, isSigned interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteSingleUseOrderCredsByOrderTx", reflect.TypeOf((*MockDatastore)(nil).DeleteSingleUseOrderCredsByOrderTx), ctx, tx, orderID, isSigned)
}

// DeleteTimeLimitedV2OrderCredsByOrderTx mocks base method.
func (m *MockDatastore) DeleteTimeLimitedV2OrderCredsByOrderTx(ctx context.Context, tx *sqlx.Tx, orderID go_uuid.UUID, itemIDs ...*go_uuid.UUID) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, tx, orderID}
	for _, a := range itemIDs {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "DeleteTimeLimitedV2OrderCredsByOrderTx", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteTimeLimitedV2OrderCredsByOrderTx indicates an expected call of DeleteTimeLimitedV2OrderCredsByOrderTx.
func (mr *MockDatastoreMockRecorder) DeleteTimeLimitedV2OrderCredsByOrderTx(ctx, tx, orderID interface{}, itemIDs ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, tx, orderID}, itemIDs...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteTimeLimitedV2OrderCredsByOrderTx", reflect.TypeOf((*MockDatastore)(nil).DeleteTimeLimitedV2OrderCredsByOrderTx), varargs...)
}

// ExternalIDExists mocks base method.
func (m *MockDatastore) ExternalIDExists(arg0 context.Context, arg1 string) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ExternalIDExists", arg0, arg1)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ExternalIDExists indicates an expected call of ExternalIDExists.
func (mr *MockDatastoreMockRecorder) ExternalIDExists(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ExternalIDExists", reflect.TypeOf((*MockDatastore)(nil).ExternalIDExists), arg0, arg1)
}

// GetIssuerByPublicKey mocks base method.
func (m *MockDatastore) GetIssuerByPublicKey(publicKey string) (*Issuer, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetIssuerByPublicKey", publicKey)
	ret0, _ := ret[0].(*Issuer)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetIssuerByPublicKey indicates an expected call of GetIssuerByPublicKey.
func (mr *MockDatastoreMockRecorder) GetIssuerByPublicKey(publicKey interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetIssuerByPublicKey", reflect.TypeOf((*MockDatastore)(nil).GetIssuerByPublicKey), publicKey)
}

// GetKey mocks base method.
func (m *MockDatastore) GetKey(id go_uuid.UUID, showExpired bool) (*Key, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetKey", id, showExpired)
	ret0, _ := ret[0].(*Key)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetKey indicates an expected call of GetKey.
func (mr *MockDatastoreMockRecorder) GetKey(id, showExpired interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetKey", reflect.TypeOf((*MockDatastore)(nil).GetKey), id, showExpired)
}

// GetKeysByMerchant mocks base method.
func (m *MockDatastore) GetKeysByMerchant(merchant string, showExpired bool) (*[]Key, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetKeysByMerchant", merchant, showExpired)
	ret0, _ := ret[0].(*[]Key)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetKeysByMerchant indicates an expected call of GetKeysByMerchant.
func (mr *MockDatastoreMockRecorder) GetKeysByMerchant(merchant, showExpired interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetKeysByMerchant", reflect.TypeOf((*MockDatastore)(nil).GetKeysByMerchant), merchant, showExpired)
}

// GetOrder mocks base method.
func (m *MockDatastore) GetOrder(orderID go_uuid.UUID) (*Order, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetOrder", orderID)
	ret0, _ := ret[0].(*Order)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetOrder indicates an expected call of GetOrder.
func (mr *MockDatastoreMockRecorder) GetOrder(orderID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetOrder", reflect.TypeOf((*MockDatastore)(nil).GetOrder), orderID)
}

// GetOrderByExternalID mocks base method.
func (m *MockDatastore) GetOrderByExternalID(externalID string) (*Order, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetOrderByExternalID", externalID)
	ret0, _ := ret[0].(*Order)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetOrderByExternalID indicates an expected call of GetOrderByExternalID.
func (mr *MockDatastoreMockRecorder) GetOrderByExternalID(externalID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetOrderByExternalID", reflect.TypeOf((*MockDatastore)(nil).GetOrderByExternalID), externalID)
}

// GetOrderCreds mocks base method.
func (m *MockDatastore) GetOrderCreds(orderID go_uuid.UUID, isSigned bool) ([]OrderCreds, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetOrderCreds", orderID, isSigned)
	ret0, _ := ret[0].([]OrderCreds)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetOrderCreds indicates an expected call of GetOrderCreds.
func (mr *MockDatastoreMockRecorder) GetOrderCreds(orderID, isSigned interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetOrderCreds", reflect.TypeOf((*MockDatastore)(nil).GetOrderCreds), orderID, isSigned)
}

// GetOrderCredsByItemID mocks base method.
func (m *MockDatastore) GetOrderCredsByItemID(orderID, itemID go_uuid.UUID, isSigned bool) (*OrderCreds, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetOrderCredsByItemID", orderID, itemID, isSigned)
	ret0, _ := ret[0].(*OrderCreds)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetOrderCredsByItemID indicates an expected call of GetOrderCredsByItemID.
func (mr *MockDatastoreMockRecorder) GetOrderCredsByItemID(orderID, itemID, isSigned interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetOrderCredsByItemID", reflect.TypeOf((*MockDatastore)(nil).GetOrderCredsByItemID), orderID, itemID, isSigned)
}

// GetOrderItem mocks base method.
func (m *MockDatastore) GetOrderItem(ctx context.Context, itemID go_uuid.UUID) (*OrderItem, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetOrderItem", ctx, itemID)
	ret0, _ := ret[0].(*OrderItem)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetOrderItem indicates an expected call of GetOrderItem.
func (mr *MockDatastoreMockRecorder) GetOrderItem(ctx, itemID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetOrderItem", reflect.TypeOf((*MockDatastore)(nil).GetOrderItem), ctx, itemID)
}

// GetOutboxMovAvgDurationSeconds mocks base method.
func (m *MockDatastore) GetOutboxMovAvgDurationSeconds() (int64, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetOutboxMovAvgDurationSeconds")
	ret0, _ := ret[0].(int64)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetOutboxMovAvgDurationSeconds indicates an expected call of GetOutboxMovAvgDurationSeconds.
func (mr *MockDatastoreMockRecorder) GetOutboxMovAvgDurationSeconds() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetOutboxMovAvgDurationSeconds", reflect.TypeOf((*MockDatastore)(nil).GetOutboxMovAvgDurationSeconds))
}

// GetPagedMerchantTransactions mocks base method.
func (m *MockDatastore) GetPagedMerchantTransactions(ctx context.Context, merchantID go_uuid.UUID, pagination *inputs.Pagination) (*[]Transaction, int, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetPagedMerchantTransactions", ctx, merchantID, pagination)
	ret0, _ := ret[0].(*[]Transaction)
	ret1, _ := ret[1].(int)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// GetPagedMerchantTransactions indicates an expected call of GetPagedMerchantTransactions.
func (mr *MockDatastoreMockRecorder) GetPagedMerchantTransactions(ctx, merchantID, pagination interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetPagedMerchantTransactions", reflect.TypeOf((*MockDatastore)(nil).GetPagedMerchantTransactions), ctx, merchantID, pagination)
}

// GetSigningOrderRequestOutboxByOrder mocks base method.
func (m *MockDatastore) GetSigningOrderRequestOutboxByOrder(ctx context.Context, orderID go_uuid.UUID) ([]SigningOrderRequestOutbox, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetSigningOrderRequestOutboxByOrder", ctx, orderID)
	ret0, _ := ret[0].([]SigningOrderRequestOutbox)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetSigningOrderRequestOutboxByOrder indicates an expected call of GetSigningOrderRequestOutboxByOrder.
func (mr *MockDatastoreMockRecorder) GetSigningOrderRequestOutboxByOrder(ctx, orderID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetSigningOrderRequestOutboxByOrder", reflect.TypeOf((*MockDatastore)(nil).GetSigningOrderRequestOutboxByOrder), ctx, orderID)
}

// GetSigningOrderRequestOutboxByOrderItem mocks base method.
func (m *MockDatastore) GetSigningOrderRequestOutboxByOrderItem(ctx context.Context, itemID go_uuid.UUID) ([]SigningOrderRequestOutbox, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetSigningOrderRequestOutboxByOrderItem", ctx, itemID)
	ret0, _ := ret[0].([]SigningOrderRequestOutbox)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetSigningOrderRequestOutboxByOrderItem indicates an expected call of GetSigningOrderRequestOutboxByOrderItem.
func (mr *MockDatastoreMockRecorder) GetSigningOrderRequestOutboxByOrderItem(ctx, itemID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetSigningOrderRequestOutboxByOrderItem", reflect.TypeOf((*MockDatastore)(nil).GetSigningOrderRequestOutboxByOrderItem), ctx, itemID)
}

// GetSigningOrderRequestOutboxByRequestID mocks base method.
func (m *MockDatastore) GetSigningOrderRequestOutboxByRequestID(ctx context.Context, dbi sqlx.QueryerContext, reqID go_uuid.UUID) (*SigningOrderRequestOutbox, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetSigningOrderRequestOutboxByRequestID", ctx, dbi, reqID)
	ret0, _ := ret[0].(*SigningOrderRequestOutbox)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetSigningOrderRequestOutboxByRequestID indicates an expected call of GetSigningOrderRequestOutboxByRequestID.
func (mr *MockDatastoreMockRecorder) GetSigningOrderRequestOutboxByRequestID(ctx, dbi, reqID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetSigningOrderRequestOutboxByRequestID", reflect.TypeOf((*MockDatastore)(nil).GetSigningOrderRequestOutboxByRequestID), ctx, dbi, reqID)
}

// GetSumForTransactions mocks base method.
func (m *MockDatastore) GetSumForTransactions(orderID go_uuid.UUID) (decimal.Decimal, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetSumForTransactions", orderID)
	ret0, _ := ret[0].(decimal.Decimal)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetSumForTransactions indicates an expected call of GetSumForTransactions.
func (mr *MockDatastoreMockRecorder) GetSumForTransactions(orderID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetSumForTransactions", reflect.TypeOf((*MockDatastore)(nil).GetSumForTransactions), orderID)
}

// GetTLV2Creds mocks base method.
func (m *MockDatastore) GetTLV2Creds(ctx context.Context, dbi sqlx.QueryerContext, ordID, itemID, reqID go_uuid.UUID) (*TimeLimitedV2Creds, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetTLV2Creds", ctx, dbi, ordID, itemID, reqID)
	ret0, _ := ret[0].(*TimeLimitedV2Creds)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetTLV2Creds indicates an expected call of GetTLV2Creds.
func (mr *MockDatastoreMockRecorder) GetTLV2Creds(ctx, dbi, ordID, itemID, reqID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetTLV2Creds", reflect.TypeOf((*MockDatastore)(nil).GetTLV2Creds), ctx, dbi, ordID, itemID, reqID)
}

// GetTimeLimitedV2OrderCredsByOrder mocks base method.
func (m *MockDatastore) GetTimeLimitedV2OrderCredsByOrder(orderID go_uuid.UUID) (*TimeLimitedV2Creds, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetTimeLimitedV2OrderCredsByOrder", orderID)
	ret0, _ := ret[0].(*TimeLimitedV2Creds)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetTimeLimitedV2OrderCredsByOrder indicates an expected call of GetTimeLimitedV2OrderCredsByOrder.
func (mr *MockDatastoreMockRecorder) GetTimeLimitedV2OrderCredsByOrder(orderID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetTimeLimitedV2OrderCredsByOrder", reflect.TypeOf((*MockDatastore)(nil).GetTimeLimitedV2OrderCredsByOrder), orderID)
}

// GetTimeLimitedV2OrderCredsByOrderItem mocks base method.
func (m *MockDatastore) GetTimeLimitedV2OrderCredsByOrderItem(itemID go_uuid.UUID) (*TimeLimitedV2Creds, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetTimeLimitedV2OrderCredsByOrderItem", itemID)
	ret0, _ := ret[0].(*TimeLimitedV2Creds)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetTimeLimitedV2OrderCredsByOrderItem indicates an expected call of GetTimeLimitedV2OrderCredsByOrderItem.
func (mr *MockDatastoreMockRecorder) GetTimeLimitedV2OrderCredsByOrderItem(itemID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetTimeLimitedV2OrderCredsByOrderItem", reflect.TypeOf((*MockDatastore)(nil).GetTimeLimitedV2OrderCredsByOrderItem), itemID)
}

// GetTransaction mocks base method.
func (m *MockDatastore) GetTransaction(externalTransactionID string) (*Transaction, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetTransaction", externalTransactionID)
	ret0, _ := ret[0].(*Transaction)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetTransaction indicates an expected call of GetTransaction.
func (mr *MockDatastoreMockRecorder) GetTransaction(externalTransactionID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetTransaction", reflect.TypeOf((*MockDatastore)(nil).GetTransaction), externalTransactionID)
}

// GetTransactions mocks base method.
func (m *MockDatastore) GetTransactions(orderID go_uuid.UUID) (*[]Transaction, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetTransactions", orderID)
	ret0, _ := ret[0].(*[]Transaction)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetTransactions indicates an expected call of GetTransactions.
func (mr *MockDatastoreMockRecorder) GetTransactions(orderID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetTransactions", reflect.TypeOf((*MockDatastore)(nil).GetTransactions), orderID)
}

// GetUncommittedVotesForUpdate mocks base method.
func (m *MockDatastore) GetUncommittedVotesForUpdate(ctx context.Context) (*sqlx.Tx, []*VoteRecord, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetUncommittedVotesForUpdate", ctx)
	ret0, _ := ret[0].(*sqlx.Tx)
	ret1, _ := ret[1].([]*VoteRecord)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// GetUncommittedVotesForUpdate indicates an expected call of GetUncommittedVotesForUpdate.
func (mr *MockDatastoreMockRecorder) GetUncommittedVotesForUpdate(ctx interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetUncommittedVotesForUpdate", reflect.TypeOf((*MockDatastore)(nil).GetUncommittedVotesForUpdate), ctx)
}

// InsertOrderCredsTx mocks base method.
func (m *MockDatastore) InsertOrderCredsTx(ctx context.Context, tx *sqlx.Tx, creds *OrderCreds) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "InsertOrderCredsTx", ctx, tx, creds)
	ret0, _ := ret[0].(error)
	return ret0
}

// InsertOrderCredsTx indicates an expected call of InsertOrderCredsTx.
func (mr *MockDatastoreMockRecorder) InsertOrderCredsTx(ctx, tx, creds interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "InsertOrderCredsTx", reflect.TypeOf((*MockDatastore)(nil).InsertOrderCredsTx), ctx, tx, creds)
}

// InsertSignedOrderCredentialsTx mocks base method.
func (m *MockDatastore) InsertSignedOrderCredentialsTx(ctx context.Context, tx *sqlx.Tx, signedOrderResult *SigningOrderResult) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "InsertSignedOrderCredentialsTx", ctx, tx, signedOrderResult)
	ret0, _ := ret[0].(error)
	return ret0
}

// InsertSignedOrderCredentialsTx indicates an expected call of InsertSignedOrderCredentialsTx.
func (mr *MockDatastoreMockRecorder) InsertSignedOrderCredentialsTx(ctx, tx, signedOrderResult interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "InsertSignedOrderCredentialsTx", reflect.TypeOf((*MockDatastore)(nil).InsertSignedOrderCredentialsTx), ctx, tx, signedOrderResult)
}

// InsertSigningOrderRequestOutbox mocks base method.
func (m *MockDatastore) InsertSigningOrderRequestOutbox(ctx context.Context, requestID, orderID, itemID go_uuid.UUID, signingOrderRequest SigningOrderRequest) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "InsertSigningOrderRequestOutbox", ctx, requestID, orderID, itemID, signingOrderRequest)
	ret0, _ := ret[0].(error)
	return ret0
}

// InsertSigningOrderRequestOutbox indicates an expected call of InsertSigningOrderRequestOutbox.
func (mr *MockDatastoreMockRecorder) InsertSigningOrderRequestOutbox(ctx, requestID, orderID, itemID, signingOrderRequest interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "InsertSigningOrderRequestOutbox", reflect.TypeOf((*MockDatastore)(nil).InsertSigningOrderRequestOutbox), ctx, requestID, orderID, itemID, signingOrderRequest)
}

// InsertTimeLimitedV2OrderCredsTx mocks base method.
func (m *MockDatastore) InsertTimeLimitedV2OrderCredsTx(ctx context.Context, tx *sqlx.Tx, tlv2 TimeAwareSubIssuedCreds) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "InsertTimeLimitedV2OrderCredsTx", ctx, tx, tlv2)
	ret0, _ := ret[0].(error)
	return ret0
}

// InsertTimeLimitedV2OrderCredsTx indicates an expected call of InsertTimeLimitedV2OrderCredsTx.
func (mr *MockDatastoreMockRecorder) InsertTimeLimitedV2OrderCredsTx(ctx, tx, tlv2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "InsertTimeLimitedV2OrderCredsTx", reflect.TypeOf((*MockDatastore)(nil).InsertTimeLimitedV2OrderCredsTx), ctx, tx, tlv2)
}

// InsertVote mocks base method.
func (m *MockDatastore) InsertVote(ctx context.Context, vr VoteRecord) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "InsertVote", ctx, vr)
	ret0, _ := ret[0].(error)
	return ret0
}

// InsertVote indicates an expected call of InsertVote.
func (mr *MockDatastoreMockRecorder) InsertVote(ctx, vr interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "InsertVote", reflect.TypeOf((*MockDatastore)(nil).InsertVote), ctx, vr)
}

// IsStripeSub mocks base method.
func (m *MockDatastore) IsStripeSub(arg0 go_uuid.UUID) (bool, string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "IsStripeSub", arg0)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(string)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// IsStripeSub indicates an expected call of IsStripeSub.
func (mr *MockDatastoreMockRecorder) IsStripeSub(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IsStripeSub", reflect.TypeOf((*MockDatastore)(nil).IsStripeSub), arg0)
}

// MarkVoteErrored mocks base method.
func (m *MockDatastore) MarkVoteErrored(ctx context.Context, vr VoteRecord, tx *sqlx.Tx) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "MarkVoteErrored", ctx, vr, tx)
	ret0, _ := ret[0].(error)
	return ret0
}

// MarkVoteErrored indicates an expected call of MarkVoteErrored.
func (mr *MockDatastoreMockRecorder) MarkVoteErrored(ctx, vr, tx interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "MarkVoteErrored", reflect.TypeOf((*MockDatastore)(nil).MarkVoteErrored), ctx, vr, tx)
}

// Migrate mocks base method.
func (m *MockDatastore) Migrate(arg0 ...uint) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{}
	for _, a := range arg0 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Migrate", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// Migrate indicates an expected call of Migrate.
func (mr *MockDatastoreMockRecorder) Migrate(arg0 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Migrate", reflect.TypeOf((*MockDatastore)(nil).Migrate), arg0...)
}

// NewMigrate mocks base method.
func (m *MockDatastore) NewMigrate() (*v4.Migrate, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NewMigrate")
	ret0, _ := ret[0].(*v4.Migrate)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// NewMigrate indicates an expected call of NewMigrate.
func (mr *MockDatastoreMockRecorder) NewMigrate() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewMigrate", reflect.TypeOf((*MockDatastore)(nil).NewMigrate))
}

// RawDB mocks base method.
func (m *MockDatastore) RawDB() *sqlx.DB {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RawDB")
	ret0, _ := ret[0].(*sqlx.DB)
	return ret0
}

// RawDB indicates an expected call of RawDB.
func (mr *MockDatastoreMockRecorder) RawDB() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RawDB", reflect.TypeOf((*MockDatastore)(nil).RawDB))
}

// RollbackTx mocks base method.
func (m *MockDatastore) RollbackTx(tx *sqlx.Tx) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "RollbackTx", tx)
}

// RollbackTx indicates an expected call of RollbackTx.
func (mr *MockDatastoreMockRecorder) RollbackTx(tx interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RollbackTx", reflect.TypeOf((*MockDatastore)(nil).RollbackTx), tx)
}

// RollbackTxAndHandle mocks base method.
func (m *MockDatastore) RollbackTxAndHandle(tx *sqlx.Tx) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RollbackTxAndHandle", tx)
	ret0, _ := ret[0].(error)
	return ret0
}

// RollbackTxAndHandle indicates an expected call of RollbackTxAndHandle.
func (mr *MockDatastoreMockRecorder) RollbackTxAndHandle(tx interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RollbackTxAndHandle", reflect.TypeOf((*MockDatastore)(nil).RollbackTxAndHandle), tx)
}

// SendSigningRequest mocks base method.
func (m *MockDatastore) SendSigningRequest(ctx context.Context, signingRequestWriter SigningRequestWriter) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SendSigningRequest", ctx, signingRequestWriter)
	ret0, _ := ret[0].(error)
	return ret0
}

// SendSigningRequest indicates an expected call of SendSigningRequest.
func (mr *MockDatastoreMockRecorder) SendSigningRequest(ctx, signingRequestWriter interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SendSigningRequest", reflect.TypeOf((*MockDatastore)(nil).SendSigningRequest), ctx, signingRequestWriter)
}

// SetOrderPaid mocks base method.
func (m *MockDatastore) SetOrderPaid(arg0 context.Context, arg1 *go_uuid.UUID) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetOrderPaid", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// SetOrderPaid indicates an expected call of SetOrderPaid.
func (mr *MockDatastoreMockRecorder) SetOrderPaid(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetOrderPaid", reflect.TypeOf((*MockDatastore)(nil).SetOrderPaid), arg0, arg1)
}

// SetOrderTrialDays mocks base method.
func (m *MockDatastore) SetOrderTrialDays(ctx context.Context, orderID *go_uuid.UUID, days int64) (*Order, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetOrderTrialDays", ctx, orderID, days)
	ret0, _ := ret[0].(*Order)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// SetOrderTrialDays indicates an expected call of SetOrderTrialDays.
func (mr *MockDatastoreMockRecorder) SetOrderTrialDays(ctx, orderID, days interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetOrderTrialDays", reflect.TypeOf((*MockDatastore)(nil).SetOrderTrialDays), ctx, orderID, days)
}

// UpdateOrder mocks base method.
func (m *MockDatastore) UpdateOrder(orderID go_uuid.UUID, status string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateOrder", orderID, status)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateOrder indicates an expected call of UpdateOrder.
func (mr *MockDatastoreMockRecorder) UpdateOrder(orderID, status interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateOrder", reflect.TypeOf((*MockDatastore)(nil).UpdateOrder), orderID, status)
}

// UpdateOrderMetadata mocks base method.
func (m *MockDatastore) UpdateOrderMetadata(orderID go_uuid.UUID, key, value string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateOrderMetadata", orderID, key, value)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateOrderMetadata indicates an expected call of UpdateOrderMetadata.
func (mr *MockDatastoreMockRecorder) UpdateOrderMetadata(orderID, key, value interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateOrderMetadata", reflect.TypeOf((*MockDatastore)(nil).UpdateOrderMetadata), orderID, key, value)
}

// UpdateSigningOrderRequestOutboxTx mocks base method.
func (m *MockDatastore) UpdateSigningOrderRequestOutboxTx(ctx context.Context, tx *sqlx.Tx, requestID go_uuid.UUID, completedAt time.Time) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateSigningOrderRequestOutboxTx", ctx, tx, requestID, completedAt)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpdateSigningOrderRequestOutboxTx indicates an expected call of UpdateSigningOrderRequestOutboxTx.
func (mr *MockDatastoreMockRecorder) UpdateSigningOrderRequestOutboxTx(ctx, tx, requestID, completedAt interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateSigningOrderRequestOutboxTx", reflect.TypeOf((*MockDatastore)(nil).UpdateSigningOrderRequestOutboxTx), ctx, tx, requestID, completedAt)
}

// UpdateTransaction mocks base method.
func (m *MockDatastore) UpdateTransaction(orderID go_uuid.UUID, externalTransactionID, status, currency, kind string, amount decimal.Decimal) (*Transaction, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateTransaction", orderID, externalTransactionID, status, currency, kind, amount)
	ret0, _ := ret[0].(*Transaction)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// UpdateTransaction indicates an expected call of UpdateTransaction.
func (mr *MockDatastoreMockRecorder) UpdateTransaction(orderID, externalTransactionID, status, currency, kind, amount interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateTransaction", reflect.TypeOf((*MockDatastore)(nil).UpdateTransaction), orderID, externalTransactionID, status, currency, kind, amount)
}
