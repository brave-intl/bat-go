package handler

import (
	"context"
	"testing"
	"time"

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
						SKUVnt:                  "sku_vnt",
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
						SKUVnt:                  "sku_vnt",
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
						SKUVnt:                  "sku_vnt",
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
						SKUVnt:                  "sku_vnt",
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

func TestCollectValidationErrors_CreatePaidOrderRequest(t *testing.T) {
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

	validItems := []model.OrderItemRequestNew{
		{
			Quantity:                1,
			SKU:                     "sku",
			SKUVnt:                  "sku_vnt",
			Location:                "location",
			Description:             "description",
			CredentialType:          "credential_type",
			CredentialValidDuration: "P1M",
		},
	}

	tests := []testCase{
		{
			name:  "invalid_type",
			given: map[string]struct{}{},
			exp:   tcExpected{},
		},

		{
			name: "no_errors",
			given: &model.CreatePaidOrderRequest{
				CheckoutSessionID: "cs_id",
				SubscriptionID:    "sub_id",
				PaymentProc:       "stripe",
				Currency:          "USD",
				Items:             validItems,
				PaidAt:            time.Now(),
				ExpiresAt:         time.Now(),
			},
			exp: tcExpected{noErr: true},
		},

		{
			name: "one_field",
			given: &model.CreatePaidOrderRequest{
				SubscriptionID: "sub_id",
				PaymentProc:    "stripe",
				Currency:       "USD",
				Items:          validItems,
				PaidAt:         time.Now(),
				ExpiresAt:      time.Now(),
			},
			exp: tcExpected{
				result: map[string]string{
					"CheckoutSessionID": "Key: 'CreatePaidOrderRequest.CheckoutSessionID' Error:Field validation for 'CheckoutSessionID' failed on the 'required' tag",
				},
				ok: true,
			},
		},

		{
			name: "few_fields",
			given: &model.CreatePaidOrderRequest{
				PaymentProc: "stripe",
				Currency:    "USDx",
				Items:       validItems,
			},
			exp: tcExpected{
				result: map[string]string{
					"CheckoutSessionID": "Key: 'CreatePaidOrderRequest.CheckoutSessionID' Error:Field validation for 'CheckoutSessionID' failed on the 'required' tag",
					"SubscriptionID":    "Key: 'CreatePaidOrderRequest.SubscriptionID' Error:Field validation for 'SubscriptionID' failed on the 'required' tag",
					"Currency":          "Key: 'CreatePaidOrderRequest.Currency' Error:Field validation for 'Currency' failed on the 'iso4217' tag",
					"PaidAt":            "Key: 'CreatePaidOrderRequest.PaidAt' Error:Field validation for 'PaidAt' failed on the 'required' tag",
					"ExpiresAt":         "Key: 'CreatePaidOrderRequest.ExpiresAt' Error:Field validation for 'ExpiresAt' failed on the 'required' tag",
				},
				ok: true,
			},
		},

		{
			name: "no_items",
			given: &model.CreatePaidOrderRequest{
				CheckoutSessionID: "cs_id",
				SubscriptionID:    "sub_id",
				PaymentProc:       "stripe",
				Currency:          "USD",
				PaidAt:            time.Now(),
				ExpiresAt:         time.Now(),
			},
			exp: tcExpected{
				result: map[string]string{
					"Items": "Key: 'CreatePaidOrderRequest.Items' Error:Field validation for 'Items' failed on the 'required' tag",
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
