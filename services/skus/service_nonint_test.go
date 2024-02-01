package skus

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/libs/datastore"

	"github.com/brave-intl/bat-go/services/skus/model"
	"github.com/brave-intl/bat-go/services/skus/storage/repository"
)

func TestCheckNumBlindedCreds(t *testing.T) {
	type tcGiven struct {
		ord    *model.Order
		item   *model.OrderItem
		ncreds int
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   error
	}

	tests := []testCase{
		{
			name: "irrelevant_credential_type",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("df140c71-740b-46c9-bedd-27be0b1e6354")),
				},
				item: &model.OrderItem{
					ID:             uuid.Must(uuid.FromString("82514074-c4f5-4515-8d8d-29ab943615b3")),
					OrderID:        uuid.Must(uuid.FromString("df140c71-740b-46c9-bedd-27be0b1e6354")),
					CredentialType: timeLimited,
				},
			},
		},

		{
			name: "single_use_valid_1",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("df140c71-740b-46c9-bedd-27be0b1e6354")),
				},
				item: &model.OrderItem{
					ID:             uuid.Must(uuid.FromString("82514074-c4f5-4515-8d8d-29ab943615b3")),
					OrderID:        uuid.Must(uuid.FromString("df140c71-740b-46c9-bedd-27be0b1e6354")),
					CredentialType: singleUse,
					Quantity:       1,
				},
				ncreds: 1,
			},
		},

		{
			name: "single_use_valid_2",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("df140c71-740b-46c9-bedd-27be0b1e6354")),
				},
				item: &model.OrderItem{
					ID:             uuid.Must(uuid.FromString("82514074-c4f5-4515-8d8d-29ab943615b3")),
					OrderID:        uuid.Must(uuid.FromString("df140c71-740b-46c9-bedd-27be0b1e6354")),
					CredentialType: singleUse,
					Quantity:       2,
				},
				ncreds: 1,
			},
		},

		{
			name: "single_use_invalid",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("df140c71-740b-46c9-bedd-27be0b1e6354")),
				},
				item: &model.OrderItem{
					ID:             uuid.Must(uuid.FromString("82514074-c4f5-4515-8d8d-29ab943615b3")),
					OrderID:        uuid.Must(uuid.FromString("df140c71-740b-46c9-bedd-27be0b1e6354")),
					CredentialType: singleUse,
					Quantity:       2,
				},
				ncreds: 3,
			},
			exp: errInvalidNCredsSingleUse,
		},

		{
			name: "tlv2_invalid_numPerInterval_missing",
			given: tcGiven{
				ord: &model.Order{
					ID:       uuid.Must(uuid.FromString("df140c71-740b-46c9-bedd-27be0b1e6354")),
					Metadata: datastore.Metadata{},
				},
				item: &model.OrderItem{
					ID:             uuid.Must(uuid.FromString("82514074-c4f5-4515-8d8d-29ab943615b3")),
					OrderID:        uuid.Must(uuid.FromString("df140c71-740b-46c9-bedd-27be0b1e6354")),
					CredentialType: timeLimitedV2,
					Quantity:       1,
				},
				ncreds: 6,
			},
			exp: model.ErrNumPerIntervalNotSet,
		},

		{
			name: "tlv2_invalid_numPerInterval_invalid",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("df140c71-740b-46c9-bedd-27be0b1e6354")),
					Metadata: datastore.Metadata{
						"numPerInterval": "NaN",
					},
				},
				item: &model.OrderItem{
					ID:             uuid.Must(uuid.FromString("82514074-c4f5-4515-8d8d-29ab943615b3")),
					OrderID:        uuid.Must(uuid.FromString("df140c71-740b-46c9-bedd-27be0b1e6354")),
					CredentialType: timeLimitedV2,
					Quantity:       1,
				},
				ncreds: 6,
			},
			exp: model.ErrInvalidNumPerInterval,
		},

		{
			name: "tlv2_invalid_numIntervals_missing",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("df140c71-740b-46c9-bedd-27be0b1e6354")),
					Metadata: datastore.Metadata{
						// We get a float64 upon fetching from the database.
						"numPerInterval": float64(2),
					},
				},
				item: &model.OrderItem{
					ID:             uuid.Must(uuid.FromString("82514074-c4f5-4515-8d8d-29ab943615b3")),
					OrderID:        uuid.Must(uuid.FromString("df140c71-740b-46c9-bedd-27be0b1e6354")),
					CredentialType: timeLimitedV2,
					Quantity:       1,
				},
				ncreds: 6,
			},
			exp: model.ErrNumIntervalsNotSet,
		},

		{
			name: "tlv2_invalid_numIntervals_invalid",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("df140c71-740b-46c9-bedd-27be0b1e6354")),
					Metadata: datastore.Metadata{
						// We get a float64 upon fetching from the database.
						"numPerInterval": float64(2),
						"numIntervals":   "NaN",
					},
				},
				item: &model.OrderItem{
					ID:             uuid.Must(uuid.FromString("82514074-c4f5-4515-8d8d-29ab943615b3")),
					OrderID:        uuid.Must(uuid.FromString("df140c71-740b-46c9-bedd-27be0b1e6354")),
					CredentialType: timeLimitedV2,
					Quantity:       1,
				},
				ncreds: 6,
			},
			exp: model.ErrInvalidNumIntervals,
		},

		{
			name: "tlv2_valid_1",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("df140c71-740b-46c9-bedd-27be0b1e6354")),
					Metadata: datastore.Metadata{
						// We get a float64 upon fetching from the database.
						"numPerInterval": float64(2),
						"numIntervals":   float64(3),
					},
				},
				item: &model.OrderItem{
					ID:             uuid.Must(uuid.FromString("82514074-c4f5-4515-8d8d-29ab943615b3")),
					OrderID:        uuid.Must(uuid.FromString("df140c71-740b-46c9-bedd-27be0b1e6354")),
					CredentialType: timeLimitedV2,
					Quantity:       1,
				},
				ncreds: 6,
			},
		},

		{
			name: "tlv2_valid_2",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("df140c71-740b-46c9-bedd-27be0b1e6354")),
					Metadata: datastore.Metadata{
						// We get a float64 upon fetching from the database.
						"numPerInterval": float64(2),
						"numIntervals":   float64(4),
					},
				},
				item: &model.OrderItem{
					ID:             uuid.Must(uuid.FromString("82514074-c4f5-4515-8d8d-29ab943615b3")),
					OrderID:        uuid.Must(uuid.FromString("df140c71-740b-46c9-bedd-27be0b1e6354")),
					CredentialType: timeLimitedV2,
					Quantity:       1,
				},
				ncreds: 6,
			},
		},

		{
			name: "tlv2_invalid",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("df140c71-740b-46c9-bedd-27be0b1e6354")),
					Metadata: datastore.Metadata{
						// We get a float64 upon fetching from the database.
						"numPerInterval": float64(2),
						"numIntervals":   float64(3),
					},
				},
				item: &model.OrderItem{
					ID:             uuid.Must(uuid.FromString("82514074-c4f5-4515-8d8d-29ab943615b3")),
					OrderID:        uuid.Must(uuid.FromString("df140c71-740b-46c9-bedd-27be0b1e6354")),
					CredentialType: timeLimitedV2,
					Quantity:       1,
				},
				ncreds: 7,
			},
			exp: errInvalidNCredsTlv2,
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := checkNumBlindedCreds(tc.given.ord, tc.given.item, tc.given.ncreds)

			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestDoItemsHaveSUOrTlv2(t *testing.T) {
	type testCase struct {
		name    string
		given   []model.OrderItem
		expSU   bool
		expTlv2 bool
	}

	tests := []testCase{
		{
			name: "nil",
		},

		{
			name:  "empty",
			given: []model.OrderItem{},
		},

		{
			name: "one_single_use",
			given: []model.OrderItem{
				{
					CredentialType: singleUse,
				},
			},
			expSU: true,
		},

		{
			name: "two_single_use",
			given: []model.OrderItem{
				{
					CredentialType: singleUse,
				},

				{
					CredentialType: singleUse,
				},
			},
			expSU: true,
		},

		{
			name: "one_time_limited",
			given: []model.OrderItem{
				{
					CredentialType: timeLimited,
				},
			},
		},

		{
			name: "two_time_limited",
			given: []model.OrderItem{
				{
					CredentialType: timeLimited,
				},

				{
					CredentialType: timeLimited,
				},
			},
		},

		{
			name: "one_time_limited_v2",
			given: []model.OrderItem{
				{
					CredentialType: timeLimitedV2,
				},
			},
			expTlv2: true,
		},

		{
			name: "two_time_limited_v2",
			given: []model.OrderItem{
				{
					CredentialType: timeLimitedV2,
				},

				{
					CredentialType: timeLimitedV2,
				},
			},
			expTlv2: true,
		},

		{
			name: "one_single_use_one_time_limited_v2",
			given: []model.OrderItem{
				{
					CredentialType: singleUse,
				},

				{
					CredentialType: timeLimitedV2,
				},
			},
			expSU:   true,
			expTlv2: true,
		},

		{
			name: "all_one",
			given: []model.OrderItem{
				{
					CredentialType: singleUse,
				},

				{
					CredentialType: timeLimited,
				},

				{
					CredentialType: timeLimitedV2,
				},
			},
			expSU:   true,
			expTlv2: true,
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			doSingleUse, doTlv2 := doItemsHaveSUOrTlv2(tc.given)

			should.Equal(t, tc.expSU, doSingleUse)
			should.Equal(t, tc.expTlv2, doTlv2)
		})
	}
}

