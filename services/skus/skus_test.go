package skus

import (
	"testing"

	"github.com/shopspring/decimal"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/services/skus/model"
)

func TestSKUNameByMobileName(t *testing.T) {
	type tcExpected struct {
		sku string
		err error
	}

	type testCase struct {
		name  string
		given string
		exp   tcExpected
	}

	tests := []testCase{
		{
			name:  "android_release_monthly_leo",
			given: "brave.leo.monthly",
			exp:   tcExpected{sku: "brave-leo-premium"},
		},

		{
			name:  "android_beta_monthly_leo",
			given: "beta.leo.monthly",
			exp:   tcExpected{sku: "brave-leo-premium"},
		},

		{
			name:  "android_nightly_monthly_leo",
			given: "nightly.leo.monthly",
			exp:   tcExpected{sku: "brave-leo-premium"},
		},

		{
			name:  "ios_monthly_leo",
			given: "braveleo.monthly",
			exp:   tcExpected{sku: "brave-leo-premium"},
		},

		{
			name:  "android_release_yearly_leo",
			given: "brave.leo.yearly",
			exp:   tcExpected{sku: "brave-leo-premium-year"},
		},

		{
			name:  "android_beta_yearly_leo",
			given: "beta.leo.yearly",
			exp:   tcExpected{sku: "brave-leo-premium-year"},
		},

		{
			name:  "android_nightly_yearly_leo",
			given: "nightly.leo.yearly",
			exp:   tcExpected{sku: "brave-leo-premium-year"},
		},

		{
			name:  "ios_yearly_leo",
			given: "braveleo.yearly",
			exp:   tcExpected{sku: "brave-leo-premium-year"},
		},

		{
			name:  "invalid",
			given: "something_else",
			exp:   tcExpected{err: model.ErrInvalidMobileProduct},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, err := skuNameByMobileName(tc.given)
			must.Equal(t, tc.exp.err, err)

			should.Equal(t, tc.exp.sku, actual)
		})
	}
}

