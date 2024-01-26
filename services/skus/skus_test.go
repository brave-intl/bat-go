package skus

import (
	"testing"

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
