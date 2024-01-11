package skus

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeSubmitReceiptReq(t *testing.T) {
	type tcGiven struct {
		req SubmitReceiptRequestV1
	}

	type exp struct {
		srr SubmitReceiptRequestV1
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   exp
	}

	tests := []testCase{
		{
			name: "success",
			given: tcGiven{
				req: SubmitReceiptRequestV1{
					Type:           "android",
					Package:        "brave,com",
					SubscriptionID: "sub-id",
					Blob:           "test",
				},
			},
			exp: exp{
				srr: SubmitReceiptRequestV1{
					Type:           "android",
					Package:        "brave,com",
					SubscriptionID: "sub-id",
					Blob:           "test",
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			b, err1 := json.Marshal(tc.given.req)
			require.NoError(t, err1)

			payload := base64.StdEncoding.EncodeToString(b)

			actual, err2 := decodeSubmitReceiptReq([]byte(payload))
			require.NoError(t, err2)

			assert.Equal(t, tc.exp.srr.Type, actual.Type)
			assert.Equal(t, tc.exp.srr.Package, actual.Package)
			assert.Equal(t, tc.exp.srr.SubscriptionID, actual.SubscriptionID)
			assert.Equal(t, tc.exp.srr.Blob, actual.Blob)
		})
	}
}

func TestValidateSubmitReceiptReq(t *testing.T) {
	type tcGiven struct {
		req SubmitReceiptRequestV1
	}

	type exp struct {
		err error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   exp
	}

	tests := []testCase{
		{
			name: "raw_receipt_required",
			given: tcGiven{req: SubmitReceiptRequestV1{
				Type:           "android",
				Package:        "brave,com",
				SubscriptionID: "sub-id",
			},
			},
			exp: exp{err: errRawReceiptRequired},
		},
		{
			name: "invalid_type",
			given: tcGiven{
				req: SubmitReceiptRequestV1{
					Type:           "some-random-type",
					Package:        "brave,com",
					SubscriptionID: "sub-id",
					Blob:           "test",
				},
			},
			exp: exp{err: errors.New("invalid type got some-random-type")},
		},
		{
			name: "success_android",
			given: tcGiven{
				req: SubmitReceiptRequestV1{
					Type:           "android",
					Package:        "brave,com",
					SubscriptionID: "sub-id",
					Blob:           "test",
				},
			},
			exp: exp{err: nil},
		},
		{
			name: "success_ios",
			given: tcGiven{
				req: SubmitReceiptRequestV1{
					Type:           "android",
					Package:        "brave,com",
					SubscriptionID: "sub-id",
					Blob:           "test",
				},
			},
			exp: exp{err: nil},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			err := validateSubmitReceiptReq(tc.given.req)
			assert.Equal(t, tc.exp.err, err)
		})
	}
}
