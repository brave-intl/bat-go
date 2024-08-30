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
			name:  "android_leo_monthly_release",
			given: "brave.leo.monthly",
			exp:   tcExpected{sku: "brave-leo-premium"},
		},

		{
			name:  "android_leo_monthly_beta",
			given: "beta.leo.monthly",
			exp:   tcExpected{sku: "brave-leo-premium"},
		},

		{
			name:  "android_leo_monthly_nightly",
			given: "nightly.leo.monthly",
			exp:   tcExpected{sku: "brave-leo-premium"},
		},

		{
			name:  "ios_leo_monthly_release",
			given: "braveleo.monthly",
			exp:   tcExpected{sku: "brave-leo-premium"},
		},

		{
			name:  "ios_leo_monthly_nightly",
			given: "nightly.braveleo.monthly",
			exp:   tcExpected{sku: "brave-leo-premium"},
		},

		{
			name:  "android_leo_annual_release",
			given: "brave.leo.yearly",
			exp:   tcExpected{sku: "brave-leo-premium-year"},
		},

		{
			name:  "android_leo_annual_beta",
			given: "beta.leo.yearly",
			exp:   tcExpected{sku: "brave-leo-premium-year"},
		},

		{
			name:  "android_leo_annual_nightly",
			given: "nightly.leo.yearly",
			exp:   tcExpected{sku: "brave-leo-premium-year"},
		},

		{
			name:  "ios_leo_annual_release",
			given: "braveleo.yearly",
			exp:   tcExpected{sku: "brave-leo-premium-year"},
		},

		{
			name:  "ios_leo_annual_nightly",
			given: "nightly.braveleo.yearly",
			exp:   tcExpected{sku: "brave-leo-premium-year"},
		},

		{
			name:  "android_vpn_monthly_release",
			given: "brave.vpn.monthly",
			exp:   tcExpected{sku: "brave-vpn-premium"},
		},

		{
			name:  "android_vpn_monthly_beta",
			given: "beta.bravevpn.monthly",
			exp:   tcExpected{sku: "brave-vpn-premium"},
		},

		{
			name:  "android_vpn_monthly_nightly",
			given: "nightly.bravevpn.monthly",
			exp:   tcExpected{sku: "brave-vpn-premium"},
		},

		{
			name:  "ios_vpn_monthly_release",
			given: "bravevpn.monthly",
			exp:   tcExpected{sku: "brave-vpn-premium"},
		},

		{
			name:  "android_vpn_annual_release",
			given: "brave.vpn.yearly",
			exp:   tcExpected{sku: "brave-vpn-premium"},
		},

		{
			name:  "android_vpn_annual_beta",
			given: "beta.bravevpn.yearly",
			exp:   tcExpected{sku: "brave-vpn-premium"},
		},

		{
			name:  "android_vpn_annual_nightly",
			given: "nightly.bravevpn.yearly",
			exp:   tcExpected{sku: "brave-vpn-premium"},
		},

		{
			name:  "ios_vpn_annual_release",
			given: "bravevpn.yearly",
			exp:   tcExpected{sku: "brave-vpn-premium"},
		},

		{
			name:  "invalid",
			given: "something_else",
			exp:   tcExpected{err: model.ErrInvalidMobileProduct},
		},

		{
			name:  "ios_vpn_monthly_legacy",
			given: "brave-firewall-vpn-premium",
			exp:   tcExpected{sku: "brave-vpn-premium"},
		},

		{
			name:  "ios_vpn_annual_legacy",
			given: "brave-firewall-vpn-premium-year",
			exp:   tcExpected{sku: "brave-vpn-premium"},
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
				set:   newOrderItemReqNewMobileSet("development"),
			},
			exp: tcExpected{
				req: model.OrderItemRequestNew{
					Quantity:                    1,
					SKU:                         "brave-leo-premium",
					Location:                    "leo.brave.software",
					Description:                 "Premium access to Leo",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1M",
					Price:                       decimal.RequireFromString("14.99"),
					IssuerTokenBuffer:           ptrTo(3),
					IssuerTokenOverlap:          ptrTo(0),
					CredentialValidDurationEach: ptrTo("P1D"),
					IssuanceInterval:            ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_OtZCXOCIO3AJE6",
						ItemID:    "price_1OuRqmHof20bphG6RXl7EHP2",
					},
				},
			},
		},

		{
			name: "ios_monthly_leo",
			given: tcGiven{
				subID: "braveleo.monthly",
				set:   newOrderItemReqNewMobileSet("development"),
			},
			exp: tcExpected{
				req: model.OrderItemRequestNew{
					Quantity:                    1,
					SKU:                         "brave-leo-premium",
					Location:                    "leo.brave.software",
					Description:                 "Premium access to Leo",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1M",
					Price:                       decimal.RequireFromString("14.99"),
					IssuerTokenBuffer:           ptrTo(3),
					IssuerTokenOverlap:          ptrTo(0),
					CredentialValidDurationEach: ptrTo("P1D"),
					IssuanceInterval:            ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_OtZCXOCIO3AJE6",
						ItemID:    "price_1OuRqmHof20bphG6RXl7EHP2",
					},
				},
			},
		},

		{
			name: "android_release_yearly_leo",
			given: tcGiven{
				subID: "brave.leo.yearly",
				set:   newOrderItemReqNewMobileSet("development"),
			},
			exp: tcExpected{
				req: model.OrderItemRequestNew{
					Quantity:                    1,
					SKU:                         "brave-leo-premium-year",
					Location:                    "leo.brave.software",
					Description:                 "Premium access to Leo Yearly",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1Y",
					Price:                       decimal.RequireFromString("149.99"),
					IssuerTokenBuffer:           ptrTo(3),
					IssuerTokenOverlap:          ptrTo(0),
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
				set:   newOrderItemReqNewMobileSet("development"),
			},
			exp: tcExpected{
				req: model.OrderItemRequestNew{
					Quantity:                    1,
					SKU:                         "brave-leo-premium-year",
					Location:                    "leo.brave.software",
					Description:                 "Premium access to Leo Yearly",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1Y",
					Price:                       decimal.RequireFromString("149.99"),
					IssuerTokenBuffer:           ptrTo(3),
					IssuerTokenOverlap:          ptrTo(0),
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
			name: "android_release_monthly_vpn",
			given: tcGiven{
				subID: "brave.vpn.monthly",
				set:   newOrderItemReqNewMobileSet("development"),
			},
			exp: tcExpected{
				req: model.OrderItemRequestNew{
					Quantity:                    1,
					SKU:                         "brave-vpn-premium",
					Location:                    "vpn.brave.software",
					Description:                 "brave-vpn-premium",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1M",
					Price:                       decimal.RequireFromString("9.99"),
					IssuerTokenBuffer:           ptrTo(31),
					IssuerTokenOverlap:          ptrTo(2),
					CredentialValidDurationEach: ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_K1c8W3oM4mUsGw",
						ItemID:    "price_1JNYuNHof20bphG6BvgeYEnt",
					},
				},
			},
		},

		{
			name: "ios_monthly_vpn",
			given: tcGiven{
				subID: "bravevpn.monthly",
				set:   newOrderItemReqNewMobileSet("development"),
			},
			exp: tcExpected{
				req: model.OrderItemRequestNew{
					Quantity:                    1,
					SKU:                         "brave-vpn-premium",
					Location:                    "vpn.brave.software",
					Description:                 "brave-vpn-premium",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1M",
					Price:                       decimal.RequireFromString("9.99"),
					IssuerTokenBuffer:           ptrTo(31),
					IssuerTokenOverlap:          ptrTo(2),
					CredentialValidDurationEach: ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_K1c8W3oM4mUsGw",
						ItemID:    "price_1JNYuNHof20bphG6BvgeYEnt",
					},
				},
			},
		},

		{
			name: "android_release_yearly_vpn",
			given: tcGiven{
				subID: "brave.vpn.yearly",
				set:   newOrderItemReqNewMobileSet("development"),
			},
			exp: tcExpected{
				req: model.OrderItemRequestNew{
					Quantity:                    1,
					SKU:                         "brave-vpn-premium",
					Location:                    "vpn.brave.software",
					Description:                 "brave-vpn-premium",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1M",
					Price:                       decimal.RequireFromString("9.99"),
					IssuerTokenBuffer:           ptrTo(31),
					IssuerTokenOverlap:          ptrTo(2),
					CredentialValidDurationEach: ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_K1c8W3oM4mUsGw",
						ItemID:    "price_1JNYuNHof20bphG6BvgeYEnt",
					},
				},
			},
		},

		{
			name: "ios_yearly_vpn",
			given: tcGiven{
				subID: "bravevpn.yearly",
				set:   newOrderItemReqNewMobileSet("development"),
			},
			exp: tcExpected{
				req: model.OrderItemRequestNew{
					Quantity:                    1,
					SKU:                         "brave-vpn-premium",
					Location:                    "vpn.brave.software",
					Description:                 "brave-vpn-premium",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1M",
					Price:                       decimal.RequireFromString("9.99"),
					IssuerTokenBuffer:           ptrTo(31),
					IssuerTokenOverlap:          ptrTo(2),
					CredentialValidDurationEach: ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_K1c8W3oM4mUsGw",
						ItemID:    "price_1JNYuNHof20bphG6BvgeYEnt",
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

func TestNewCreateOrderReqNewMobile(t *testing.T) {
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
				item:  newOrderItemReqNewMobileSet("development")["brave-leo-premium"],
			},

			exp: model.CreateOrderRequestNew{
				Currency: "USD",

				StripeMetadata: &model.OrderStripeMetadata{
					SuccessURI: "https://account.brave.software/account/?intent=provision",
					CancelURI:  "https://account.brave.software/plans/?intent=checkout",
				},

				Items: []model.OrderItemRequestNew{
					{
						Quantity:                    1,
						SKU:                         "brave-leo-premium",
						Location:                    "leo.brave.software",
						Description:                 "Premium access to Leo",
						CredentialType:              "time-limited-v2",
						CredentialValidDuration:     "P1M",
						Price:                       decimal.RequireFromString("14.99"),
						IssuerTokenBuffer:           ptrTo(3),
						IssuerTokenOverlap:          ptrTo(0),
						CredentialValidDurationEach: ptrTo("P1D"),
						IssuanceInterval:            ptrTo("P1D"),
						StripeMetadata: &model.ItemStripeMetadata{
							ProductID: "prod_OtZCXOCIO3AJE6",
							ItemID:    "price_1OuRqmHof20bphG6RXl7EHP2",
						},
					},
				},
			},
		},

		{
			name: "staging_leo_yearly",
			given: tcGiven{
				ppcfg: newPaymentProcessorConfig("staging"),
				item:  newOrderItemReqNewMobileSet("staging")["brave-leo-premium-year"],
			},
			exp: model.CreateOrderRequestNew{
				Currency: "USD",

				StripeMetadata: &model.OrderStripeMetadata{
					SuccessURI: "https://account.bravesoftware.com/account/?intent=provision",
					CancelURI:  "https://account.bravesoftware.com/plans/?intent=checkout",
				},

				Items: []model.OrderItemRequestNew{
					{
						Quantity:                    1,
						SKU:                         "brave-leo-premium-year",
						Location:                    "leo.bravesoftware.com",
						Description:                 "Premium access to Leo Yearly",
						CredentialType:              "time-limited-v2",
						CredentialValidDuration:     "P1Y",
						Price:                       decimal.RequireFromString("149.99"),
						IssuerTokenBuffer:           ptrTo(3),
						IssuerTokenOverlap:          ptrTo(0),
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
				item:  newOrderItemReqNewMobileSet("production")["brave-leo-premium"],
			},
			exp: model.CreateOrderRequestNew{
				Currency: "USD",

				StripeMetadata: &model.OrderStripeMetadata{
					SuccessURI: "https://account.brave.com/account/?intent=provision",
					CancelURI:  "https://account.brave.com/plans/?intent=checkout",
				},

				Items: []model.OrderItemRequestNew{
					{
						Quantity:                    1,
						SKU:                         "brave-leo-premium",
						Location:                    "leo.brave.com",
						Description:                 "Premium access to Leo",
						CredentialType:              "time-limited-v2",
						CredentialValidDuration:     "P1M",
						Price:                       decimal.RequireFromString("14.99"),
						IssuerTokenBuffer:           ptrTo(3),
						IssuerTokenOverlap:          ptrTo(0),
						CredentialValidDurationEach: ptrTo("P1D"),
						IssuanceInterval:            ptrTo("P1D"),
						StripeMetadata: &model.ItemStripeMetadata{
							ProductID: "prod_O9uKDYsRPXNgfB",
							ItemID:    "price_1OoS8YBSm1mtrN9nB5gKoYwh",
						},
					},
				},
			},
		},

		{
			name: "development_vpn_monthly",
			given: tcGiven{
				ppcfg: newPaymentProcessorConfig("development"),
				item:  newOrderItemReqNewMobileSet("development")["brave-vpn-premium"],
			},

			exp: model.CreateOrderRequestNew{
				Currency: "USD",

				StripeMetadata: &model.OrderStripeMetadata{
					SuccessURI: "https://account.brave.software/account/?intent=provision",
					CancelURI:  "https://account.brave.software/plans/?intent=checkout",
				},

				Items: []model.OrderItemRequestNew{
					{
						Quantity:                    1,
						SKU:                         "brave-vpn-premium",
						Location:                    "vpn.brave.software",
						Description:                 "brave-vpn-premium",
						CredentialType:              "time-limited-v2",
						CredentialValidDuration:     "P1M",
						Price:                       decimal.RequireFromString("9.99"),
						IssuerTokenBuffer:           ptrTo(31),
						IssuerTokenOverlap:          ptrTo(2),
						CredentialValidDurationEach: ptrTo("P1D"),
						StripeMetadata: &model.ItemStripeMetadata{
							ProductID: "prod_K1c8W3oM4mUsGw",
							ItemID:    "price_1JNYuNHof20bphG6BvgeYEnt",
						},
					},
				},
			},
		},

		{
			name: "staging_vpn_yearly",
			given: tcGiven{
				ppcfg: newPaymentProcessorConfig("staging"),
				item:  newOrderItemReqNewMobileSet("staging")["brave-vpn-premium"],
			},
			exp: model.CreateOrderRequestNew{
				Currency: "USD",

				StripeMetadata: &model.OrderStripeMetadata{
					SuccessURI: "https://account.bravesoftware.com/account/?intent=provision",
					CancelURI:  "https://account.bravesoftware.com/plans/?intent=checkout",
				},

				Items: []model.OrderItemRequestNew{
					{
						Quantity:                    1,
						SKU:                         "brave-vpn-premium",
						Location:                    "vpn.bravesoftware.com",
						Description:                 "brave-vpn-premium",
						CredentialType:              "time-limited-v2",
						CredentialValidDuration:     "P1M",
						Price:                       decimal.RequireFromString("9.99"),
						IssuerTokenBuffer:           ptrTo(31),
						IssuerTokenOverlap:          ptrTo(2),
						CredentialValidDurationEach: ptrTo("P1D"),
						StripeMetadata: &model.ItemStripeMetadata{
							ProductID: "prod_Lhv4OM1aAPxflY",
							ItemID:    "price_1L0VEhBSm1mtrN9nGB4kZkfh",
						},
					},
				},
			},
		},

		{
			name: "production_vpn_monthly",
			given: tcGiven{
				ppcfg: newPaymentProcessorConfig("production"),
				item:  newOrderItemReqNewMobileSet("production")["brave-vpn-premium"],
			},
			exp: model.CreateOrderRequestNew{
				Currency: "USD",

				StripeMetadata: &model.OrderStripeMetadata{
					SuccessURI: "https://account.brave.com/account/?intent=provision",
					CancelURI:  "https://account.brave.com/plans/?intent=checkout",
				},

				Items: []model.OrderItemRequestNew{
					{
						Quantity:                    1,
						SKU:                         "brave-vpn-premium",
						Location:                    "vpn.brave.com",
						Description:                 "brave-vpn-premium",
						CredentialType:              "time-limited-v2",
						CredentialValidDuration:     "P1M",
						Price:                       decimal.RequireFromString("9.99"),
						IssuerTokenBuffer:           ptrTo(31),
						IssuerTokenOverlap:          ptrTo(2),
						CredentialValidDurationEach: ptrTo("P1D"),
						StripeMetadata: &model.ItemStripeMetadata{
							ProductID: "prod_Lhv8qsPsn6WHrx",
							ItemID:    "price_1L0VHmBSm1mtrN9nT5DPmUZb",
						},
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := newCreateOrderReqNewMobile(tc.given.ppcfg, tc.given.item)
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestNewOrderItemReqNewMobileSet(t *testing.T) {
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
					SKU:                         "brave-leo-premium",
					Location:                    "leo.brave.com",
					Description:                 "Premium access to Leo",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1M",
					Price:                       decimal.RequireFromString("14.99"),
					IssuerTokenBuffer:           ptrTo(3),
					IssuerTokenOverlap:          ptrTo(0),
					CredentialValidDurationEach: ptrTo("P1D"),
					IssuanceInterval:            ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_O9uKDYsRPXNgfB",
						ItemID:    "price_1OoS8YBSm1mtrN9nB5gKoYwh",
					},
				},

				"brave-leo-premium-year": model.OrderItemRequestNew{
					Quantity:                    1,
					SKU:                         "brave-leo-premium-year",
					Location:                    "leo.brave.com",
					Description:                 "Premium access to Leo Yearly",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1Y",
					Price:                       decimal.RequireFromString("149.99"),
					IssuerTokenBuffer:           ptrTo(3),
					IssuerTokenOverlap:          ptrTo(0),
					CredentialValidDurationEach: ptrTo("P1D"),
					IssuanceInterval:            ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_O9uKDYsRPXNgfB",
						ItemID:    "price_1NXmfTBSm1mtrN9nybnyolId",
					},
				},

				"brave-vpn-premium": model.OrderItemRequestNew{
					Quantity:                    1,
					SKU:                         "brave-vpn-premium",
					Location:                    "vpn.brave.com",
					Description:                 "brave-vpn-premium",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1M",
					Price:                       decimal.RequireFromString("9.99"),
					IssuerTokenBuffer:           ptrTo(31),
					IssuerTokenOverlap:          ptrTo(2),
					CredentialValidDurationEach: ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_Lhv8qsPsn6WHrx",
						ItemID:    "price_1L0VHmBSm1mtrN9nT5DPmUZb",
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
					SKU:                         "brave-leo-premium",
					Location:                    "leo.bravesoftware.com",
					Description:                 "Premium access to Leo",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1M",
					Price:                       decimal.RequireFromString("14.99"),
					IssuerTokenBuffer:           ptrTo(3),
					IssuerTokenOverlap:          ptrTo(0),
					CredentialValidDurationEach: ptrTo("P1D"),
					IssuanceInterval:            ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_OKRYJ77wYOk771",
						ItemID:    "price_1OuRuUBSm1mtrN9nWFtJYSML",
					},
				},

				"brave-leo-premium-year": model.OrderItemRequestNew{
					Quantity:                    1,
					SKU:                         "brave-leo-premium-year",
					Location:                    "leo.bravesoftware.com",
					Description:                 "Premium access to Leo Yearly",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1Y",
					Price:                       decimal.RequireFromString("149.99"),
					IssuerTokenBuffer:           ptrTo(3),
					IssuerTokenOverlap:          ptrTo(0),
					CredentialValidDurationEach: ptrTo("P1D"),
					IssuanceInterval:            ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_OKRYJ77wYOk771",
						ItemID:    "price_1NXmfTBSm1mtrN9nybnyolId",
					},
				},

				"brave-vpn-premium": model.OrderItemRequestNew{
					Quantity:                    1,
					SKU:                         "brave-vpn-premium",
					Location:                    "vpn.bravesoftware.com",
					Description:                 "brave-vpn-premium",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1M",
					Price:                       decimal.RequireFromString("9.99"),
					IssuerTokenBuffer:           ptrTo(31),
					IssuerTokenOverlap:          ptrTo(2),
					CredentialValidDurationEach: ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_Lhv4OM1aAPxflY",
						ItemID:    "price_1L0VEhBSm1mtrN9nGB4kZkfh",
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
					SKU:                         "brave-leo-premium",
					Location:                    "leo.brave.software",
					Description:                 "Premium access to Leo",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1M",
					Price:                       decimal.RequireFromString("14.99"),
					IssuerTokenBuffer:           ptrTo(3),
					IssuerTokenOverlap:          ptrTo(0),
					CredentialValidDurationEach: ptrTo("P1D"),
					IssuanceInterval:            ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_OtZCXOCIO3AJE6",
						ItemID:    "price_1OuRqmHof20bphG6RXl7EHP2",
					},
				},

				"brave-leo-premium-year": model.OrderItemRequestNew{
					Quantity:                    1,
					SKU:                         "brave-leo-premium-year",
					Location:                    "leo.brave.software",
					Description:                 "Premium access to Leo Yearly",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1Y",
					Price:                       decimal.RequireFromString("149.99"),
					IssuerTokenBuffer:           ptrTo(3),
					IssuerTokenOverlap:          ptrTo(0),
					CredentialValidDurationEach: ptrTo("P1D"),
					IssuanceInterval:            ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_OtZCXOCIO3AJE6",
						ItemID:    "price_1O6re8Hof20bphG6tqdNEEAp",
					},
				},

				"brave-vpn-premium": model.OrderItemRequestNew{
					Quantity:                    1,
					SKU:                         "brave-vpn-premium",
					Location:                    "vpn.brave.software",
					Description:                 "brave-vpn-premium",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1M",
					Price:                       decimal.RequireFromString("9.99"),
					IssuerTokenBuffer:           ptrTo(31),
					IssuerTokenOverlap:          ptrTo(2),
					CredentialValidDurationEach: ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_K1c8W3oM4mUsGw",
						ItemID:    "price_1JNYuNHof20bphG6BvgeYEnt",
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
					SKU:                         "brave-leo-premium",
					Location:                    "leo.brave.software",
					Description:                 "Premium access to Leo",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1M",
					Price:                       decimal.RequireFromString("14.99"),
					IssuerTokenBuffer:           ptrTo(3),
					IssuerTokenOverlap:          ptrTo(0),
					CredentialValidDurationEach: ptrTo("P1D"),
					IssuanceInterval:            ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_OtZCXOCIO3AJE6",
						ItemID:    "price_1OuRqmHof20bphG6RXl7EHP2",
					},
				},

				"brave-leo-premium-year": model.OrderItemRequestNew{
					Quantity:                    1,
					SKU:                         "brave-leo-premium-year",
					Location:                    "leo.brave.software",
					Description:                 "Premium access to Leo Yearly",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1Y",
					Price:                       decimal.RequireFromString("149.99"),
					IssuerTokenBuffer:           ptrTo(3),
					IssuerTokenOverlap:          ptrTo(0),
					CredentialValidDurationEach: ptrTo("P1D"),
					IssuanceInterval:            ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_OtZCXOCIO3AJE6",
						ItemID:    "price_1O6re8Hof20bphG6tqdNEEAp",
					},
				},

				"brave-vpn-premium": model.OrderItemRequestNew{
					Quantity:                    1,
					SKU:                         "brave-vpn-premium",
					Location:                    "vpn.brave.software",
					Description:                 "brave-vpn-premium",
					CredentialType:              "time-limited-v2",
					CredentialValidDuration:     "P1M",
					Price:                       decimal.RequireFromString("9.99"),
					IssuerTokenBuffer:           ptrTo(31),
					IssuerTokenOverlap:          ptrTo(2),
					CredentialValidDurationEach: ptrTo("P1D"),
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "prod_K1c8W3oM4mUsGw",
						ItemID:    "price_1JNYuNHof20bphG6BvgeYEnt",
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := newOrderItemReqNewMobileSet(tc.given)
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestPaymentProcessorConfig(t *testing.T) {
	type testCase struct {
		name     string
		env      string
		expected *premiumPaymentProcConfig
	}

	tests := []testCase{
		{
			name: "prod",
			env:  "prod",
			expected: &premiumPaymentProcConfig{
				successURI: "https://account.brave.com/account/?intent=provision",
				cancelURI:  "https://account.brave.com/plans/?intent=checkout",
			},
		},

		{
			name: "production",
			env:  "production",
			expected: &premiumPaymentProcConfig{
				successURI: "https://account.brave.com/account/?intent=provision",
				cancelURI:  "https://account.brave.com/plans/?intent=checkout",
			},
		},

		{
			name: "sandbox",
			env:  "sandbox",
			expected: &premiumPaymentProcConfig{
				successURI: "https://account.bravesoftware.com/account/?intent=provision",
				cancelURI:  "https://account.bravesoftware.com/plans/?intent=checkout",
			},
		},

		{
			name: "staging",
			env:  "staging",
			expected: &premiumPaymentProcConfig{
				successURI: "https://account.bravesoftware.com/account/?intent=provision",
				cancelURI:  "https://account.bravesoftware.com/plans/?intent=checkout",
			},
		},

		{
			name: "dev",
			env:  "dev",
			expected: &premiumPaymentProcConfig{
				successURI: "https://account.brave.software/account/?intent=provision",
				cancelURI:  "https://account.brave.software/plans/?intent=checkout",
			},
		},

		{
			name: "development",
			env:  "development",
			expected: &premiumPaymentProcConfig{
				successURI: "https://account.brave.software/account/?intent=provision",
				cancelURI:  "https://account.brave.software/plans/?intent=checkout",
			},
		},

		{
			name: "local",
			env:  "local",
			expected: &premiumPaymentProcConfig{
				successURI: "https://account.brave.software/account/?intent=provision",
				cancelURI:  "https://account.brave.software/plans/?intent=checkout",
			},
		},

		{
			name: "test",
			env:  "test",
			expected: &premiumPaymentProcConfig{
				successURI: "https://account.brave.software/account/?intent=provision",
				cancelURI:  "https://account.brave.software/plans/?intent=checkout",
			},
		},

		{
			name: "garbage",
			env:  "garbage",
			expected: &premiumPaymentProcConfig{
				successURI: "https://account.brave.software/account/?intent=provision",
				cancelURI:  "https://account.brave.software/plans/?intent=checkout",
			},
		},

		{
			name: "empty_env",
			expected: &premiumPaymentProcConfig{
				successURI: "https://account.brave.software/account/?intent=provision",
				cancelURI:  "https://account.brave.software/plans/?intent=checkout",
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := newPaymentProcessorConfig(tc.env)
			should.Equal(t, tc.expected, actual)
		})
	}
}