func TestNewOrderItemReqForSubID(t *testing.T) {
	type tcGiven struct {
		subID string
		set   map[string]model.OrderItemRequestNew
	}

	type tcExpected struct {
		req model.OrderItemRequestNew
		err error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "invalid_sub_id",
			given: tcGiven{
				subID: "something_else",
			},
			exp: tcExpected{
				err: model.ErrInvalidMobileProduct,
			},
		},

		{
			name: "valid_sub_id_missing_in_set",
			given: tcGiven{
				subID: "brave.leo.monthly",
				set: map[string]model.OrderItemRequestNew{
					"brave-leo-premium-year": model.OrderItemRequestNew{
						SKU: "brave-leo-premium-year",
					},
				},
			},
			exp: tcExpected{
				err: model.ErrInvalidMobileProduct,
			},
		},

		{
			name: "android_release_monthly_leo",
			given: tcGiven{
				subID: "brave.leo.monthly",
				set:   newOrderItemReqNewLeoSet("development"),
			},
			exp: tcExpected{
				req: model.OrderItemRequestNew{
					Quantity:                    1,
					IssuerTokenBuffer:           3,
					SKU:                         "brave-leo-premium",
					Location:                    "leo.brave.software",
					Description:                 "Premium access to Leo",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1M",
					Price:                       decimal.RequireFromString("15.00"),
					CredentialValidDurationEach: ptrTo("P1D"),
					IssuanceInterval:            ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_OtZCXOCIO3AJE6",
						ItemID:    "price_1O5m3lHof20bphG6DloANAcc",
					},
				},
			},
		},

		{
			name: "ios_monthly_leo",
			given: tcGiven{
				subID: "braveleo.monthly",
				set:   newOrderItemReqNewLeoSet("development"),
			},
			exp: tcExpected{
				req: model.OrderItemRequestNew{
					Quantity:                    1,
					IssuerTokenBuffer:           3,
					SKU:                         "brave-leo-premium",
					Location:                    "leo.brave.software",
					Description:                 "Premium access to Leo",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1M",
					Price:                       decimal.RequireFromString("15.00"),
					CredentialValidDurationEach: ptrTo("P1D"),
					IssuanceInterval:            ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_OtZCXOCIO3AJE6",
						ItemID:    "price_1O5m3lHof20bphG6DloANAcc",
					},
				},
			},
		},

		{
			name: "android_release_yearly_leo",
			given: tcGiven{
				subID: "brave.leo.yearly",
				set:   newOrderItemReqNewLeoSet("development"),
			},
			exp: tcExpected{
				req: model.OrderItemRequestNew{
					Quantity:                    1,
					IssuerTokenBuffer:           3,
					SKU:                         "brave-leo-premium-year",
					Location:                    "leo.brave.software",
					Description:                 "Premium access to Leo Yearly",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1Y",
					Price:                       decimal.RequireFromString("150.00"),
					CredentialValidDurationEach: ptrTo("P1D"),
					IssuanceInterval:            ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_OtZCXOCIO3AJE6",
						ItemID:    "price_1O6re8Hof20bphG6tqdNEEAp",
					},
				},
			},
		},

		{
			name: "ios_yearly_leo",
			given: tcGiven{
				subID: "braveleo.yearly",
				set:   newOrderItemReqNewLeoSet("development"),
			},
			exp: tcExpected{
				req: model.OrderItemRequestNew{
					Quantity:                    1,
					IssuerTokenBuffer:           3,
					SKU:                         "brave-leo-premium-year",
					Location:                    "leo.brave.software",
					Description:                 "Premium access to Leo Yearly",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1Y",
					Price:                       decimal.RequireFromString("150.00"),
					CredentialValidDurationEach: ptrTo("P1D"),
					IssuanceInterval:            ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_OtZCXOCIO3AJE6",
						ItemID:    "price_1O6re8Hof20bphG6tqdNEEAp",
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, err := newOrderItemReqForSubID(tc.given.set, tc.given.subID)
			must.Equal(t, tc.exp.err, err)

			should.Equal(t, tc.exp.req, actual)
		})
	}
}

