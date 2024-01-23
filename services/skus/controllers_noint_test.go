package skus

import (
	"context"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/services/skus/model"
)

func TestCollectValidationErrors_ReceiptRequest(t *testing.T) {
	type tcExpected struct {
		result map[string]string
		ok     bool
		noErr  bool
	}

	type testCase struct {
		name  string
		given any
		exp   tcExpected
	}

	tests := []testCase{
		{
			name:  "invalid_type",
			given: map[string]struct{}{},
			exp:   tcExpected{},
		},

		{
			name: "no_errors_01",
			given: model.ReceiptRequest{
				Type:           model.VendorGoogle,
				Blob:           "some_data",
				Package:        "some_package",
				SubscriptionID: "some_subscription",
			},

			exp: tcExpected{noErr: true},
		},

		{
			name: "no_errors_02",
			given: model.ReceiptRequest{
				Type:           model.VendorApple,
				Blob:           "some_data",
				Package:        "some_package",
				SubscriptionID: "some_subscription",
			},

			exp: tcExpected{noErr: true},
		},

		{
			name: "invalid_vendor",
			given: model.ReceiptRequest{
				Type:           model.Vendor("brave"),
				Blob:           "some_data",
				Package:        "some_package",
				SubscriptionID: "some_subscription",
			},

			exp: tcExpected{
				result: map[string]string{
					"Type": "Key: 'ReceiptRequest.Type' Error:Field validation for 'Type' failed on the 'oneof' tag",
				},
				ok: true,
			},
		},

		{
			name: "no_blob",
			given: model.ReceiptRequest{
				Type:           model.VendorApple,
				Package:        "some_package",
				SubscriptionID: "some_subscription",
			},

			exp: tcExpected{
				result: map[string]string{
					"Blob": "Key: 'ReceiptRequest.Blob' Error:Field validation for 'Blob' failed on the 'required' tag",
				},
				ok: true,
			},
		},

		{
			name: "both_fields_error",
			given: model.ReceiptRequest{
				Type:           model.Vendor("brave"),
				Package:        "some_package",
				SubscriptionID: "some_subscription",
			},

			exp: tcExpected{
				result: map[string]string{
					"Type": "Key: 'ReceiptRequest.Type' Error:Field validation for 'Type' failed on the 'oneof' tag",
					"Blob": "Key: 'ReceiptRequest.Blob' Error:Field validation for 'Blob' failed on the 'required' tag",
				},
				ok: true,
			},
		},
	}

	valid := validator.New()

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			verr := valid.StructCtx(context.TODO(), tc.given)
			require.Equal(t, tc.exp.noErr, verr == nil)

			act, ok := collectValidationErrors(verr)

			assert.Equal(t, tc.exp.ok, ok)
			assert.Equal(t, tc.exp.result, act)
		})
	}
}
