package consumer

import (
	"testing"
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMessage(t *testing.T) {
	type args struct {
		body interface{}
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "success",
			args: args{body: "body"},
			want: "\"body\"",
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.NoError(t, err)
			},
		},
		{
			name: "error",
			args: args{body: make(chan int)}, // bad value.
			want: "",
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorContains(t, err, "error creating new message: ")
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewMessage(tc.args.body)
			if !tc.wantErr(t, err) {
				return
			}
			assert.NotEmpty(t, got.ID)
			assert.WithinDuration(t, time.Now(), got.Timestamp, 500*time.Millisecond)
			assert.NotNil(t, got.Headers)
			assert.Equal(t, got.Body, tc.want)
		})
	}
}

func TestNewMessageFromString(t *testing.T) {
	type args struct {
		data string
	}
	tests := []struct {
		name    string
		args    args
		want    Message
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "success",
			args: args{
				data: `{"id":"961f8da4-1975-476c-a6af-3d374cc3e2c3","timestamp":"2023-09-24T12:07:35.226242454Z","headers":{"key": "value"},"body":"body"}`,
			},
			want: Message{
				ID:        parseUUID(t, "961f8da4-1975-476c-a6af-3d374cc3e2c3"),
				Timestamp: parseTime(t, time.RFC3339, "2023-09-24T12:07:35.226242454Z"),
				Headers:   Headers{"key": "value"},
				Body:      "body",
			},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.NoError(t, err)
			},
		},
		{
			name: "error",
			args: args{
				data: `""`,
			},
			want: Message{},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorContains(t, err, "error creating new message: ")
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewMessageFromString(tc.args.data)
			if !tc.wantErr(t, err) {
				return
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func parseUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()

	id, err := uuid.FromString(s)
	require.NoError(t, err)

	return id
}

func parseTime(t *testing.T, layout string, s string) time.Time {
	t.Helper()

	out, err := time.Parse(layout, s)
	require.NoError(t, err)

	return out
}
