package xslack

import (
	"context"
	"errors"
	"net/http"
	"testing"

	should "github.com/stretchr/testify/assert"
)

func TestClient_SendMessage(t *testing.T) {
	type tcGiven struct {
		msg  *Message
		doer httpDoer
	}

	type tcExpected struct {
		err error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "http_do_error",
			given: tcGiven{
				msg: &Message{},
				doer: &mockDoer{
					fnDo: func(req *http.Request) (*http.Response, error) {
						return nil, errors.New("http_error")
					},
				},
			},
			exp: tcExpected{
				err: errors.New("http_error"),
			},
		},

		{
			name: "not_http_status_okay",
			given: tcGiven{
				msg: &Message{},
				doer: &mockDoer{
					fnDo: func(req *http.Request) (*http.Response, error) {
						return &http.Response{StatusCode: http.StatusInternalServerError}, nil
					},
				},
			},
			exp: tcExpected{
				err: errors.New("xslack: failed to send message: status code 500"),
			},
		},

		{
			name: "success",
			given: tcGiven{
				msg: &Message{},
				doer: &mockDoer{
					fnDo: func(req *http.Request) (*http.Response, error) {
						return &http.Response{StatusCode: http.StatusOK}, nil
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			cl := &Client{
				doer: tc.given.doer,
			}

			ctx := context.Background()

			actual := cl.SendMessage(ctx, tc.given.msg)
			should.Equal(t, tc.exp.err, actual)
		})
	}
}

type mockDoer struct {
	fnDo func(req *http.Request) (*http.Response, error)
}

func (m *mockDoer) Do(req *http.Request) (*http.Response, error) {
	if m.fnDo == nil {
		return &http.Response{}, nil
	}

	return m.fnDo(req)
}
