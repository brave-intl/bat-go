package model_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/shopspring/decimal"
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

func TestOrderItemRequestNew_Unmarshal(t *testing.T) {
	type testCase struct {
		name  string
		given []byte
		exp   *model.OrderItemRequestNew
	}

	tests := []testCase{
		{
			name:  "empty_input",
			given: []byte(`{}`),
			exp:   &model.OrderItemRequestNew{},
		},

		{
			name: "price_string",
			given: []byte(`{
				"price": "1"
			}`),
			exp: &model.OrderItemRequestNew{
				Price: mustDecimalFromString("1"),
			},
		},

		{
			name: "price_int",
			given: []byte(`{
				"price": 1
			}`),
			exp: &model.OrderItemRequestNew{
				Price: decimal.NewFromInt(1),
			},
		},

		{
			name: "each_credential_valid_duration",
			given: []byte(`{
				"each_credential_valid_duration": "P1D"
			}`),
			exp: &model.OrderItemRequestNew{
				CredentialValidDurationEach: ptrTo("P1D"),
			},
		},

		{
			name: "issuance_interval",
			given: []byte(`{
				"issuance_interval": "P1M"
			}`),
			exp: &model.OrderItemRequestNew{
				IssuanceInterval: ptrTo("P1M"),
			},
		},

		{
			name: "stripe_metadata",
			given: []byte(`{
				"stripe_metadata": {
					"product_id": "product_id",
					"item_id": "item_id"
				}
			}`),
			exp: &model.OrderItemRequestNew{
				StripeMetadata: &model.ItemStripeMetadata{
					ProductID: "product_id",
					ItemID:    "item_id",
				},
			},
		},

		{
			name: "optional_fields_together",
			given: []byte(`{
				"price": "1",
				"each_credential_valid_duration": "P1D",
				"issuance_interval": "P1M",
				"stripe_metadata": {
					"product_id": "product_id",
					"item_id": "item_id"
				}
			}`),
			exp: &model.OrderItemRequestNew{
				Price:                       mustDecimalFromString("1"),
				CredentialValidDurationEach: ptrTo("P1D"),
				IssuanceInterval:            ptrTo("P1M"),
				StripeMetadata: &model.ItemStripeMetadata{
					ProductID: "product_id",
					ItemID:    "item_id",
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			act := &model.OrderItemRequestNew{}

			err := json.Unmarshal(tc.given, act)
			must.Equal(t, nil, err)

			should.Equal(t, tc.exp, act)
		})
	}
}

func TestItemStripeMetadata_Metadata(t *testing.T) {
	type testCase struct {
		name  string
		given *model.ItemStripeMetadata
		exp   map[string]interface{}
	}

	tests := []testCase{
		{
			name: "nil",
		},

		{
			name:  "empty",
			given: &model.ItemStripeMetadata{},
			exp:   map[string]interface{}{},
		},

		{
			name: "product_id",
			given: &model.ItemStripeMetadata{
				ProductID: "product_id",
			},
			exp: map[string]interface{}{
				"stripe_product_id": "product_id",
			},
		},

		{
			name: "item_id",
			given: &model.ItemStripeMetadata{
				ItemID: "item_id",
			},
			exp: map[string]interface{}{
				"stripe_item_id": "item_id",
			},
		},

		{
			name: "everything",
			given: &model.ItemStripeMetadata{
				ProductID: "product_id",
				ItemID:    "item_id",
			},
			exp: map[string]interface{}{
				"stripe_product_id": "product_id",
				"stripe_item_id":    "item_id",
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			act := tc.given.Metadata()
			should.Equal(t, tc.exp, act)
		})
	}
}

func TestOrderStripeMetadata(t *testing.T) {
	type tcGiven struct {
		data *model.OrderStripeMetadata
		oid  string
	}

	type tcExpected struct {
		surl string
		curl string
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "empty",
		},

		{
			name: "add_id",
			given: tcGiven{
				data: &model.OrderStripeMetadata{
					SuccessURI: "https://example.com/success",
					CancelURI:  "https://example.com/cancel",
				},
				oid: "some_order_id",
			},
			exp: tcExpected{
				surl: "https://example.com/success?order_id=some_order_id",
				curl: "https://example.com/cancel?order_id=some_order_id",
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			act1, err := tc.given.data.SuccessURL(tc.given.oid)
			must.Equal(t, nil, err)

			should.Equal(t, tc.exp.surl, act1)

			act2, err := tc.given.data.CancelURL(tc.given.oid)
			must.Equal(t, nil, err)

			should.Equal(t, tc.exp.curl, act2)
		})
	}
}

func TestOrderItemList_TotalCost(t *testing.T) {
	type testCase struct {
		name  string
		given []model.OrderItem
		exp   decimal.Decimal
	}

	tests := []testCase{
		{
			name: "empty_zero",
		},

		{
			name: "single_zero",
			given: []model.OrderItem{
				{},
			},
		},

		{
			name: "single_nonzero",
			given: []model.OrderItem{
				{Subtotal: decimal.NewFromInt(10)},
			},
			exp: decimal.NewFromInt(10),
		},

		{
			name: "many_zero_nonzero",
			given: []model.OrderItem{
				{},
				{Subtotal: decimal.NewFromInt(10)},
			},
			exp: decimal.NewFromInt(10),
		},

		{
			name: "many_nonzero",
			given: []model.OrderItem{
				{Subtotal: decimal.NewFromInt(11)},
				{Subtotal: decimal.NewFromInt(10)},
			},
			exp: decimal.NewFromInt(21),
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			act := model.OrderItemList(tc.given).TotalCost()
			should.Equal(t, true, tc.exp.Equal(act))
		})
	}
}

func mustDecimalFromString(v string) decimal.Decimal {
	result, err := decimal.NewFromString(v)
	if err != nil {
		panic(err)
	}

	return result
}

func ptrTo[T any](v T) *T {
	return &v
}
