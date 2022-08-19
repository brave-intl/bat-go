// Code generated by MockGen. DO NOT EDIT.
// Source: ./services/promotion/service.go

// Package promotion is a generated GoMock package.
package promotion

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	kafka "github.com/segmentio/kafka-go"
)

// MockKafkaReader is a mock of KafkaReader interface.
type MockKafkaReader struct {
	ctrl     *gomock.Controller
	recorder *MockKafkaReaderMockRecorder
}

// MockKafkaReaderMockRecorder is the mock recorder for MockKafkaReader.
type MockKafkaReaderMockRecorder struct {
	mock *MockKafkaReader
}

// NewMockKafkaReader creates a new mock instance.
func NewMockKafkaReader(ctrl *gomock.Controller) *MockKafkaReader {
	mock := &MockKafkaReader{ctrl: ctrl}
	mock.recorder = &MockKafkaReaderMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockKafkaReader) EXPECT() *MockKafkaReaderMockRecorder {
	return m.recorder
}

// ReadMessage mocks base method.
func (m *MockKafkaReader) ReadMessage(ctx context.Context) (kafka.Message, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReadMessage", ctx)
	ret0, _ := ret[0].(kafka.Message)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ReadMessage indicates an expected call of ReadMessage.
func (mr *MockKafkaReaderMockRecorder) ReadMessage(ctx interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReadMessage", reflect.TypeOf((*MockKafkaReader)(nil).ReadMessage), ctx)
}
