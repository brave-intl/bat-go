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
