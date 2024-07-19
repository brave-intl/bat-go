package handler

import (
	"context"
	"testing"

	"github.com/go-playground/validator/v10"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/services/skus/model"
)

func TestCollectValidationErrors_CreateOrderRequestNew(t *testing.T) {
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
			given: &model.CreateOrderRequestNew{
				Email:    "you@example.com",
				Currency: "USD",
				StripeMetadata: &model.OrderStripeMetadata{
					SuccessURI: "https://example.com/success",
					CancelURI:  "https://example.com/cancel",
				},
				PaymentMethods: []string{"stripe"},
				Items: []model.OrderItemRequestNew{
					{
						Quantity:                1,
						SKU:                     "sku",
						Location:                "location",
						Description:             "description",
						CredentialType:          "credential_type",
						CredentialValidDuration: "P1M",
						StripeMetadata: &model.ItemStripeMetadata{
							ProductID: "product_id",
							ItemID:    "item_id",
						},
					},
				},
			},
			exp: tcExpected{noErr: true},
		},

		{
			name: "no_errors_02_no_payment_methods",
			given: &model.CreateOrderRequestNew{
				Email:    "you@example.com",
				Currency: "USD",
				StripeMetadata: &model.OrderStripeMetadata{
					SuccessURI: "https://example.com/success",
					CancelURI:  "https://example.com/cancel",
				},
				Items: []model.OrderItemRequestNew{
					{
						Quantity:                1,
						SKU:                     "sku",
						Location:                "location",
						Description:             "description",
						CredentialType:          "credential_type",
						CredentialValidDuration: "P1M",
						StripeMetadata: &model.ItemStripeMetadata{
							ProductID: "product_id",
							ItemID:    "item_id",
						},
					},
				},
			},
			exp: tcExpected{noErr: true},
		},

		{
			name: "one_field",
			given: &model.CreateOrderRequestNew{
				Email:    "you_example.com",
				Currency: "USD",
				StripeMetadata: &model.OrderStripeMetadata{
					SuccessURI: "https://example.com/success",
					CancelURI:  "https://example.com/cancel",
				},
				PaymentMethods: []string{"stripe"},
				Items: []model.OrderItemRequestNew{
					{
						Quantity:                1,
						SKU:                     "sku",
						Location:                "location",
						Description:             "description",
						CredentialType:          "credential_type",
						CredentialValidDuration: "P1M",
						StripeMetadata: &model.ItemStripeMetadata{
							ProductID: "product_id",
							ItemID:    "item_id",
						},
					},
				},
			},
			exp: tcExpected{
				result: map[string]string{
					"Email": "Key: 'CreateOrderRequestNew.Email' Error:Field validation for 'Email' failed on the 'email' tag",
				},
				ok: true,
			},
		},

		{
			name: "few_fields",
			given: &model.CreateOrderRequestNew{
				Email:    "you_example.com",
				Currency: "USDx",
				StripeMetadata: &model.OrderStripeMetadata{
					SuccessURI: "https://example.com/success",
					CancelURI:  "sdsds",
				},
				PaymentMethods: []string{"stripe"},
				Items: []model.OrderItemRequestNew{
					{
						Quantity:                1,
						SKU:                     "sku",
						Location:                "location",
						Description:             "description",
						CredentialType:          "credential_type",
						CredentialValidDuration: "P1M",
						StripeMetadata: &model.ItemStripeMetadata{
							ProductID: "product_id",
							ItemID:    "item_id",
						},
					},
				},
			},
			exp: tcExpected{
				result: map[string]string{
					"Email":     "Key: 'CreateOrderRequestNew.Email' Error:Field validation for 'Email' failed on the 'email' tag",
					"Currency":  "Key: 'CreateOrderRequestNew.Currency' Error:Field validation for 'Currency' failed on the 'iso4217' tag",
					"CancelURI": "Key: 'CreateOrderRequestNew.StripeMetadata.CancelURI' Error:Field validation for 'CancelURI' failed on the 'http_url' tag",
				},
				ok: true,
			},
		},
	}

	valid := validator.New()

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			verr := valid.StructCtx(context.Background(), tc.given)
			must.Equal(t, tc.exp.noErr, verr == nil)

			act, ok := collectValidationErrors(verr)

			should.Equal(t, tc.exp.ok, ok)
			should.Equal(t, tc.exp.result, act)
		})
	}
}