func TestNewMobileOrderMdata(t *testing.T) {
	type tcGiven struct {
		extID string
		req   model.ReceiptRequest
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   datastore.Metadata
	}

	tests := []testCase{
		{
			name: "android",
			given: tcGiven{
				extID: "extID",
				req: model.ReceiptRequest{
					Type:           model.VendorGoogle,
					Blob:           "blob",
					Package:        "package",
					SubscriptionID: "subID",
				},
			},
			exp: datastore.Metadata{
				"externalID":       "extID",
				"paymentProcessor": "android",
				"vendor":           "android",
			},
		},

		{
			name: "ios",
			given: tcGiven{
				extID: "extID",
				req: model.ReceiptRequest{
					Type:           model.VendorApple,
					Blob:           "blob",
					Package:        "package",
					SubscriptionID: "subID",
				},
			},
			exp: datastore.Metadata{
				"externalID":       "extID",
				"paymentProcessor": "ios",
				"vendor":           "ios",
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := newMobileOrderMdata(tc.given.req, tc.given.extID)
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestNewOrderNewForReq(t *testing.T) {
	type tcGiven struct {
		merchID string
		status  string
		req     *model.CreateOrderRequestNew
		items   []model.OrderItem
	}

	type tcExpected struct {
		ord *model.OrderNew
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
				merchID: model.MerchID,
				status:  model.OrderStatusPending,
				req: &model.CreateOrderRequestNew{
					Currency:       "USD",
					PaymentMethods: []string{"stripe"},
				},
			},
			exp: tcExpected{
				err: model.ErrInvalidOrderRequest,
			},
		},

		{
			name: "total_zero_paid",
			given: tcGiven{
				merchID: model.MerchID,
				status:  model.OrderStatusPending,
				req: &model.CreateOrderRequestNew{
					Currency:       "USD",
					PaymentMethods: []string{"stripe"},
				},
				items: []model.OrderItem{
					{
						SKU:            "sku",
						Currency:       "USD",
						CredentialType: "credential_type",
						Price:          decimal.NewFromInt(0),
						Quantity:       1,
						Subtotal:       decimal.NewFromInt(0),
					},
				},
			},
			exp: tcExpected{
				ord: &model.OrderNew{
					MerchantID:            "brave.com",
					Currency:              "USD",
					Status:                model.OrderStatusPaid,
					TotalPrice:            decimal.NewFromInt(0),
					AllowedPaymentMethods: pq.StringArray([]string{"stripe"}),
					ValidFor:              ptrTo(time.Duration(0)),
				},
			},
		},

		{
			name: "one_item_use_location",
			given: tcGiven{
				merchID: model.MerchID,
				status:  model.OrderStatusPending,
				req: &model.CreateOrderRequestNew{
					Currency:       "USD",
					PaymentMethods: []string{"stripe"},
				},
				items: []model.OrderItem{
					{
						SKU:            "sku",
						Currency:       "USD",
						CredentialType: "credential_type",
						Price:          decimal.NewFromInt(1),
						Quantity:       1,
						Subtotal:       decimal.NewFromInt(1),
						Location: datastore.NullString{
							NullString: sql.NullString{
								String: "location",
								Valid:  true,
							},
						},
					},
				},
			},
			exp: tcExpected{
				ord: &model.OrderNew{
					MerchantID:            "brave.com",
					Currency:              "USD",
					Status:                model.OrderStatusPending,
					TotalPrice:            decimal.NewFromInt(1),
					AllowedPaymentMethods: pq.StringArray([]string{"stripe"}),
					ValidFor:              ptrTo(time.Duration(0)),
					Location: sql.NullString{
						String: "location",
						Valid:  true,
					},
				},
			},
		},

		{
			name: "two_items_no_location",
			given: tcGiven{
				merchID: model.MerchID,
				status:  model.OrderStatusPending,
				req: &model.CreateOrderRequestNew{
					Currency:       "USD",
					PaymentMethods: []string{"stripe"},
				},
				items: []model.OrderItem{
					{
						SKU:            "sku01",
						Currency:       "USD",
						CredentialType: "credential_type",
						Price:          decimal.NewFromInt(1),
						Quantity:       1,
						Subtotal:       decimal.NewFromInt(1),
						Location: datastore.NullString{
							NullString: sql.NullString{
								String: "location01",
								Valid:  true,
							},
						},
					},

					{
						SKU:            "sku02",
						Currency:       "USD",
						CredentialType: "credential_type",
						Price:          decimal.NewFromInt(1),
						Quantity:       1,
						Subtotal:       decimal.NewFromInt(1),
						Location: datastore.NullString{
							NullString: sql.NullString{
								String: "location02",
								Valid:  true,
							},
						},
					},
				},
			},
			exp: tcExpected{
				ord: &model.OrderNew{
					MerchantID:            "brave.com",
					Currency:              "USD",
					Status:                model.OrderStatusPending,
					TotalPrice:            decimal.NewFromInt(2),
					AllowedPaymentMethods: pq.StringArray([]string{"stripe"}),
					ValidFor:              ptrTo(time.Duration(0)),
				},
			},
		},

		{
			name: "valid_for_from_first_item",
			given: tcGiven{
				merchID: model.MerchID,
				status:  model.OrderStatusPending,
				req: &model.CreateOrderRequestNew{
					Currency:       "USD",
					PaymentMethods: []string{"stripe"},
				},
				items: []model.OrderItem{
					{
						SKU:            "sku01",
						Currency:       "USD",
						CredentialType: "credential_type",
						Price:          decimal.NewFromInt(1),
						Quantity:       1,
						Subtotal:       decimal.NewFromInt(1),
						Location: datastore.NullString{
							NullString: sql.NullString{
								String: "location01",
								Valid:  true,
							},
						},
						ValidFor: ptrTo(time.Duration(24 * 30 * time.Hour)),
					},

					{
						SKU:            "sku02",
						Currency:       "USD",
						CredentialType: "credential_type",
						Price:          decimal.NewFromInt(1),
						Quantity:       1,
						Subtotal:       decimal.NewFromInt(1),
						Location: datastore.NullString{
							NullString: sql.NullString{
								String: "location02",
								Valid:  true,
							},
						},
					},
				},
			},
			exp: tcExpected{
				ord: &model.OrderNew{
					MerchantID:            "brave.com",
					Currency:              "USD",
					Status:                model.OrderStatusPending,
					TotalPrice:            decimal.NewFromInt(2),
					AllowedPaymentMethods: pq.StringArray([]string{"stripe"}),
					ValidFor:              ptrTo(time.Duration(24 * 30 * time.Hour)),
				},
			},
		},

		{
			name: "explicit_paid",
			given: tcGiven{
				merchID: model.MerchID,
				status:  model.OrderStatusPaid,
				req: &model.CreateOrderRequestNew{
					Currency:       "USD",
					PaymentMethods: []string{"stripe"},
				},
				items: []model.OrderItem{
					{
						SKU:            "sku",
						Currency:       "USD",
						CredentialType: "credential_type",
						Price:          decimal.NewFromInt(1),
						Quantity:       1,
						Subtotal:       decimal.NewFromInt(1),
						Location: datastore.NullString{
							NullString: sql.NullString{
								String: "location",
								Valid:  true,
							},
						},
						ValidFor: ptrTo(time.Duration(24 * 30 * time.Hour)),
					},
				},
			},
			exp: tcExpected{
				ord: &model.OrderNew{
					MerchantID:            "brave.com",
					Currency:              "USD",
					Status:                model.OrderStatusPaid,
					TotalPrice:            decimal.NewFromInt(1),
					AllowedPaymentMethods: pq.StringArray([]string{"stripe"}),
					ValidFor:              ptrTo(time.Duration(24 * 30 * time.Hour)),
					Location: sql.NullString{
						String: "location",
						Valid:  true,
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, err := newOrderNewForReq(tc.given.req, tc.given.items, tc.given.merchID, tc.given.status)
			must.Equal(t, tc.exp.err, err)

			should.Equal(t, tc.exp.ord, actual)
		})
	}
}

func TestCreateOrderWithReceipt(t *testing.T) {
	type tcGiven struct {
		svc   *mockPaidOrderCreator
		set   map[string]model.OrderItemRequestNew
		ppcfg *premiumPaymentProcConfig
		req   model.ReceiptRequest
		extID string
	}

	type tcExpected struct {
		ord *model.Order
		err error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "error_in_newOrderItemReqForSubID",
			given: tcGiven{
				svc:   &mockPaidOrderCreator{},
				set:   newOrderItemReqNewLeoSet("development"),
				ppcfg: newPaymentProcessorConfig("development"),
				req: model.ReceiptRequest{
					Type:           model.VendorGoogle,
					Blob:           "blob",
					Package:        "package",
					SubscriptionID: "invalid",
				},
			},
			exp: tcExpected{err: model.ErrInvalidMobileProduct},
		},

		{
			name: "error_in_createOrder",
			given: tcGiven{
				svc: &mockPaidOrderCreator{
					fnCreateOrder: func(ctx context.Context, req *model.CreateOrderRequestNew, ordNew *model.OrderNew, items []model.OrderItem) (*model.Order, error) {
						return nil, model.Error("something went wrong")
					},
				},
				set:   newOrderItemReqNewLeoSet("development"),
				ppcfg: newPaymentProcessorConfig("development"),
				req: model.ReceiptRequest{
					Type:           model.VendorGoogle,
					Blob:           "blob",
					Package:        "package",
					SubscriptionID: "brave.leo.monthly",
				},
			},
			exp: tcExpected{err: model.Error("something went wrong")},
		},

		{
			name: "error_in_UpdateOrderStatusPaidWithMetadata",
			given: tcGiven{
				svc: &mockPaidOrderCreator{
					fnCreateOrder: func(ctx context.Context, req *model.CreateOrderRequestNew, ordNew *model.OrderNew, items []model.OrderItem) (*model.Order, error) {
						return &model.Order{}, nil
					},

					fnUpdateOrderStatusPaidWithMetadata: func(ctx context.Context, oid *uuid.UUID, mdata datastore.Metadata) error {
						return model.Error("something went wrong")
					},
				},
				set:   newOrderItemReqNewLeoSet("development"),
				ppcfg: newPaymentProcessorConfig("development"),
				req: model.ReceiptRequest{
					Type:           model.VendorGoogle,
					Blob:           "blob",
					Package:        "package",
					SubscriptionID: "brave.leo.monthly",
				},
			},
			exp: tcExpected{err: model.Error("something went wrong")},
		},

		{
			name: "successful_case",
			given: tcGiven{
				svc: &mockPaidOrderCreator{
					fnCreateOrder: func(ctx context.Context, req *model.CreateOrderRequestNew, ordNew *model.OrderNew, items []model.OrderItem) (*model.Order, error) {
						result := &model.Order{
							ID: uuid.Must(uuid.FromString("1b251573-a45a-4f57-89f7-93b7da538817")),
							Items: []model.OrderItem{
								{ID: uuid.Must(uuid.FromString("22482ad4-e43b-44bd-860e-99e617ad9f6d"))},
							},
						}

						return result, nil
					},
				},
				set:   newOrderItemReqNewLeoSet("development"),
				ppcfg: newPaymentProcessorConfig("development"),
				req: model.ReceiptRequest{
					Type:           model.VendorGoogle,
					Blob:           "blob",
					Package:        "package",
					SubscriptionID: "brave.leo.monthly",
				},
			},
			exp: tcExpected{
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("1b251573-a45a-4f57-89f7-93b7da538817")),
					Items: []model.OrderItem{
						{ID: uuid.Must(uuid.FromString("22482ad4-e43b-44bd-860e-99e617ad9f6d"))},
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, err := createOrderWithReceipt(context.Background(), tc.given.svc, tc.given.set, tc.given.ppcfg, tc.given.req, tc.given.extID)
			must.Equal(t, tc.exp.err, err)

			if tc.exp.err != nil {
				return
			}

			should.Equal(t, tc.exp.ord, actual)
		})
	}
}

func TestService_checkOrderReceipt(t *testing.T) {
	type tcGiven struct {
		orderID uuid.UUID
		extID   string
		repo    *repository.MockOrder
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   error
	}

	tests := []testCase{
		{
			name: "order_not_found",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
				extID:   "extID_01",
				repo: &repository.MockOrder{
					FnGetByExternalID: func(ctx context.Context, dbi sqlx.QueryerContext, extID string) (*model.Order, error) {
						return nil, model.ErrOrderNotFound
					},
				},
			},
			exp: model.ErrOrderNotFound,
		},

		{
			name: "some_error",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
				extID:   "extID_01",
				repo: &repository.MockOrder{
					FnGetByExternalID: func(ctx context.Context, dbi sqlx.QueryerContext, extID string) (*model.Order, error) {
						return nil, model.Error("some error")
					},
				},
			},
			exp: model.Error("some error"),
		},

		{
			name: "order_receipt_dont_match",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
				extID:   "extID_01",
				repo: &repository.MockOrder{
					FnGetByExternalID: func(ctx context.Context, dbi sqlx.QueryerContext, extID string) (*model.Order, error) {
						result := &model.Order{
							ID: uuid.Must(uuid.FromString("decade00-0000-4000-a000-000000000000")),
						}

						return result, nil
					},
				},
			},
			exp: model.ErrNoMatchOrderReceipt,
		},

		{
			name: "happy_path",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
				extID:   "extID_01",
				repo: &repository.MockOrder{
					FnGetByExternalID: func(ctx context.Context, dbi sqlx.QueryerContext, extID string) (*model.Order, error) {
						result := &model.Order{
							ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
						}

						return result, nil
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			actual := checkOrderReceipt(ctx, nil, tc.given.repo, tc.given.orderID, tc.given.extID)
			should.Equal(t, tc.exp, actual)
		})
	}
}

type mockPaidOrderCreator struct {
	fnCreateOrder                       func(ctx context.Context, req *model.CreateOrderRequestNew, ordNew *model.OrderNew, items []model.OrderItem) (*model.Order, error)
	fnUpdateOrderStatusPaidWithMetadata func(ctx context.Context, oid *uuid.UUID, mdata datastore.Metadata) error
}

func (s *mockPaidOrderCreator) createOrder(ctx context.Context, req *model.CreateOrderRequestNew, ordNew *model.OrderNew, items []model.OrderItem) (*model.Order, error) {
	if s.fnCreateOrder == nil {
		return &model.Order{}, nil
	}

	return s.fnCreateOrder(ctx, req, ordNew, items)
}

func (s *mockPaidOrderCreator) UpdateOrderStatusPaidWithMetadata(ctx context.Context, oid *uuid.UUID, mdata datastore.Metadata) error {
	if s.fnUpdateOrderStatusPaidWithMetadata == nil {
		return nil
	}

	return s.fnUpdateOrderStatusPaidWithMetadata(ctx, oid, mdata)
}
