package model_test

import (
	"errors"
	"testing"

	"github.com/lib/pq"
	should "github.com/stretchr/testify/assert"

	"github.com/brave-intl/bat-go/services/skus/model"
)

func TestOrder_IsStripePayable(t *testing.T) {
	type testCase struct {
		name  string
		given model.Order
		exp   bool
	}

	tests := []testCase{
		{
			name: "empty",
		},

		{
			name:  "something_else",
			given: model.Order{AllowedPaymentMethods: pq.StringArray{"something_else"}},
		},

		{
			name:  "stripe_only",
			given: model.Order{AllowedPaymentMethods: pq.StringArray{"stripe"}},
			exp:   true,
		},

		{
			name:  "something_else_stripe",
			given: model.Order{AllowedPaymentMethods: pq.StringArray{"something_else", "stripe"}},
			exp:   true,
		},

		{
			name:  "stripe_something_else",
			given: model.Order{AllowedPaymentMethods: pq.StringArray{"stripe", "something_else"}},
			exp:   true,
		},

		{
			name:  "more_stripe_something_else",
			given: model.Order{AllowedPaymentMethods: pq.StringArray{"more", "stripe", "something_else"}},
			exp:   true,
		},

		{
			name:  "mixed",
			given: model.Order{AllowedPaymentMethods: pq.StringArray{"more", "stripe", "something_else", "stripe"}},
			exp:   true,
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			act := tc.given.IsStripePayable()
			should.Equal(t, tc.exp, act)
		})
	}
}

func TestEnsureEqualPaymentMethods(t *testing.T) {
	type tcGiven struct {
		a []string
		b []string
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   error
	}

	tests := []testCase{
		{
			name: "empty",
		},

		{
			name: "stripe_empty",
			given: tcGiven{
				a: []string{"stripe"},
			},
			exp: model.ErrDifferentPaymentMethods,
		},

		{
			name: "stripe_something",
			given: tcGiven{
				a: []string{"stripe"},
				b: []string{"something"},
			},
			exp: model.ErrDifferentPaymentMethods,
		},

		{
			name: "equal_single",
			given: tcGiven{
				a: []string{"stripe"},
				b: []string{"stripe"},
			},
		},

		{
			name: "equal_sorting",
			given: tcGiven{
				a: []string{"cash", "stripe"},
				b: []string{"stripe", "cash"},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			act := model.EnsureEqualPaymentMethods(tc.given.a, tc.given.b)
			should.Equal(t, true, errors.Is(tc.exp, act))
		})
	}
}
