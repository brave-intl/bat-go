package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/libs/clients"
	testutils "github.com/brave-intl/bat-go/libs/test"
	"github.com/brave-intl/bat-go/services/settlement/internal/consumer"
	"github.com/brave-intl/bat-go/services/settlement/internal/payment"

	"github.com/stretchr/testify/assert"
)

func TestSubmit_Handle(t *testing.T) {
	type fields struct {
		payment PaymentClient
	}

	type args struct {
		ctx     context.Context
		message consumer.Message
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "success",
			fields: fields{
				payment: &mockPaymentClient{fnSubmit: func(ctx context.Context, authorizationHeader payment.AuthorizationHeader, details payment.SerializedDetails) (payment.Submit, error) {
					return payment.Submit{}, nil
				}},
			},
			args: args{
				ctx: context.Background(),
				message: consumer.Message{
					Headers: consumer.Headers{testutils.RandomString(): testutils.RandomString()},
				},
			},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.NoError(t, err)
			},
		},
		{
			name: "retry_error",
			fields: fields{
				payment: &mockPaymentClient{
					fnSubmit: func(ctx context.Context, authorizationHeader payment.AuthorizationHeader, details payment.SerializedDetails) (payment.Submit, error) {
						return payment.Submit{}, newHTTPErrorWithStatus(http.StatusRequestTimeout)
					}},
			},
			args: args{
				ctx: context.Background(),
				message: consumer.Message{
					Headers: consumer.Headers{testutils.RandomString(): testutils.RandomString()},
				},
			},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorAs(t, err, &consumer.RetryError{})
			},
		},
		{
			name: "retry_unknown_unwrap_error",
			fields: fields{
				payment: &mockPaymentClient{
					fnSubmit: func(ctx context.Context, authorizationHeader payment.AuthorizationHeader, details payment.SerializedDetails) (payment.Submit, error) {
						return payment.Submit{}, errors.New("random error")
					}},
			},
			args: args{
				ctx: context.Background(),
				message: consumer.Message{
					Headers: consumer.Headers{testutils.RandomString(): testutils.RandomString()},
				},
			},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorAs(t, err, &consumer.RetryError{})
			},
		},
		{
			name: "permanent_error",
			fields: fields{
				payment: &mockPaymentClient{
					fnSubmit: func(ctx context.Context, authorizationHeader payment.AuthorizationHeader, details payment.SerializedDetails) (payment.Submit, error) {
						return payment.Submit{}, newHTTPErrorWithStatus(http.StatusUnauthorized)
					}},
			},
			args: args{
				ctx: context.Background(),
				message: consumer.Message{
					Headers: consumer.Headers{testutils.RandomString(): testutils.RandomString()},
				},
			},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorContains(t, err, "error calling submit:")
			},
		},
		{
			name: "retry_submit",
			fields: fields{
				payment: &mockPaymentClient{
					fnSubmit: func(ctx context.Context, authorizationHeader payment.AuthorizationHeader, details payment.SerializedDetails) (payment.Submit, error) {
						return payment.Submit{RetryAfter: time.Duration(10)}, nil
					}},
			},
			args: args{
				ctx: context.Background(),
				message: consumer.Message{
					Headers: consumer.Headers{testutils.RandomString(): testutils.RandomString()},
				},
			},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorIs(t, err, consumer.RetryError{Err: ErrSubmitNotComplete, RetryAfter: time.Duration(10)})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &Handler{
				payment: tc.fields.payment,
			}
			err := s.Handle(tc.args.ctx, tc.args.message)
			tc.wantErr(t, err)
		})
	}
}

func Test_canRetry(t *testing.T) {
	type args struct {
		err error
	}

	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "is_retriable",
			args: args{
				err: newHTTPErrorWithStatus(http.StatusRequestTimeout),
			},
			want: true,
		},
		{
			name: "not_retriable",
			args: args{
				err: newHTTPErrorWithStatus(http.StatusUnauthorized),
			},
			want: false,
		},
		{
			name: "unwrap_error",
			args: args{
				err: errors.New(testutils.RandomString()),
			},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isRetry(tc.args.err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func newHTTPErrorWithStatus(code int) error {
	return clients.NewHTTPError(errors.New(testutils.RandomString()), "/random/path", testutils.RandomString(), code, testutils.RandomString())
}

type mockPaymentClient struct {
	fnSubmit func(ctx context.Context, authorizationHeader payment.AuthorizationHeader, details payment.SerializedDetails) (payment.Submit, error)
}

func (m *mockPaymentClient) Submit(ctx context.Context, authorizationHeader payment.AuthorizationHeader, details payment.SerializedDetails) (payment.Submit, error) {
	if m.fnSubmit != nil {
		return m.fnSubmit(ctx, authorizationHeader, details)
	}
	return payment.Submit{}, nil
}
