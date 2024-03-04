//go:build integration

package skus

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/brave-intl/bat-go/libs/ptr"
	"github.com/brave-intl/bat-go/services/skus/storage/repository"
	"github.com/jmoiron/sqlx"
	"github.com/shopspring/decimal"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"
	"google.golang.org/api/androidpublisher/v3"

	"github.com/brave-intl/bat-go/libs/datastore"
	timeutils "github.com/brave-intl/bat-go/libs/time"
	"github.com/brave-intl/bat-go/services/skus/model"
)

func TestCredChunkFn(t *testing.T) {
	// Jan 1, 2021
	issued := time.Date(2021, time.January, 20, 0, 0, 0, 0, time.UTC)

	// 1 day
	day, err := timeutils.ParseDuration("P1D")
	if err != nil {
		t.Errorf("failed to parse 1 day: %s", err.Error())
	}

	// 1 month
	mo, err := timeutils.ParseDuration("P1M")
	if err != nil {
		t.Errorf("failed to parse 1 month: %s", err.Error())
	}

	this, next := credChunkFn(*day)(issued)
	if this.Day() != 20 {
		t.Errorf("day - the next day should be 2")
	}
	if this.Month() != 1 {
		t.Errorf("day - the next month should be 1")
	}
	if next.Day() != 21 {
		t.Errorf("day - the next day should be 2")
	}
	if next.Month() != 1 {
		t.Errorf("day - the next month should be 1")
	}

	this, next = credChunkFn(*mo)(issued)
	if this.Day() != 1 {
		t.Errorf("mo - the next day should be 1")
	}
	if this.Month() != 1 {
		t.Errorf("mo - the next month should be 2")
	}
	if next.Day() != 1 {
		t.Errorf("mo - the next day should be 1")
	}
	if next.Month() != 2 {
		t.Errorf("mo - the next month should be 2")
	}
}

