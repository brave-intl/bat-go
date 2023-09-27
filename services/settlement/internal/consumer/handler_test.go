package consumer

import (
	"context"
	"errors"
	"testing"

	tu "github.com/brave-intl/bat-go/libs/test"
	"github.com/brave-intl/bat-go/services/settlement/internal/consumer/redis"

	"github.com/stretchr/testify/assert"
)

func TestDLQHandler_Handle(t *testing.T) {
	type fields struct {
		adder  XAdder
		conf   Config
		dlq    string
		newMsg MessageFactory
	}

	type args struct {
		ctx             context.Context
		xMsg            redis.XMessage
		processingError error
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "error_creating_new_dlq_message",
			fields: fields{
				newMsg: func(body interface{}) (Message, error) {
					return Message{}, errors.New(tu.RandomString())
				},
			},
			args: args{
				ctx:             context.TODO(),
				xMsg:            redis.XMessage{},
				processingError: errors.New(tu.RandomString()),
			},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorContains(t, err, "error creating new dlq message: ")
			},
		},
		{
			name: "error_sending_dlq_message",
			fields: fields{
				adder: &mockXAdder{
					fnXAdd: func(ctx context.Context, args redis.XAddArgs) error {
						return errors.New(tu.RandomString())
					},
				},
				conf: Config{
					streamName:    tu.RandomString(),
					consumerID:    tu.RandomString(),
					consumerGroup: tu.RandomString(),
				},
				dlq: tu.RandomString(),
				newMsg: func(body interface{}) (Message, error) {
					return NewMessage(tu.RandomString())
				},
			},
			args: args{
				ctx:             context.TODO(),
				xMsg:            redis.XMessage{},
				processingError: errors.New(tu.RandomString()),
			},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorContains(t, err, "error sending dlq message: ")
			},
		},
		{
			name: "success",
			fields: fields{
				adder: &mockXAdder{
					fnXAdd: func(ctx context.Context, args redis.XAddArgs) error {
						return nil
					},
				},
				conf: Config{
					streamName:    tu.RandomString(),
					consumerID:    tu.RandomString(),
					consumerGroup: tu.RandomString(),
				},
				dlq: tu.RandomString(),
				newMsg: func(body interface{}) (Message, error) {
					return NewMessage(tu.RandomString())
				},
			},
			args: args{
				ctx:             context.TODO(),
				xMsg:            redis.XMessage{},
				processingError: errors.New(tu.RandomString()),
			},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.NoError(t, err)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := &DLQHandler{
				adder:  tc.fields.adder,
				conf:   tc.fields.conf,
				dlq:    tc.fields.dlq,
				newMsg: tc.fields.newMsg,
			}
			err := d.Handle(tc.args.ctx, tc.args.xMsg, tc.args.processingError)
			tc.wantErr(t, err)
		})
	}
}

type mockXAdder struct {
	fnXAdd func(ctx context.Context, args redis.XAddArgs) error
}

func (m *mockXAdder) XAdd(ctx context.Context, args redis.XAddArgs) error {
	if m.fnXAdd != nil {
		return m.fnXAdd(ctx, args)
	}
	return nil
}
