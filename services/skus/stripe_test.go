package skus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	uuid "github.com/satori/go.uuid"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v72"
)

func TestParseStripeNotification(t *testing.T) {
	// WARNING: The main purpose of this test is to check if parsing works.
	//
	// It's been discovered that SKUs uses v72 which corresponds to the API version 2020-08-27.
	// Stripe sends us webhooks in the 2020-03-02 format (v70 and v71).
	//
	// So the code in handleStripeWebhook has been unreliable all this time.
	//
	// This test makes sure that the data we need can still be obtained despite the version mismatch.
	// After the version has been updated, this test should still pass.
	t.Run("invoice_paid", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join("testdata", "stripe_invoice_paid.json"))
		must.Equal(t, nil, err)

		event := &stripe.Event{}

		{
			err := json.Unmarshal(raw, event)
			must.Equal(t, nil, err)
		}

		should.Equal(t, "invoice.paid", event.Type)

		ntf, err := parseStripeNotification(event)
		must.Equal(t, nil, err)

		should.Equal(t, "sub_1PZ6NTBSm1mtrN9nhOEgB0jm", ntf.invoice.Subscription.ID)

		must.Equal(t, 1, len(ntf.invoice.Lines.Data))

		sub := ntf.invoice.Lines.Data[0]

		should.Equal(t, "subscription", string(sub.Type))
		should.Equal(t, "sub_1PZ6NTBSm1mtrN9nhOEgB0jm", sub.Subscription)
		should.Equal(t, "f0eb952b-90df-4fd3-b079-c4ea1effb38d", sub.Metadata["orderID"])

		must.Equal(t, true, sub.Period != nil)
		should.Equal(t, int64(1720163778), sub.Period.Start)
		should.Equal(t, time.Date(2024, time.July, 5, 07, 16, 18, 0, time.UTC), time.Unix(sub.Period.Start, 0).UTC())

		should.Equal(t, int64(1720768578), sub.Period.End)
		should.Equal(t, time.Date(2024, time.July, 12, 07, 16, 18, 0, time.UTC), time.Unix(sub.Period.End, 0).UTC())
	})

	t.Run("customer_subscription_deleted", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join("testdata", "stripe_sub_deleted.json"))
		must.Equal(t, nil, err)

		event := &stripe.Event{}

		{
			err := json.Unmarshal(raw, event)
			must.Equal(t, nil, err)
		}

		should.Equal(t, "customer.subscription.deleted", event.Type)

		ntf, err := parseStripeNotification(event)
		must.Equal(t, nil, err)

		should.Equal(t, "sub_1PZ6NTBSm1mtrN9nhOEgB0jm", ntf.sub.ID)
		should.Equal(t, "f0eb952b-90df-4fd3-b079-c4ea1effb38d", ntf.sub.Metadata["orderID"])
	})
}