func TestCreateOrderItems(t *testing.T) {
	type tcExpected struct {
		result []model.OrderItem
		err    error
	}

	type testCase struct {
		name  string
		given *model.CreateOrderRequestNew
		exp   tcExpected
	}

	tests := []testCase{
		{
			name:  "empty",
			given: &model.CreateOrderRequestNew{},
			exp: tcExpected{
				result: []model.OrderItem{},
			},
		},

		{
			name: "invalid_CredentialValidDurationEach",
			given: &model.CreateOrderRequestNew{
				Items: []model.OrderItemRequestNew{
					{
						CredentialValidDuration:     "P1M",
						CredentialValidDurationEach: ptr.To("rubbish"),
					},
				},
			},
			exp: tcExpected{
				err: timeutils.ErrUnsupportedFormat,
			},
		},

		{
			name: "ensure_currency_set",
			given: &model.CreateOrderRequestNew{
				Currency: "USD",
				Items: []model.OrderItemRequestNew{
					{
						Location:                "location",
						Description:             "description",
						CredentialValidDuration: "P1M",
					},

					{
						Location:                "location",
						Description:             "description",
						CredentialValidDuration: "P1M",
					},
				},
			},
			exp: tcExpected{
				result: []model.OrderItem{
					{
						Currency:    "USD",
						ValidForISO: ptr.To("P1M"),
						Location: datastore.NullString{
							NullString: sql.NullString{
								Valid:  true,
								String: "location",
							},
						},
						Description: datastore.NullString{
							NullString: sql.NullString{
								Valid:  true,
								String: "description",
							},
						},
						IssuerConfig: &model.IssuerConfig{
							Buffer:  30,
							Overlap: 5,
						},
						Subtotal: decimal.NewFromInt(0),
					},

					{
						Currency:    "USD",
						ValidForISO: ptr.To("P1M"),
						Location: datastore.NullString{
							NullString: sql.NullString{
								Valid:  true,
								String: "location",
							},
						},
						Description: datastore.NullString{
							NullString: sql.NullString{
								Valid:  true,
								String: "description",
							},
						},

						IssuerConfig: &model.IssuerConfig{
							Buffer:  30,
							Overlap: 5,
						},
						Subtotal: decimal.NewFromInt(0),
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			act, err := createOrderItems(tc.given)
			must.Equal(t, true, errors.Is(err, tc.exp.err))

			if tc.exp.err != nil {
				return
			}

			must.Equal(t, len(tc.exp.result), len(act))

			// Override ValidFor because it's not deterministic.
			for j := range act {
				tc.exp.result[j].ValidFor = act[j].ValidFor
			}

			should.Equal(t, tc.exp.result, act)
		})
	}
}

func TestCreateOrderItem(t *testing.T) {
	type tcExpected struct {
		result *model.OrderItem
		err    error
	}

	type testCase struct {
		name  string
		given *model.OrderItemRequestNew
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "invalid_CredentialValidDurationEach",
			given: &model.OrderItemRequestNew{
				CredentialValidDurationEach: ptr.To("rubbish"),
			},
			exp: tcExpected{
				err: timeutils.ErrUnsupportedFormat,
			},
		},

		{
			name: "invalid_CredentialValidDuration",
			given: &model.OrderItemRequestNew{
				CredentialValidDuration:     "rubbish",
				CredentialValidDurationEach: ptr.To("P1M"),
			},
			exp: tcExpected{
				err: timeutils.ErrUnsupportedFormat,
			},
		},

		{
			name: "full_example",
			given: &model.OrderItemRequestNew{
				SKU:                         "sku",
				CredentialType:              "credential_type",
				CredentialValidDuration:     "P1M",
				CredentialValidDurationEach: ptr.To("P1D"),
				IssuanceInterval:            ptr.To("P1M"),
				Price:                       decimal.NewFromInt(10),
				Location:                    "location",
				Description:                 "description",
				Quantity:                    2,
				StripeMetadata: &model.ItemStripeMetadata{
					ProductID: "product_id",
					ItemID:    "item_id",
				},
				IssuerTokenBuffer: 10,
			},
			exp: tcExpected{
				result: &model.OrderItem{
					SKU:                       "sku",
					CredentialType:            "credential_type",
					ValidFor:                  mustDurationFromISO("P1M"),
					ValidForISO:               ptr.To("P1M"),
					EachCredentialValidForISO: ptr.To("P1D"),
					IssuanceIntervalISO:       ptr.To("P1M"),
					Price:                     decimal.NewFromInt(10),
					Location: datastore.NullString{
						NullString: sql.NullString{
							Valid:  true,
							String: "location",
						},
					},
					Description: datastore.NullString{
						NullString: sql.NullString{
							Valid:  true,
							String: "description",
						},
					},
					Quantity: 2,
					Metadata: map[string]interface{}{
						"stripe_product_id": "product_id",
						"stripe_item_id":    "item_id",
					},
					Subtotal: decimal.NewFromInt(10).Mul(decimal.NewFromInt(int64(2))),
					IssuerConfig: &model.IssuerConfig{
						Buffer:  10,
						Overlap: 5,
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			act, err := createOrderItem(tc.given)
			must.Equal(t, true, errors.Is(err, tc.exp.err))

			if tc.exp.err != nil {
				return
			}

			// Override ValidFor because it's not deterministic.
			tc.exp.result.ValidFor = act.ValidFor

			should.Equal(t, tc.exp.result, act)
		})
	}
}

func TestUpdateAndroidSubscription(t *testing.T) {
	type tcGiven struct {
		rec model.ReceiptRequest
		s   Service
	}

	type tcExpected struct {
		assertErr should.ErrorAssertionFunc
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "get_order_by_external_id_error",
			given: tcGiven{
				rec: model.ReceiptRequest{
					Type:           model.VendorGoogle,
					Blob:           "blob",
					Package:        "package",
					SubscriptionID: "subscriptionId",
				},
				s: Service{
					Datastore: mockDB(t),
					orderRepo: &repository.MockOrder{
						FnGetByExternalID: func(ctx context.Context, dbi sqlx.QueryerContext, extID string) (*model.Order, error) {
							return nil, errors.New("error")
						},
					},
				},
			},
			exp: tcExpected{
				assertErr: func(t should.TestingT, err error, i ...interface{}) bool {
					return should.ErrorContains(t, err, "failed to get order from db: ")
				},
			},
		},
		{
			name: "no_order",
			given: tcGiven{
				rec: model.ReceiptRequest{
					Type:           model.VendorGoogle,
					Blob:           "blob",
					Package:        "package",
					SubscriptionID: "subscriptionId",
				},
				s: Service{
					Datastore: mockDB(t),
					orderRepo: &repository.MockOrder{
						FnGetByExternalID: func(ctx context.Context, dbi sqlx.QueryerContext, extID string) (*model.Order, error) {
							return nil, nil
						},
					},
				},
			},
			exp: tcExpected{
				assertErr: func(t should.TestingT, err error, i ...interface{}) bool {
					return should.ErrorIs(t, err, model.ErrOrderNotFound)
				},
			},
		},
		{
			name: "get_subscription_purchase_error",
			given: tcGiven{
				rec: model.ReceiptRequest{
					Type:           model.VendorGoogle,
					Blob:           "blob",
					Package:        "package",
					SubscriptionID: "subscriptionId",
				},
				s: Service{
					Datastore: mockDB(t),
					orderRepo: &repository.MockOrder{},
					androidSubPurchaserV2: &mockAndroidPublisherV2{
						fnGetSubscriptionPurchase: func(_ context.Context, pkgName, token string) (*androidpublisher.SubscriptionPurchaseV2, error) {
							return nil, errors.New("error")
						},
					},
				},
			},
			exp: tcExpected{
				assertErr: func(t should.TestingT, err error, i ...interface{}) bool {
					return should.ErrorContains(t, err, "error")
				},
			},
		},
		{
			name: "line_item_not_found",
			given: tcGiven{
				rec: model.ReceiptRequest{
					Type:           model.VendorGoogle,
					Blob:           "blob",
					Package:        "package",
					SubscriptionID: "subscriptionId",
				},
				s: Service{
					Datastore: mockDB(t),
					orderRepo: &repository.MockOrder{},
					androidSubPurchaserV2: &mockAndroidPublisherV2{
						fnGetSubscriptionPurchase: func(_ context.Context, pkgName, token string) (*androidpublisher.SubscriptionPurchaseV2, error) {
							return &androidpublisher.SubscriptionPurchaseV2{
								LineItems: []*androidpublisher.SubscriptionPurchaseLineItem{
									{
										ProductId:  "ProductId",
										ExpiryTime: time.Now().Format(time.RFC3339Nano),
									},
								},
								SubscriptionState: stateActive,
							}, nil
						},
					},
				},
			},
			exp: tcExpected{
				assertErr: func(t should.TestingT, err error, i ...interface{}) bool {
					return errors.Is(err, errLineItemNotFound)
				},
			},
		},
		{
			name: "time_parse_error",
			given: tcGiven{
				rec: model.ReceiptRequest{
					Type:           model.VendorGoogle,
					Blob:           "blob",
					Package:        "package",
					SubscriptionID: "subscriptionId",
				},
				s: Service{
					Datastore: mockDB(t),
					orderRepo: &repository.MockOrder{},
					androidSubPurchaserV2: &mockAndroidPublisherV2{
						fnGetSubscriptionPurchase: func(_ context.Context, pkgName, token string) (*androidpublisher.SubscriptionPurchaseV2, error) {
							return &androidpublisher.SubscriptionPurchaseV2{
								LineItems: []*androidpublisher.SubscriptionPurchaseLineItem{
									{
										ProductId:  "subscriptionId",
										ExpiryTime: time.Now().Format(time.RFC1123),
									},
								},
								SubscriptionState: stateActive,
							}, nil
						},
					},
				},
			},
			exp: tcExpected{
				assertErr: func(t should.TestingT, err error, i ...interface{}) bool {
					return should.ErrorContains(t, err, "error parsing item time: ")
				},
			},
		},
		{
			name: "success",
			given: tcGiven{
				rec: model.ReceiptRequest{
					Type:           model.VendorGoogle,
					Blob:           "blob",
					Package:        "package",
					SubscriptionID: "subscriptionId",
				},
				s: Service{
					Datastore: mockDB(t),
					orderRepo: &repository.MockOrder{},
					androidSubPurchaserV2: &mockAndroidPublisherV2{
						fnGetSubscriptionPurchase: func(_ context.Context, pkgName, token string) (*androidpublisher.SubscriptionPurchaseV2, error) {
							return &androidpublisher.SubscriptionPurchaseV2{
								LineItems: []*androidpublisher.SubscriptionPurchaseLineItem{
									{
										ProductId:  "subscriptionId",
										ExpiryTime: time.Now().Format(time.RFC3339Nano),
									},
								},
								SubscriptionState: stateActive,
							}, nil
						},
					},
				},
			},
			exp: tcExpected{
				assertErr: func(t should.TestingT, err error, i ...interface{}) bool {
					return should.NoError(t, err)
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.s.updateOrderAndroid(context.TODO(), tc.given.rec)
			tc.exp.assertErr(t, actual)
		})
	}

}

func TestSubState(t *testing.T) {
	type tcGiven struct {
		state string
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   string
	}

	tests := []testCase{
		{
			name: "active",
			given: tcGiven{
				state: stateActive,
			},
			exp: OrderStatusPaid,
		},
		{
			name: "in_grace_period",
			given: tcGiven{
				state: stateInGracePeriod,
			},
			exp: OrderStatusPaid,
		},
		{
			name: "canceled",
			given: tcGiven{
				state: stateCanceled,
			},
			exp: OrderStatusCanceled,
		},
		{
			name: "unknown_state",
			given: tcGiven{
				state: "state",
			},
		},
		{
			name: "empty_state",
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := shouldUpdateOrderState(tc.given.state)
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestFindLineItem(t *testing.T) {
	type tcGiven struct {
		productID string
		lineItems []*androidpublisher.SubscriptionPurchaseLineItem
	}

	type exp struct {
		lineItem *androidpublisher.SubscriptionPurchaseLineItem
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   exp
	}

	tests := []testCase{
		{
			name: "found",
			given: tcGiven{
				productID: "ProductId",
				lineItems: []*androidpublisher.SubscriptionPurchaseLineItem{
					{
						ProductId: "ProductId",
					},
				},
			},
			exp: exp{
				lineItem: &androidpublisher.SubscriptionPurchaseLineItem{
					ProductId: "ProductId",
				},
			},
		},
		{
			name: "not_found",
			given: tcGiven{
				productID: "another-product-id",
				lineItems: []*androidpublisher.SubscriptionPurchaseLineItem{
					{
						ProductId: "ProductId",
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := findLineItem(tc.given.lineItems, tc.given.productID)
			should.Equal(t, tc.exp.lineItem, actual)
		})
	}
}

func mockDB(t *testing.T) Datastore {
	mdb, mock, err := sqlmock.New()
	must.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectCommit()

	dbi := sqlx.NewDb(mdb, "sqlmock")

	p := &Postgres{Postgres: datastore.Postgres{DB: dbi}}

	return p
}

type mockAndroidPublisherV2 struct {
	fnGetSubscriptionPurchase func(_ context.Context, pkgName, token string) (*androidpublisher.SubscriptionPurchaseV2, error)
}

func (m *mockAndroidPublisherV2) GetSubscriptionPurchase(_ context.Context, pkgName, token string) (*androidpublisher.SubscriptionPurchaseV2, error) {
	if m.fnGetSubscriptionPurchase == nil {
		return nil, nil
	}
	return m.fnGetSubscriptionPurchase(context.TODO(), pkgName, token)
}

func mustDurationFromISO(v string) *time.Duration {
	result, err := durationFromISO(v)
	if err != nil {
		panic(err)
	}

	return &result
}