func TestNewCreateOrderReqNewLeo(t *testing.T) {
	type tcGiven struct {
		ppcfg *premiumPaymentProcConfig
		item  model.OrderItemRequestNew
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   model.CreateOrderRequestNew
	}

	tests := []testCase{
		{
			name: "development_leo_monthly",
			given: tcGiven{
				ppcfg: newPaymentProcessorConfig("development"),
				item:  newOrderItemReqNewLeoSet("development")["brave-leo-premium"],
			},

			exp: model.CreateOrderRequestNew{
				Currency: "USD",

				StripeMetadata: &model.OrderStripeMetadata{
					SuccessURI: "https://account.brave.software/account/?intent=provision",
					CancelURI:  "https://account.brave.software/plans/?intent=checkout",
				},
				PaymentMethods: []string{"stripe"},

				Items: []model.OrderItemRequestNew{
					{
						Quantity:                    1,
						IssuerTokenBuffer:           3,
						SKU:                         "brave-leo-premium",
						Location:                    "leo.brave.software",
						Description:                 "Premium access to Leo",
						CredentialType:              "time-limited-v2",
						CredentialValidDuration:     "P1M",
						Price:                       decimal.RequireFromString("15.00"),
						CredentialValidDurationEach: ptrTo("P1D"),
						IssuanceInterval:            ptrTo("P1D"),
						StripeMetadata: &model.ItemStripeMetadata{
							ProductID: "prod_OtZCXOCIO3AJE6",
							ItemID:    "price_1O5m3lHof20bphG6DloANAcc",
						},
					},
				},
			},
		},

		{
			name: "staging_leo_yearly",
			given: tcGiven{
				ppcfg: newPaymentProcessorConfig("staging"),
				item:  newOrderItemReqNewLeoSet("staging")["brave-leo-premium-year"],
			},
			exp: model.CreateOrderRequestNew{
				Currency: "USD",

				StripeMetadata: &model.OrderStripeMetadata{
					SuccessURI: "https://account.bravesoftware.com/account/?intent=provision",
					CancelURI:  "https://account.bravesoftware.com/plans/?intent=checkout",
				},
				PaymentMethods: []string{"stripe"},

				Items: []model.OrderItemRequestNew{
					{
						Quantity:                    1,
						IssuerTokenBuffer:           3,
						SKU:                         "brave-leo-premium-year",
						Location:                    "leo.bravesoftware.com",
						Description:                 "Premium access to Leo Yearly",
						CredentialType:              "time-limited-v2",
						CredentialValidDuration:     "P1Y",
						Price:                       decimal.RequireFromString("150.00"),
						CredentialValidDurationEach: ptrTo("P1D"),
						IssuanceInterval:            ptrTo("P1D"),
						StripeMetadata: &model.ItemStripeMetadata{
							ProductID: "prod_OKRYJ77wYOk771",
							ItemID:    "price_1NXmfTBSm1mtrN9nybnyolId",
						},
					},
				},
			},
		},

		{
			name: "production_leo_monthly",
			given: tcGiven{
				ppcfg: newPaymentProcessorConfig("production"),
				item:  newOrderItemReqNewLeoSet("production")["brave-leo-premium"],
			},
			exp: model.CreateOrderRequestNew{
				Currency: "USD",

				StripeMetadata: &model.OrderStripeMetadata{
					SuccessURI: "https://account.brave.com/account/?intent=provision",
					CancelURI:  "https://account.brave.com/plans/?intent=checkout",
				},
				PaymentMethods: []string{"stripe"},

				Items: []model.OrderItemRequestNew{
					{
						Quantity:                    1,
						IssuerTokenBuffer:           3,
						SKU:                         "brave-leo-premium",
						Location:                    "leo.brave.com",
						Description:                 "Premium access to Leo",
						CredentialType:              "time-limited-v2",
						CredentialValidDuration:     "P1M",
						Price:                       decimal.RequireFromString("15.00"),
						CredentialValidDurationEach: ptrTo("P1D"),
						IssuanceInterval:            ptrTo("P1D"),
						StripeMetadata: &model.ItemStripeMetadata{
							ProductID: "prod_O9uKDYsRPXNgfB",
							ItemID:    "price_1NXmj0BSm1mtrN9nF0elIhiq",
						},
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := newCreateOrderReqNewLeo(tc.given.ppcfg, tc.given.item)
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestNewOrderItemReqNewLeoSet(t *testing.T) {
	type testCase struct {
		name  string
		given string
		exp   map[string]model.OrderItemRequestNew
	}

	tests := []testCase{
		{
			name:  "production",
			given: "production",
			exp: map[string]model.OrderItemRequestNew{
				"brave-leo-premium": model.OrderItemRequestNew{
					Quantity:                    1,
					IssuerTokenBuffer:           3,
					SKU:                         "brave-leo-premium",
					Location:                    "leo.brave.com",
					Description:                 "Premium access to Leo",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1M",
					Price:                       decimal.RequireFromString("15.00"),
					CredentialValidDurationEach: ptrTo("P1D"),
					IssuanceInterval:            ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_O9uKDYsRPXNgfB",
						ItemID:    "price_1NXmj0BSm1mtrN9nF0elIhiq",
					},
				},

				"brave-leo-premium-year": model.OrderItemRequestNew{
					Quantity:                    1,
					IssuerTokenBuffer:           3,
					SKU:                         "brave-leo-premium-year",
					Location:                    "leo.brave.com",
					Description:                 "Premium access to Leo Yearly",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1Y",
					Price:                       decimal.RequireFromString("150.00"),
					CredentialValidDurationEach: ptrTo("P1D"),
					IssuanceInterval:            ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_O9uKDYsRPXNgfB",
						ItemID:    "price_1NXmfTBSm1mtrN9nybnyolId",
					},
				},
			},
		},

		{
			name:  "staging",
			given: "staging",
			exp: map[string]model.OrderItemRequestNew{
				"brave-leo-premium": model.OrderItemRequestNew{
					Quantity:                    1,
					IssuerTokenBuffer:           3,
					SKU:                         "brave-leo-premium",
					Location:                    "leo.bravesoftware.com",
					Description:                 "Premium access to Leo",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1M",
					Price:                       decimal.RequireFromString("15.00"),
					CredentialValidDurationEach: ptrTo("P1D"),
					IssuanceInterval:            ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_OKRYJ77wYOk771",
						ItemID:    "price_1NXmfTBSm1mtrN9nYjSNMs4X",
					},
				},

				"brave-leo-premium-year": model.OrderItemRequestNew{
					Quantity:                    1,
					IssuerTokenBuffer:           3,
					SKU:                         "brave-leo-premium-year",
					Location:                    "leo.bravesoftware.com",
					Description:                 "Premium access to Leo Yearly",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1Y",
					Price:                       decimal.RequireFromString("150.00"),
					CredentialValidDurationEach: ptrTo("P1D"),
					IssuanceInterval:            ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_OKRYJ77wYOk771",
						ItemID:    "price_1NXmfTBSm1mtrN9nybnyolId",
					},
				},
			},
		},

		{
			name:  "development",
			given: "development",
			exp: map[string]model.OrderItemRequestNew{
				"brave-leo-premium": model.OrderItemRequestNew{
					Quantity:                    1,
					IssuerTokenBuffer:           3,
					SKU:                         "brave-leo-premium",
					Location:                    "leo.brave.software",
					Description:                 "Premium access to Leo",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1M",
					Price:                       decimal.RequireFromString("15.00"),
					CredentialValidDurationEach: ptrTo("P1D"),
					IssuanceInterval:            ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_OtZCXOCIO3AJE6",
						ItemID:    "price_1O5m3lHof20bphG6DloANAcc",
					},
				},

				"brave-leo-premium-year": model.OrderItemRequestNew{
					Quantity:                    1,
					IssuerTokenBuffer:           3,
					SKU:                         "brave-leo-premium-year",
					Location:                    "leo.brave.software",
					Description:                 "Premium access to Leo Yearly",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1Y",
					Price:                       decimal.RequireFromString("150.00"),
					CredentialValidDurationEach: ptrTo("P1D"),
					IssuanceInterval:            ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_OtZCXOCIO3AJE6",
						ItemID:    "price_1O6re8Hof20bphG6tqdNEEAp",
					},
				},
			},
		},

		{
			name:  "unrecognised_defaults_to_development",
			given: "garbage_environment",
			exp: map[string]model.OrderItemRequestNew{
				"brave-leo-premium": model.OrderItemRequestNew{
					Quantity:                    1,
					IssuerTokenBuffer:           3,
					SKU:                         "brave-leo-premium",
					Location:                    "leo.brave.software",
					Description:                 "Premium access to Leo",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1M",
					Price:                       decimal.RequireFromString("15.00"),
					CredentialValidDurationEach: ptrTo("P1D"),
					IssuanceInterval:            ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_OtZCXOCIO3AJE6",
						ItemID:    "price_1O5m3lHof20bphG6DloANAcc",
					},
				},

				"brave-leo-premium-year": model.OrderItemRequestNew{
					Quantity:                    1,
					IssuerTokenBuffer:           3,
					SKU:                         "brave-leo-premium-year",
					Location:                    "leo.brave.software",
					Description:                 "Premium access to Leo Yearly",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1Y",
					Price:                       decimal.RequireFromString("150.00"),
					CredentialValidDurationEach: ptrTo("P1D"),
					IssuanceInterval:            ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_OtZCXOCIO3AJE6",
						ItemID:    "price_1O6re8Hof20bphG6tqdNEEAp",
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := newOrderItemReqNewLeoSet(tc.given)
			should.Equal(t, tc.exp, actual)
		})
	}
}
