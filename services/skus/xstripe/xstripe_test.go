package xstripe

import (
	"testing"

	should "github.com/stretchr/testify/assert"
	"github.com/stripe/stripe-go/v72"
)

func TestCustomerEmailFromSession(t *testing.T) {
	tests := []struct {
		name  string
		exp   string
		given *stripe.CheckoutSession
	}{
		{
			name:  "nil_customer_no_email",
			given: &stripe.CheckoutSession{},
		},

		{
			name: "customer_empty_email_no_email",
			given: &stripe.CheckoutSession{
				Customer: &stripe.Customer{},
			},
		},

		{
			name: "customer_empty_email_email",
			given: &stripe.CheckoutSession{
				Customer:      &stripe.Customer{},
				CustomerEmail: "you@example.com",
			},
			exp: "you@example.com",
		},

		{
			name: "customer_no_email",
			given: &stripe.CheckoutSession{
				Customer: &stripe.Customer{
					Email: "me@example.com",
				},
			},
			exp: "me@example.com",
		},

		{
			name: "customer_email",
			given: &stripe.CheckoutSession{
				Customer: &stripe.Customer{
					Email: "me@example.com",
				},
				CustomerEmail: "you@example.com",
			},
			exp: "me@example.com",
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := CustomerEmailFromSession(tc.given)
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestCustomerIDFromSession(t *testing.T) {
	tests := []struct {
		name  string
		exp   string
		given *stripe.CheckoutSession
	}{
		{
			name:  "nil_customer_no_email",
			given: &stripe.CheckoutSession{},
		},

		{
			name: "customer_empty_email",
			given: &stripe.CheckoutSession{
				Customer: &stripe.Customer{},
			},
		},

		{
			name: "customer_email_no_id",
			given: &stripe.CheckoutSession{
				Customer: &stripe.Customer{
					Email: "me@example.com",
				},
			},
		},

		{
			name: "customer_email_id",
			given: &stripe.CheckoutSession{
				Customer: &stripe.Customer{
					ID:    "cus_id",
					Email: "me@example.com",
				},
			},
			exp: "cus_id",
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := CustomerIDFromSession(tc.given)
			should.Equal(t, tc.exp, actual)
		})
	}
}
