package model_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/lib/pq"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/libs/clients/radom"
	"github.com/brave-intl/bat-go/libs/datastore"
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

func TestOrder_CreateRadomCheckoutSessionWithTime(t *testing.T) {
	type tcGiven struct {
		order     *model.Order
		client    *radom.MockClient
		saddr     string
		expiresAt time.Time
	}
	type tcExpected struct {
		val model.CreateCheckoutSessionResponse
		err error
	}
	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}
	tests := []testCase{
		{
			name: "no_items",
			given: tcGiven{
				order:  &model.Order{},
				client: &radom.MockClient{},
			},
			exp: tcExpected{
				err: model.ErrInvalidOrderNoItems,
			},
		},

		{
			name: "no_radom_success_uri",
			given: tcGiven{
				order: &model.Order{
					Items: []model.OrderItem{{}},
				},
				client: &radom.MockClient{},
			},
			exp: tcExpected{
				err: model.ErrInvalidOrderNoSuccessURL,
			},
		},

		{
			name: "no_radom_cancel_uri",
			given: tcGiven{
				order: &model.Order{
					Items: []model.OrderItem{
						{
							Metadata: datastore.Metadata{
								"radom_success_uri": "something",
							},
						},
					},
				},
				client: &radom.MockClient{},
			},
			exp: tcExpected{
				err: model.ErrInvalidOrderNoCancelURL,
			},
		},

		{
			name: "no_radom_product_id",
			given: tcGiven{
				order: &model.Order{
					Items: []model.OrderItem{
						{
							Metadata: datastore.Metadata{
								"radom_success_uri": "something_success",
								"radom_cancel_uri":  "something_cancel",
							},
						},
					},
				},
				client: &radom.MockClient{},
			},
			exp: tcExpected{
				err: model.ErrInvalidOrderNoProductID,
			},
		},

		{
			name: "client_error",
			given: tcGiven{
				order: &model.Order{
					Items: []model.OrderItem{
						{
							Metadata: datastore.Metadata{
								"radom_success_uri": "something_success",
								"radom_cancel_uri":  "something_cancel",
								"radom_product_id":  "something_id",
							},
						},
					},
				},
				client: &radom.MockClient{
					FnCreateCheckoutSession: func(ctx context.Context, req *radom.CheckoutSessionRequest) (*radom.CheckoutSessionResponse, error) {
						return nil, net.ErrClosed
					},
				},
			},
			exp: tcExpected{
				err: net.ErrClosed,
			},
		},

		{
			name: "client_success",
			given: tcGiven{
				order: &model.Order{
					Items: []model.OrderItem{
						{
							Metadata: datastore.Metadata{
								"radom_success_uri": "something_success",
								"radom_cancel_uri":  "something_cancel",
								"radom_product_id":  "something_id",
							},
						},
					},
				},
				client: &radom.MockClient{
					FnCreateCheckoutSession: func(ctx context.Context, req *radom.CheckoutSessionRequest) (*radom.CheckoutSessionResponse, error) {
						result := &radom.CheckoutSessionResponse{
							SessionID:  "session_id",
							SessionURL: "session_url",
						}

						return result, nil
					},
				},
			},
			exp: tcExpected{
				val: model.CreateCheckoutSessionResponse{
					SessionID: "session_id",
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.TODO()
			act, err := tc.given.order.CreateRadomCheckoutSessionWithTime(
				ctx,
				tc.given.client,
				tc.given.saddr,
				tc.given.expiresAt,
			)
			must.Equal(t, true, errors.Is(err, tc.exp.err))

			if tc.exp.err != nil {
				return
			}
			should.Equal(t, tc.exp.val, act)
		})
	}
}