func TestStripeNotification_shouldProcess(t *testing.T) {
	tests := []struct {
		name  string
		given *stripeNotification
		exp   bool
	}{
		{
			name: "renew",
			given: &stripeNotification{
				raw:     &stripe.Event{Type: "invoice.paid"},
				invoice: &stripe.Invoice{},
			},
			exp: true,
		},

		{
			name: "cancel",
			given: &stripeNotification{
				raw: &stripe.Event{Type: "customer.subscription.deleted"},
				sub: &stripe.Subscription{},
			},
			exp: true,
		},

		{
			name: "skip",
			given: &stripeNotification{
				raw:     &stripe.Event{Type: "invoice.updated"},
				invoice: &stripe.Invoice{},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.shouldProcess()
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestStripeNotification_shouldRenew(t *testing.T) {
	tests := []struct {
		name  string
		given *stripeNotification
		exp   bool
	}{
		{
			name: "no_invoice_wrong_type",
			given: &stripeNotification{
				raw: &stripe.Event{Type: "something_else"},
			},
		},

		{
			name: "invoice_wrong_type",
			given: &stripeNotification{
				raw:     &stripe.Event{Type: "something_else"},
				invoice: &stripe.Invoice{},
			},
		},

		{
			name: "no_invoice_correct_type",
			given: &stripeNotification{
				raw: &stripe.Event{Type: "invoice.paid"},
			},
		},

		{
			name: "renew",
			given: &stripeNotification{
				raw:     &stripe.Event{Type: "invoice.paid"},
				invoice: &stripe.Invoice{},
			},
			exp: true,
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.shouldRenew()
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestStripeNotification_shouldCancel(t *testing.T) {
	tests := []struct {
		name  string
		given *stripeNotification
		exp   bool
	}{
		{
			name: "no_sub_wrong_type",
			given: &stripeNotification{
				raw: &stripe.Event{Type: "something_else"},
			},
		},

		{
			name: "sub_wrong_type",
			given: &stripeNotification{
				raw: &stripe.Event{Type: "something_else"},
				sub: &stripe.Subscription{},
			},
		},

		{
			name: "no_sub_correct_type",
			given: &stripeNotification{
				raw: &stripe.Event{Type: "customer.subscription.deleted"},
			},
		},

		{
			name: "cancel",
			given: &stripeNotification{
				raw: &stripe.Event{Type: "customer.subscription.deleted"},
				sub: &stripe.Subscription{},
			},
			exp: true,
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.shouldCancel()
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestStripeNotification_ntfType(t *testing.T) {
	tests := []struct {
		name  string
		given *stripeNotification
		exp   string
	}{
		{
			name: "invoice_paid",
			given: &stripeNotification{
				raw: &stripe.Event{Type: "invoice.paid"},
			},
			exp: "invoice.paid",
		},

		{
			name: "customer_subscription_deleted",
			given: &stripeNotification{
				raw: &stripe.Event{Type: "customer.subscription.deleted"},
			},
			exp: "customer.subscription.deleted",
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.ntfType()
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestStripeNotification_ntfSubType(t *testing.T) {
	tests := []struct {
		name  string
		given *stripeNotification
		exp   string
	}{
		{
			name: "invoice",
			given: &stripeNotification{
				invoice: &stripe.Invoice{},
			},
			exp: "invoice",
		},

		{
			name: "subscription",
			given: &stripeNotification{
				sub: &stripe.Subscription{},
			},
			exp: "subscription",
		},

		{
			name: "unknown_both",
			given: &stripeNotification{
				invoice: &stripe.Invoice{},
				sub:     &stripe.Subscription{},
			},
			exp: "unknown",
		},

		{
			name:  "unknown_neither",
			given: &stripeNotification{},
			exp:   "unknown",
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.ntfSubType()
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestStripeNotification_effect(t *testing.T) {
	tests := []struct {
		name  string
		given *stripeNotification
		exp   string
	}{
		{
			name: "renew",
			given: &stripeNotification{
				raw:     &stripe.Event{Type: "invoice.paid"},
				invoice: &stripe.Invoice{},
			},
			exp: "renew",
		},

		{
			name: "cancel",
			given: &stripeNotification{
				raw: &stripe.Event{Type: "customer.subscription.deleted"},
				sub: &stripe.Subscription{},
			},
			exp: "cancel",
		},

		{
			name: "skip",
			given: &stripeNotification{
				raw:     &stripe.Event{Type: "invoice.updated"},
				invoice: &stripe.Invoice{},
			},
			exp: "skip",
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.effect()
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestStripeNotification_subID(t *testing.T) {
	type tcExpected struct {
		val string
		err error
	}

	type testCase struct {
		name  string
		given *stripeNotification
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "invoice_no_sub",
			given: &stripeNotification{
				raw:     &stripe.Event{Type: "invoice.paid"},
				invoice: &stripe.Invoice{},
			},
			exp: tcExpected{
				err: errStripeNoInvoiceSub,
			},
		},

		{
			name: "invoice_sub",
			given: &stripeNotification{
				raw: &stripe.Event{Type: "invoice.paid"},
				invoice: &stripe.Invoice{
					Subscription: &stripe.Subscription{ID: "sub_id"},
				},
			},
			exp: tcExpected{
				val: "sub_id",
			},
		},

		{
			name: "sub",
			given: &stripeNotification{
				raw: &stripe.Event{Type: "customer.subscription.deleted"},
				sub: &stripe.Subscription{ID: "sub_id"},
			},
			exp: tcExpected{
				val: "sub_id",
			},
		},

		{
			name: "unsupported",
			given: &stripeNotification{
				raw: &stripe.Event{Type: "unsupported"},
			},
			exp: tcExpected{
				err: errStripeUnsupportedEvent,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, err := tc.given.subID()
			must.Equal(t, tc.exp.err, err)

			should.Equal(t, tc.exp.val, actual)
		})
	}
}

func TestStripeNotification_orderID(t *testing.T) {
	type tcExpected struct {
		val uuid.UUID
		err error
	}

	type testCase struct {
		name  string
		given *stripeNotification
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "invoice_no_lines",
			given: &stripeNotification{
				raw:     &stripe.Event{Type: "invoice.paid"},
				invoice: &stripe.Invoice{},
			},
			exp: tcExpected{
				val: uuid.Nil,
				err: errStripeNoInvoiceLines,
			},
		},

		{
			name: "invoice_no_empty_lintes",
			given: &stripeNotification{
				raw: &stripe.Event{Type: "invoice.paid"},
				invoice: &stripe.Invoice{
					Lines: &stripe.InvoiceLineList{},
				},
			},
			exp: tcExpected{
				val: uuid.Nil,
				err: errStripeNoInvoiceLines,
			},
		},

		{
			name: "invoice_no_order_id",
			given: &stripeNotification{
				raw: &stripe.Event{Type: "invoice.paid"},
				invoice: &stripe.Invoice{
					Lines: &stripe.InvoiceLineList{
						Data: []*stripe.InvoiceLine{
							{
								Metadata: map[string]string{"key": "value"},
							},
						},
					},
				},
			},
			exp: tcExpected{
				val: uuid.Nil,
				err: errStripeOrderIDMissing,
			},
		},

		{
			name: "invoice_valid",
			given: &stripeNotification{
				raw: &stripe.Event{Type: "invoice.paid"},
				invoice: &stripe.Invoice{
					Lines: &stripe.InvoiceLineList{
						Data: []*stripe.InvoiceLine{
							{
								Metadata: map[string]string{
									"orderID": "f100ded0-0000-4000-a000-000000000000",
								},
							},
						},
					},
				},
			},
			exp: tcExpected{
				val: uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
			},
		},

		{
			name: "sub_no_order_id",
			given: &stripeNotification{
				raw: &stripe.Event{Type: "customer.subscription.deleted"},
				sub: &stripe.Subscription{
					ID:       "sub_id",
					Metadata: map[string]string{"key": "value"},
				},
			},
			exp: tcExpected{
				val: uuid.Nil,
				err: errStripeOrderIDMissing,
			},
		},

		{
			name: "sub_valid",
			given: &stripeNotification{
				raw: &stripe.Event{Type: "customer.subscription.deleted"},
				sub: &stripe.Subscription{
					ID: "sub_id",
					Metadata: map[string]string{
						"orderID": "f100ded0-0000-4000-a000-000000000000",
					},
				},
			},
			exp: tcExpected{
				val: uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
			},
		},

		{
			name: "unsupported",
			given: &stripeNotification{
				raw: &stripe.Event{Type: "unsupported"},
			},
			exp: tcExpected{
				val: uuid.Nil,
				err: errStripeUnsupportedEvent,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, err := tc.given.orderID()
			must.Equal(t, tc.exp.err, err)

			should.Equal(t, tc.exp.val, actual)
		})
	}
}

func TestStripeNotification_expiresTime(t *testing.T) {
	type tcExpected struct {
		val time.Time
		err error
	}

	type testCase struct {
		name  string
		given *stripeNotification
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "no_invoice",
			given: &stripeNotification{
				raw: &stripe.Event{Type: "invoice.paid"},
			},
			exp: tcExpected{
				err: errStripeUnsupportedEvent,
			},
		},

		{
			name: "invoice_no_lines",
			given: &stripeNotification{
				raw:     &stripe.Event{Type: "invoice.paid"},
				invoice: &stripe.Invoice{},
			},
			exp: tcExpected{
				err: errStripeNoInvoiceLines,
			},
		},

		{
			name: "invoice_no_empty_lintes",
			given: &stripeNotification{
				raw: &stripe.Event{Type: "invoice.paid"},
				invoice: &stripe.Invoice{
					Lines: &stripe.InvoiceLineList{},
				},
			},
			exp: tcExpected{
				err: errStripeNoInvoiceLines,
			},
		},

		{
			name: "invoice_no_period",
			given: &stripeNotification{
				raw: &stripe.Event{Type: "invoice.paid"},
				invoice: &stripe.Invoice{
					Lines: &stripe.InvoiceLineList{
						Data: []*stripe.InvoiceLine{&stripe.InvoiceLine{}},
					},
				},
			},
			exp: tcExpected{
				err: errStripeInvalidSubPeriod,
			},
		},

		{
			name: "invoice_zero_period",
			given: &stripeNotification{
				raw: &stripe.Event{Type: "invoice.paid"},
				invoice: &stripe.Invoice{
					Lines: &stripe.InvoiceLineList{
						Data: []*stripe.InvoiceLine{
							&stripe.InvoiceLine{
								Period: &stripe.Period{},
							},
						},
					},
				},
			},
			exp: tcExpected{
				err: errStripeInvalidSubPeriod,
			},
		},

		{
			name: "valid",
			given: &stripeNotification{
				raw: &stripe.Event{Type: "invoice.paid"},
				invoice: &stripe.Invoice{
					Lines: &stripe.InvoiceLineList{
						Data: []*stripe.InvoiceLine{
							&stripe.InvoiceLine{
								Period: &stripe.Period{
									Start: 1719792000,
									End:   1722470400,
								},
							},
						},
					},
				},
			},
			exp: tcExpected{
				val: time.Date(2024, time.August, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, err := tc.given.expiresTime()
			must.Equal(t, tc.exp.err, err)

			should.Equal(t, tc.exp.val, actual)
		})
	}
}
