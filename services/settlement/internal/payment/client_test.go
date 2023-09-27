package payment

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_parseXSubmitRetryAfter(t *testing.T) {
	type args struct {
		resp *http.Response
	}
	tests := []struct {
		name    string
		args    args
		want    uint64
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "success",
			args: args{resp: newRespWithHeader("x-submit-retry-after", "1")},
			want: 1,
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.NoError(t, err)
			},
		},
		{
			name: "error_no_value",
			args: args{resp: newRespWithHeader("random-header", "-1")},
			want: 0,
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorIs(t, err, ErrHeaderNotFound)
			},
		},
		{
			name: "error_negative_value",
			args: args{resp: newRespWithHeader("x-submit-retry-after", "-1")},
			want: 0,
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorContains(t, err, "error parsing unsigned int: ")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseXSubmitRetryAfter(tc.args.resp)
			if !tc.wantErr(t, err) {
				return
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func newRespWithHeader(k string, v string) *http.Response {
	resp := http.Response{
		Header: map[string][]string{},
	}
	resp.Header.Set(k, v)
	return &resp
}
