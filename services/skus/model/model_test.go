package model_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"

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
				Price: decimal.RequireFromString("1"),
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
				"period": "month",
				"price": "1",
				"each_credential_valid_duration": "P1D",
				"issuance_interval": "P1M",
				"stripe_metadata": {
					"product_id": "product_id",
					"item_id": "item_id"
				}
			}`),
			exp: &model.OrderItemRequestNew{
				Period:                      "month",
				Price:                       decimal.RequireFromString("1"),
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

func TestOrderRadomMetadata(t *testing.T) {
	type tcGiven struct {
		data *model.OrderRadomMetadata
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
				data: &model.OrderRadomMetadata{
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

func TestOrder_HasItem(t *testing.T) {
	type tcGiven struct {
		order  *model.Order
		itemID uuid.UUID
	}

	type tcExpected struct {
		item *model.OrderItem
		ok   bool
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "no_items_nothing_found",
			given: tcGiven{
				order:  &model.Order{},
				itemID: uuid.Must(uuid.FromString("b5e3f3e4-0bd4-4fd5-a693-a50f4dbfd6ac")),
			},
		},

		{
			name: "one_item_not_found",
			given: tcGiven{
				order: &model.Order{
					Items: []model.OrderItem{
						{
							ID: uuid.Must(uuid.FromString("dbc6416a-7713-4aa5-8968-56aef7ec0e81")),
						},
					},
				},
				itemID: uuid.Must(uuid.FromString("b5e3f3e4-0bd4-4fd5-a693-a50f4dbfd6ac")),
			},
		},

		{
			name: "two_items_not_found",
			given: tcGiven{
				order: &model.Order{
					Items: []model.OrderItem{
						{
							ID: uuid.Must(uuid.FromString("dbc6416a-7713-4aa5-8968-56aef7ec0e81")),
						},

						{
							ID: uuid.Must(uuid.FromString("4efbedfe-a598-43a4-a345-17653d6289e8")),
						},
					},
				},
				itemID: uuid.Must(uuid.FromString("b5e3f3e4-0bd4-4fd5-a693-a50f4dbfd6ac")),
			},
		},

		{
			name: "one_item_found",
			given: tcGiven{
				order: &model.Order{
					Items: []model.OrderItem{
						{
							ID: uuid.Must(uuid.FromString("b5e3f3e4-0bd4-4fd5-a693-a50f4dbfd6ac")),
						},
					},
				},
				itemID: uuid.Must(uuid.FromString("b5e3f3e4-0bd4-4fd5-a693-a50f4dbfd6ac")),
			},
			exp: tcExpected{
				item: &model.OrderItem{
					ID: uuid.Must(uuid.FromString("b5e3f3e4-0bd4-4fd5-a693-a50f4dbfd6ac")),
				},
				ok: true,
			},
		},

		{
			name: "many_items_found",
			given: tcGiven{
				order: &model.Order{
					Items: []model.OrderItem{
						{
							ID: uuid.Must(uuid.FromString("dbc6416a-7713-4aa5-8968-56aef7ec0e81")),
						},

						{
							ID: uuid.Must(uuid.FromString("4efbedfe-a598-43a4-a345-17653d6289e8")),
						},

						{
							ID: uuid.Must(uuid.FromString("b5e3f3e4-0bd4-4fd5-a693-a50f4dbfd6ac")),
						},
					},
				},
				itemID: uuid.Must(uuid.FromString("b5e3f3e4-0bd4-4fd5-a693-a50f4dbfd6ac")),
			},
			exp: tcExpected{
				item: &model.OrderItem{
					ID: uuid.Must(uuid.FromString("b5e3f3e4-0bd4-4fd5-a693-a50f4dbfd6ac")),
				},
				ok: true,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			item, ok := tc.given.order.HasItem(tc.given.itemID)
			must.Equal(t, tc.exp.ok, ok)

			if !tc.exp.ok {
				return
			}

			should.Equal(t, tc.exp.item, item)
		})
	}
}

func TestOrder_StripeSubID(t *testing.T) {
	type tcExpected struct {
		val string
		ok  bool
	}

	type testCase struct {
		name  string
		given model.Order
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "no_metadata",
		},

		{
			name: "no_field",
			given: model.Order{
				Metadata: datastore.Metadata{"key": "value"},
			},
		},

		{
			name: "not_string",
			given: model.Order{
				Metadata: datastore.Metadata{
					"stripeSubscriptionId": 42,
				},
			},
		},

		{
			name: "empty_string",
			given: model.Order{
				Metadata: datastore.Metadata{
					"stripeSubscriptionId": "",
				},
			},
			exp: tcExpected{ok: true},
		},

		{
			name: "sub_id",
			given: model.Order{
				Metadata: datastore.Metadata{
					"stripeSubscriptionId": "sub_id",
				},
			},
			exp: tcExpected{val: "sub_id", ok: true},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, ok := tc.given.StripeSubID()
			should.Equal(t, tc.exp.ok, ok)
			should.Equal(t, tc.exp.val, actual)
		})
	}
}

func TestOrder_StripeSessID(t *testing.T) {
	type tcExpected struct {
		val string
		ok  bool
	}

	type testCase struct {
		name  string
		given model.Order
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "no_metadata",
		},

		{
			name: "no_field",
			given: model.Order{
				Metadata: datastore.Metadata{"key": "value"},
			},
		},

		{
			name: "not_string",
			given: model.Order{
				Metadata: datastore.Metadata{
					"stripeCheckoutSessionId": 42,
				},
			},
		},

		{
			name: "empty_string",
			given: model.Order{
				Metadata: datastore.Metadata{
					"stripeCheckoutSessionId": "",
				},
			},
			exp: tcExpected{ok: true},
		},

		{
			name: "sess_id",
			given: model.Order{
				Metadata: datastore.Metadata{
					"stripeCheckoutSessionId": "sess_id",
				},
			},
			exp: tcExpected{val: "sess_id", ok: true},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, ok := tc.given.StripeSessID()
			should.Equal(t, tc.exp.ok, ok)
			should.Equal(t, tc.exp.val, actual)
		})
	}
}

func TestOrder_IsIOS(t *testing.T) {
	type testCase struct {
		name  string
		given model.Order
		exp   bool
	}

	tests := []testCase{
		{
			name: "no_metadata",
		},

		{
			name: "no_pp",
			given: model.Order{
				Metadata: datastore.Metadata{"key": "value"},
			},
		},

		{
			name: "pp_stripe_no_vn",
			given: model.Order{
				Metadata: datastore.Metadata{
					"paymentProcessor": "stripe",
				},
			},
		},

		{
			name: "pp_stripe_vn_android",
			given: model.Order{
				Metadata: datastore.Metadata{
					"paymentProcessor": "stripe",
					"vendor":           "android",
				},
			},
		},

		{
			name: "pp_ios_vn_android",
			given: model.Order{
				Metadata: datastore.Metadata{
					"paymentProcessor": "ios",
					"vendor":           "android",
				},
			},
		},

		{
			name: "pp_stripe_vn_ios",
			given: model.Order{
				Metadata: datastore.Metadata{
					"paymentProcessor": "stripe",
					"vendor":           "ios",
				},
			},
		},

		{
			name: "ios",
			given: model.Order{
				Metadata: datastore.Metadata{
					"paymentProcessor": "ios",
					"vendor":           "ios",
				},
			},
			exp: true,
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.IsIOS()
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestOrder_IsAndroid(t *testing.T) {
	type testCase struct {
		name  string
		given model.Order
		exp   bool
	}

	tests := []testCase{
		{
			name: "no_metadata",
		},

		{
			name: "no_pp",
			given: model.Order{
				Metadata: datastore.Metadata{"key": "value"},
			},
		},

		{
			name: "pp_stripe_no_vn",
			given: model.Order{
				Metadata: datastore.Metadata{
					"paymentProcessor": "stripe",
				},
			},
		},

		{
			name: "pp_stripe_vn_ios",
			given: model.Order{
				Metadata: datastore.Metadata{
					"paymentProcessor": "stripe",
					"vendor":           "ios",
				},
			},
		},

		{
			name: "pp_android_vn_ios",
			given: model.Order{
				Metadata: datastore.Metadata{
					"paymentProcessor": "android",
					"vendor":           "ios",
				},
			},
		},

		{
			name: "pp_stripe_vn_android",
			given: model.Order{
				Metadata: datastore.Metadata{
					"paymentProcessor": "stripe",
					"vendor":           "android",
				},
			},
		},

		{
			name: "android",
			given: model.Order{
				Metadata: datastore.Metadata{
					"paymentProcessor": "android",
					"vendor":           "android",
				},
			},
			exp: true,
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.IsAndroid()
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestOrder_IsStripe(t *testing.T) {
	type testCase struct {
		name  string
		given model.Order
		exp   bool
	}

	tests := []testCase{
		{
			name: "no_metadata",
		},

		{
			name: "no_pp",
			given: model.Order{
				Metadata: datastore.Metadata{"key": "value"},
			},
		},

		{
			name: "false_pp_ios",
			given: model.Order{
				Metadata: datastore.Metadata{
					"paymentProcessor": "ios",
				},
			},
		},

		{
			name: "false_pp_android",
			given: model.Order{
				Metadata: datastore.Metadata{
					"paymentProcessor": "android",
				},
			},
		},

		{
			name: "pp_stripe",
			given: model.Order{
				Metadata: datastore.Metadata{
					"paymentProcessor": "stripe",
				},
			},
			exp: true,
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.IsStripe()
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestOrder_PaymentProc(t *testing.T) {
	type tcExpected struct {
		val string
		ok  bool
	}

	type testCase struct {
		name  string
		given model.Order
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "no_metadata",
		},

		{
			name: "no_field",
			given: model.Order{
				Metadata: datastore.Metadata{"key": "value"},
			},
		},

		{
			name: "not_string",
			given: model.Order{
				Metadata: datastore.Metadata{
					"paymentProcessor": 42,
				},
			},
		},

		{
			name: "empty_string",
			given: model.Order{
				Metadata: datastore.Metadata{
					"paymentProcessor": "",
				},
			},
			exp: tcExpected{ok: true},
		},

		{
			name: "sub_id",
			given: model.Order{
				Metadata: datastore.Metadata{
					"paymentProcessor": "stripe",
				},
			},
			exp: tcExpected{val: "stripe", ok: true},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, ok := tc.given.PaymentProc()
			should.Equal(t, tc.exp.ok, ok)
			should.Equal(t, tc.exp.val, actual)
		})
	}
}

func TestOrder_Vendor(t *testing.T) {
	type tcExpected struct {
		val model.Vendor
		ok  bool
	}

	type testCase struct {
		name  string
		given model.Order
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "no_metadata",
			exp: tcExpected{
				val: model.VendorUnknown,
			},
		},

		{
			name: "no_field",
			given: model.Order{
				Metadata: datastore.Metadata{"key": "value"},
			},
			exp: tcExpected{
				val: model.VendorUnknown,
			},
		},

		{
			name: "not_string",
			given: model.Order{
				Metadata: datastore.Metadata{
					"vendor": 42,
				},
			},
			exp: tcExpected{
				val: model.VendorUnknown,
			},
		},

		{
			name: "empty_string",
			given: model.Order{
				Metadata: datastore.Metadata{
					"vendor": "",
				},
			},
			exp: tcExpected{
				ok: true,
			},
		},

		{
			name: "something_else",
			given: model.Order{
				Metadata: datastore.Metadata{
					"vendor": "something_else",
				},
			},
			exp: tcExpected{
				val: model.Vendor("something_else"),
				ok:  true,
			},
		},

		{
			name: "apple",
			given: model.Order{
				Metadata: datastore.Metadata{
					"vendor": "ios",
				},
			},
			exp: tcExpected{
				val: model.VendorApple,
				ok:  true,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, ok := tc.given.Vendor()
			should.Equal(t, tc.exp.ok, ok)
			should.Equal(t, tc.exp.val, actual)
		})
	}
}

func TestOrder_ShouldSetTrialDays(t *testing.T) {
	type testCase struct {
		name  string
		given model.Order
		exp   bool
	}

	tests := []testCase{
		{
			name:  "not_paid",
			given: model.Order{Status: model.OrderStatusPending},
		},

		{
			name: "not_paid_not_stripe",
			given: model.Order{
				Status:                model.OrderStatusPending,
				AllowedPaymentMethods: pq.StringArray{"something"},
			},
		},

		{
			name:  "paid",
			given: model.Order{Status: model.OrderStatusPaid},
		},

		{
			name: "paid_not_stripe",
			given: model.Order{
				Status:                model.OrderStatusPaid,
				AllowedPaymentMethods: pq.StringArray{"something"},
			},
		},

		{
			name: "paid_stripe",
			given: model.Order{
				Status:                model.OrderStatusPaid,
				AllowedPaymentMethods: pq.StringArray{"stripe"},
			},
		},

		{
			name: "not_paid_stripe",
			given: model.Order{
				Status:                model.OrderStatusPending,
				AllowedPaymentMethods: pq.StringArray{"stripe"},
			},
			exp: true,
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.ShouldSetTrialDays()
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestOrderItem_StripeItemID(t *testing.T) {
	type tcExpected struct {
		val string
		ok  bool
	}

	type testCase struct {
		name  string
		given model.OrderItem
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "no_metadata",
		},

		{
			name: "no_field",
			given: model.OrderItem{
				Metadata: datastore.Metadata{"key": "value"},
			},
		},

		{
			name: "not_string",
			given: model.OrderItem{
				Metadata: datastore.Metadata{
					"stripe_item_id": 42,
				},
			},
		},

		{
			name: "empty_string",
			given: model.OrderItem{
				Metadata: datastore.Metadata{
					"stripe_item_id": "",
				},
			},
			exp: tcExpected{ok: true},
		},

		{
			name: "stripe_item_id",
			given: model.OrderItem{
				Metadata: datastore.Metadata{
					"stripe_item_id": "stripe_item_id",
				},
			},
			exp: tcExpected{val: "stripe_item_id", ok: true},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, ok := tc.given.StripeItemID()
			should.Equal(t, tc.exp.ok, ok)
			should.Equal(t, tc.exp.val, actual)
		})
	}
}

func TestOrderItemRequestNew_Metadata(t *testing.T) {
	type tcGiven struct {
		oreq model.OrderItemRequestNew
	}

	type tcExpected struct {
		metadata map[string]interface{}
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "nil",
		},

		{
			name: "stripe_metadata",
			given: tcGiven{
				oreq: model.OrderItemRequestNew{
					StripeMetadata: &model.ItemStripeMetadata{
						ProductID: "product_1",
						ItemID:    "item_1",
					},
				},
			},
			exp: tcExpected{
				metadata: map[string]interface{}{
					"stripe_product_id": "product_1",
					"stripe_item_id":    "item_1",
				},
			},
		},

		{
			name: "radom_metadata",
			given: tcGiven{
				oreq: model.OrderItemRequestNew{
					RadomMetadata: &model.ItemRadomMetadata{
						ProductID: "product_1",
					},
				},
			},
			exp: tcExpected{
				metadata: map[string]interface{}{
					"radom_product_id": "product_1",
				},
			},
		},

		{
			name: "no_metadata",
			given: tcGiven{
				oreq: model.OrderItemRequestNew{},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.oreq.Metadata()
			should.Equal(t, tc.exp.metadata, actual)
		})
	}
}

func mustDecimalFromString(v string) decimal.Decimal {
	result, err := decimal.NewFromString(v)
	if err != nil {
		panic(err)
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, ok := tc.given.StripeItemID()
			should.Equal(t, tc.exp.ok, ok)
			should.Equal(t, tc.exp.val, actual)
		})
	}
}

func TestOrderItemRequestNew_TokenBufferOrDefault(t *testing.T) {
	type testCase struct {
		name  string
		given *model.OrderItemRequestNew
		exp   int
	}

	tests := []testCase{
		{
			name: "zero_nil",
		},

		{
			name: "one_non_tlv2",
			given: &model.OrderItemRequestNew{
				CredentialType: "time-limited",
			},
			exp: 1,
		},

		{
			name: "default",
			given: &model.OrderItemRequestNew{
				CredentialType: "time-limited-v2",
			},
			exp: 30,
		},

		{
			name: "set_value",
			given: &model.OrderItemRequestNew{
				CredentialType:    "time-limited-v2",
				IssuerTokenBuffer: ptrTo(3),
			},
			exp: 3,
		},

		{
			name: "set_value_zero",
			given: &model.OrderItemRequestNew{
				CredentialType:    "time-limited-v2",
				IssuerTokenBuffer: ptrTo(0),
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.TokenBufferOrDefault()
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestOrderItemRequestNew_TokenOverlapOrDefault(t *testing.T) {
	type testCase struct {
		name  string
		given *model.OrderItemRequestNew
		exp   int
	}

	tests := []testCase{
		{
			name: "zero_nil",
		},

		{
			name: "one_non_tlv2",
			given: &model.OrderItemRequestNew{
				CredentialType: "time-limited",
			},
		},

		{
			name: "default",
			given: &model.OrderItemRequestNew{
				CredentialType: "time-limited-v2",
			},
			exp: 5,
		},

		{
			name: "set_value",
			given: &model.OrderItemRequestNew{
				CredentialType:     "time-limited-v2",
				IssuerTokenOverlap: ptrTo(2),
			},
			exp: 2,
		},

		{
			name: "set_value_zero",
			given: &model.OrderItemRequestNew{
				CredentialType:     "time-limited-v2",
				IssuerTokenOverlap: ptrTo(0),
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.TokenOverlapOrDefault()
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestOrderItemRequestNew_IsTLV2(t *testing.T) {
	type testCase struct {
		name  string
		given *model.OrderItemRequestNew
		exp   bool
	}

	tests := []testCase{
		{
			name: "false_nil",
		},

		{
			name: "false_non_tlv2",
			given: &model.OrderItemRequestNew{
				CredentialType: "time-limited",
			},
		},

		{
			name: "true_tlv2",
			given: &model.OrderItemRequestNew{
				CredentialType: "time-limited-v2",
			},
			exp: true,
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.IsTLV2()
			should.Equal(t, tc.exp, actual)
		})
	}
}

func ptrTo[T any](v T) *T {
	return &v
}
