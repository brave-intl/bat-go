package skus

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/awa/go-iap/appstore"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v72"
	"google.golang.org/api/androidpublisher/v3"

	"github.com/brave-intl/bat-go/libs/clients/cbr"
	"github.com/brave-intl/bat-go/libs/datastore"
	berrs "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/handlers"

	"github.com/brave-intl/bat-go/services/skus/model"
	"github.com/brave-intl/bat-go/services/skus/radom"
	"github.com/brave-intl/bat-go/services/skus/storage/repository"
	"github.com/brave-intl/bat-go/services/skus/xstripe"
)

func TestService_uniqBatchesTxTime(t *testing.T) {
	type tcGiven struct {
		orderID  uuid.UUID
		itemID   uuid.UUID
		from     time.Time
		to       time.Time
		ordRepo  *repository.MockOrder
		itemRepo *repository.MockOrderItem
		tlv2Repo *repository.MockTLV2
	}

	type tcExpected struct {
		lim int
		val int
		err error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "order_not_found",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				itemID:  uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
				from:    time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				to:      time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				ordRepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						return nil, model.ErrOrderNotFound
					},
				},
				itemRepo: &repository.MockOrderItem{},
				tlv2Repo: &repository.MockTLV2{},
			},
			exp: tcExpected{
				err: model.ErrOrderNotFound,
			},
		},

		{
			name: "order_not_paid",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				itemID:  uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
				from:    time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				to:      time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				ordRepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID:     id,
							Status: "pending",
						}

						return result, nil
					},
				},
				itemRepo: &repository.MockOrderItem{},
				tlv2Repo: &repository.MockTLV2{},
			},
			exp: tcExpected{
				err: model.ErrOrderNotPaid,
			},
		},

		{
			name: "invalid_order_no_items",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				itemID:  uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
				from:    time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				to:      time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				ordRepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID:     id,
							Status: "paid",
						}

						return result, nil
					},
				},
				itemRepo: &repository.MockOrderItem{
					FnFindByOrderID: func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) ([]model.OrderItem, error) {
						return []model.OrderItem{}, nil
					},
				},
				tlv2Repo: &repository.MockTLV2{},
			},
			exp: tcExpected{
				err: model.ErrInvalidOrderNoItems,
			},
		},

		{
			name: "legacy_default_item_unsupported_cred_type",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				itemID:  uuid.Nil,
				from:    time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				to:      time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				ordRepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID:     id,
							Status: "paid",
						}

						return result, nil
					},
				},
				itemRepo: &repository.MockOrderItem{
					FnFindByOrderID: func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) ([]model.OrderItem, error) {
						result := []model.OrderItem{
							{
								ID:      uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
								OrderID: uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
							},
						}

						return result, nil
					},
				},
				tlv2Repo: &repository.MockTLV2{},
			},
			exp: tcExpected{
				err: model.ErrUnsupportedCredType,
			},
		},

		{
			name: "legacy_default_item_uniq_batches_error",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				itemID:  uuid.Nil,
				from:    time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				to:      time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				ordRepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID:     id,
							Status: "paid",
						}

						return result, nil
					},
				},
				itemRepo: &repository.MockOrderItem{
					FnFindByOrderID: func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) ([]model.OrderItem, error) {
						result := []model.OrderItem{
							{
								ID:             uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
								OrderID:        uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
								CredentialType: "time-limited-v2",
							},
						}

						return result, nil
					},
				},
				tlv2Repo: &repository.MockTLV2{
					FnUniqBatches: func(ctx context.Context, dbi sqlx.QueryerContext, orderID, itemID uuid.UUID, from, to time.Time) (int, error) {
						return 0, model.Error("something_went_wrong")
					},
				},
			},
			exp: tcExpected{
				err: model.Error("something_went_wrong"),
			},
		},

		{
			name: "legacy_default_item_success",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				itemID:  uuid.Nil,
				from:    time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				to:      time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				ordRepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID:     id,
							Status: "paid",
						}

						return result, nil
					},
				},
				itemRepo: &repository.MockOrderItem{
					FnFindByOrderID: func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) ([]model.OrderItem, error) {
						result := []model.OrderItem{
							{
								ID:             uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
								OrderID:        uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
								CredentialType: "time-limited-v2",
							},
						}

						return result, nil
					},
				},
				tlv2Repo: &repository.MockTLV2{
					FnUniqBatches: func(ctx context.Context, dbi sqlx.QueryerContext, orderID, itemID uuid.UUID, from, to time.Time) (int, error) {
						return 1, nil
					},
				},
			},
			exp: tcExpected{
				lim: 10,
				val: 1,
			},
		},

		{
			name: "explicit_item_not_found",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				itemID:  uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
				from:    time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				to:      time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				ordRepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID:     id,
							Status: "paid",
						}

						return result, nil
					},
				},
				itemRepo: &repository.MockOrderItem{
					FnFindByOrderID: func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) ([]model.OrderItem, error) {
						result := []model.OrderItem{
							{
								ID:      uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000001")),
								OrderID: uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
							},
						}

						return result, nil
					},
				},
				tlv2Repo: &repository.MockTLV2{},
			},
			exp: tcExpected{
				err: model.ErrOrderItemNotFound,
			},
		},

		{
			name: "explicit_item_success",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				itemID:  uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
				from:    time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				to:      time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				ordRepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID:     id,
							Status: "paid",
						}

						return result, nil
					},
				},
				itemRepo: &repository.MockOrderItem{
					FnFindByOrderID: func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) ([]model.OrderItem, error) {
						result := []model.OrderItem{
							{
								ID:             uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
								OrderID:        uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
								CredentialType: "time-limited-v2",
							},
						}

						return result, nil
					},
				},
				tlv2Repo: &repository.MockTLV2{
					FnUniqBatches: func(ctx context.Context, dbi sqlx.QueryerContext, orderID, itemID uuid.UUID, from, to time.Time) (int, error) {
						return 2, nil
					},
				},
			},
			exp: tcExpected{
				lim: 10,
				val: 2,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{
				orderRepo:     tc.given.ordRepo,
				orderItemRepo: tc.given.itemRepo,
				tlv2Repo:      tc.given.tlv2Repo,
			}

			ctx := context.Background()

			lim, nact, err := svc.uniqBatchesTxTime(ctx, nil, tc.given.orderID, tc.given.itemID, tc.given.from, tc.given.to)
			must.Equal(t, true, errors.Is(err, tc.exp.err))

			if tc.exp.err != nil {
				return
			}

			should.Equal(t, tc.exp.lim, lim)
			should.Equal(t, tc.exp.val, nact)
		})
	}
}

func TestService_processPlayStoreNotificationTx(t *testing.T) {
	type tcGiven struct {
		extID string
		ntf   *playStoreDevNotification
		orepo *repository.MockOrder
		prepo *repository.MockOrderPayHistory
		pscl  *mockPSClient
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   error
	}

	tests := []testCase{
		{
			name: "get_order_error",
			given: tcGiven{
				extID: "PURCHASE_TOKEN_01",
				ntf: &playStoreDevNotification{
					PackageName:    "com.brave.browser_nightly",
					EventTimeMilli: "1717200001000", // 2024-06-01 00:00:01
					SubscriptionNtf: &playStoreSubscriptionNtf{
						Type:          2,
						PurchaseToken: "PURCHASE_TOKEN_01",
						SubID:         "nightly.bravevpn.monthly",
					},
				},
				orepo: &repository.MockOrder{
					FnGetByExternalID: func(ctx context.Context, dbi sqlx.QueryerContext, extID string) (*model.Order, error) {
						return nil, model.Error("something_went_wrong")
					},
				},
				prepo: &repository.MockOrderPayHistory{},
				pscl:  &mockPSClient{},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "sub_should_renew_fetch_error",
			given: tcGiven{
				extID: "PURCHASE_TOKEN_01",
				ntf: &playStoreDevNotification{
					PackageName:    "com.brave.browser_nightly",
					EventTimeMilli: "1717200001000", // 2024-06-01 00:00:01
					SubscriptionNtf: &playStoreSubscriptionNtf{
						Type:          2,
						PurchaseToken: "PURCHASE_TOKEN_01",
						SubID:         "nightly.bravevpn.monthly",
					},
				},
				orepo: &repository.MockOrder{},
				prepo: &repository.MockOrderPayHistory{},
				pscl: &mockPSClient{
					fnVerifySubscription: func(ctx context.Context, pkgName, subID, token string) (*androidpublisher.SubscriptionPurchase, error) {
						return nil, model.Error("something_went_wrong")
					},
				},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "sub_should_renew",
			given: tcGiven{
				extID: "PURCHASE_TOKEN_01",
				ntf: &playStoreDevNotification{
					PackageName:    "com.brave.browser_nightly",
					EventTimeMilli: json.Number(strconv.FormatInt(time.Now().UnixMilli(), 10)),
					SubscriptionNtf: &playStoreSubscriptionNtf{
						Type:          2,
						PurchaseToken: "PURCHASE_TOKEN_01",
						SubID:         "nightly.bravevpn.monthly",
					},
				},
				orepo: &repository.MockOrder{
					FnSetExpiresAt: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
						if when.Equal(time.Date(2024, time.July, 2, 0, 0, 0, 0, time.UTC)) {
							return nil
						}

						return model.Error("unexpected")
					},
				},
				prepo: &repository.MockOrderPayHistory{},
				pscl: &mockPSClient{
					fnVerifySubscription: func(ctx context.Context, pkgName, subID, token string) (*androidpublisher.SubscriptionPurchase, error) {
						result := &androidpublisher.SubscriptionPurchase{
							PaymentState:     ptrTo[int64](1),
							ExpiryTimeMillis: time.Date(2024, time.July, 1, 0, 0, 0, 0, time.UTC).UnixMilli(),
						}

						return result, nil
					},
				},
			},
		},

		{
			name: "sub_should_cancel",
			given: tcGiven{
				extID: "PURCHASE_TOKEN_01",
				ntf: &playStoreDevNotification{
					PackageName:    "com.brave.browser_nightly",
					EventTimeMilli: json.Number(strconv.FormatInt(time.Now().UnixMilli(), 10)),
					SubscriptionNtf: &playStoreSubscriptionNtf{
						Type:          3,
						PurchaseToken: "PURCHASE_TOKEN_01",
						SubID:         "nightly.bravevpn.monthly",
					},
				},
				orepo: &repository.MockOrder{},
				prepo: &repository.MockOrderPayHistory{},
				pscl:  &mockPSClient{},
			},
		},

		{
			name: "void_should_cancel",
			given: tcGiven{
				extID: "PURCHASE_TOKEN_01",
				ntf: &playStoreDevNotification{
					PackageName:       "com.brave.browser_nightly",
					EventTimeMilli:    json.Number(strconv.FormatInt(time.Now().UnixMilli(), 10)),
					VoidedPurchaseNtf: &playStoreVoidedPurchaseNtf{ProductType: 1},
				},
				orepo: &repository.MockOrder{},
				prepo: &repository.MockOrderPayHistory{},
				pscl:  &mockPSClient{},
			},
		},

		{
			name: "skip_sub",
			given: tcGiven{
				extID: "PURCHASE_TOKEN_01",
				ntf: &playStoreDevNotification{
					PackageName:    "com.brave.browser_nightly",
					EventTimeMilli: json.Number(strconv.FormatInt(time.Now().UnixMilli(), 10)),
					SubscriptionNtf: &playStoreSubscriptionNtf{
						Type:          20,
						PurchaseToken: "PURCHASE_TOKEN_01",
						SubID:         "nightly.bravevpn.monthly",
					},
				},
				orepo: &repository.MockOrder{},
				prepo: &repository.MockOrderPayHistory{},
				pscl:  &mockPSClient{},
			},
		},

		{
			name: "skip_void",
			given: tcGiven{
				extID: "PURCHASE_TOKEN_01",
				ntf: &playStoreDevNotification{
					PackageName:       "com.brave.browser_nightly",
					EventTimeMilli:    json.Number(strconv.FormatInt(time.Now().UnixMilli(), 10)),
					VoidedPurchaseNtf: &playStoreVoidedPurchaseNtf{ProductType: 2},
				},
				orepo: &repository.MockOrder{},
				prepo: &repository.MockOrderPayHistory{},
				pscl:  &mockPSClient{},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{
				orderRepo:          tc.given.orepo,
				payHistRepo:        tc.given.prepo,
				vendorReceiptValid: &receiptVerifier{playStoreCl: tc.given.pscl},
			}

			ctx := context.Background()

			err := svc.processPlayStoreNotificationTx(ctx, nil, tc.given.ntf, tc.given.extID)
			should.Equal(t, true, errors.Is(err, tc.exp))
		})
	}
}

func TestService_processAppStoreNotificationTx(t *testing.T) {
	type tcGiven struct {
		ntf   *appStoreSrvNotification
		txn   *appStoreTransaction
		orepo *repository.MockOrder
		prepo *repository.MockOrderPayHistory
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   error
	}

	tests := []testCase{
		{
			name: "get_order_error",
			given: tcGiven{
				ntf: &appStoreSrvNotification{val: &appstore.SubscriptionNotificationV2DecodedPayload{}},
				txn: &appStoreTransaction{OriginalTransactionId: "123456789000001"},
				orepo: &repository.MockOrder{
					FnGetByExternalID: func(ctx context.Context, dbi sqlx.QueryerContext, extID string) (*model.Order, error) {
						return nil, model.Error("something_went_wrong")
					},
				},
				prepo: &repository.MockOrderPayHistory{},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "should_renew",
			given: tcGiven{
				ntf: &appStoreSrvNotification{
					val: &appstore.SubscriptionNotificationV2DecodedPayload{
						NotificationType: appstore.NotificationTypeV2DidRenew,
						Subtype:          appstore.SubTypeV2BillingRecovery,
					},
				},
				txn: &appStoreTransaction{
					OriginalTransactionId: "123456789000001",
					ExpiresDate:           1704067200000,
				},

				orepo: &repository.MockOrder{
					FnSetExpiresAt: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
						if when.Equal(time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC)) {
							return nil
						}

						return model.Error("unexpected")
					},
				},
				prepo: &repository.MockOrderPayHistory{},
			},
		},

		{
			name: "should_cancel",
			given: tcGiven{
				ntf: &appStoreSrvNotification{
					val: &appstore.SubscriptionNotificationV2DecodedPayload{
						NotificationType: appstore.NotificationTypeV2DidChangeRenewalStatus,
						Subtype:          appstore.SubTypeV2AutoRenewDisabled,
					},
				},
				txn: &appStoreTransaction{
					OriginalTransactionId: "123456789000001",
					ExpiresDate:           1704067201000,
				},

				orepo: &repository.MockOrder{},
				prepo: &repository.MockOrderPayHistory{},
			},
		},

		{
			name: "anything_else",
			given: tcGiven{
				ntf: &appStoreSrvNotification{
					val: &appstore.SubscriptionNotificationV2DecodedPayload{
						NotificationType: appstore.NotificationTypeV2PriceIncrease,
						Subtype:          appstore.SubTypeV2Accepted,
					},
				},
				txn: &appStoreTransaction{
					OriginalTransactionId: "123456789000001",
					ExpiresDate:           1704067201000,
				},

				orepo: &repository.MockOrder{},
				prepo: &repository.MockOrderPayHistory{},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{
				orderRepo:   tc.given.orepo,
				payHistRepo: tc.given.prepo,
			}

			ctx := context.Background()

			err := svc.processAppStoreNotificationTx(ctx, nil, tc.given.ntf, tc.given.txn)
			should.Equal(t, true, errors.Is(err, tc.exp))
		})
	}
}

func TestService_renewOrderWithExpPaidTimeTx(t *testing.T) {
	type tcGiven struct {
		id    uuid.UUID
		expt  time.Time
		paidt time.Time
		orepo *repository.MockOrder
		prepo *repository.MockOrderPayHistory
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   error
	}

	tests := []testCase{
		{
			name: "set_status_failed",
			given: tcGiven{
				id:    uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				expt:  time.UnixMilli(1735689599000),
				paidt: time.UnixMilli(1704067201000),
				orepo: &repository.MockOrder{
					FnSetStatus: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, status string) error {
						return model.Error("something_went_wrong")
					},
				},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "set_expires_at_failed",
			given: tcGiven{
				id:    uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				expt:  time.UnixMilli(1735689599000),
				paidt: time.UnixMilli(1704067201000),
				orepo: &repository.MockOrder{
					FnSetExpiresAt: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
						return model.Error("something_went_wrong")
					},
				},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "set_last_paid_at_failed",
			given: tcGiven{
				id:    uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				expt:  time.UnixMilli(1735689599000),
				paidt: time.UnixMilli(1704067201000),
				orepo: &repository.MockOrder{
					FnSetLastPaidAt: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
						return model.Error("something_went_wrong")
					},
				},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "insert_pay_history_failed",
			given: tcGiven{
				id:    uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				expt:  time.UnixMilli(1735689599000),
				paidt: time.UnixMilli(1704067201000),
				orepo: &repository.MockOrder{},
				prepo: &repository.MockOrderPayHistory{
					FnInsert: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
						return model.Error("something_went_wrong")
					},
				},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "success",
			given: tcGiven{
				id:    uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				expt:  time.UnixMilli(1735689599000),
				paidt: time.UnixMilli(1704067201000),
				orepo: &repository.MockOrder{
					FnSetStatus: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, status string) error {
						if !uuid.Equal(id, uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000"))) {
							return model.Error("unexpected: id")
						}

						if status != model.OrderStatusPaid {
							return model.Error("unexpected: status")
						}

						return nil
					},

					FnSetExpiresAt: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
						if !uuid.Equal(id, uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000"))) {
							return model.Error("unexpected: id")
						}

						if !when.Equal(time.UnixMilli(1735689599000)) {
							return model.Error("unexpected: expt")
						}

						return nil
					},

					FnSetLastPaidAt: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
						if !uuid.Equal(id, uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000"))) {
							return model.Error("unexpected: id")
						}

						if !when.Equal(time.UnixMilli(1704067201000)) {
							return model.Error("unexpected: expt")
						}

						return nil
					},
				},
				prepo: &repository.MockOrderPayHistory{
					FnInsert: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
						if !uuid.Equal(id, uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000"))) {
							return model.Error("unexpected: id")
						}

						if !when.Equal(time.UnixMilli(1704067201000)) {
							return model.Error("unexpected: expt")
						}

						return nil
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{
				orderRepo:   tc.given.orepo,
				payHistRepo: tc.given.prepo,
			}

			ctx := context.Background()

			err := svc.renewOrderWithExpPaidTimeTx(ctx, nil, tc.given.id, tc.given.expt, tc.given.paidt)
			should.Equal(t, true, errors.Is(err, tc.exp))
		})
	}
}

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
		vnd   model.Vendor
		extID string
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
				vnd:   model.VendorGoogle,
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
				vnd:   model.VendorApple,
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
			actual := newMobileOrderMdata(tc.given.vnd, tc.given.extID)
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
						SKUVnt:         "sku",
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
						SKUVnt:         "sku",
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
						SKUVnt:         "sku01",
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
						SKUVnt:         "sku02",
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
						SKUVnt:         "sku01",
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
						SKUVnt:         "sku02",
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
						SKUVnt:         "sku",
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
		rcpt  model.ReceiptData
		paidt time.Time
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
				set:   newOrderItemReqNewMobileSet("development"),
				ppcfg: newPaymentProcessorConfig("development"),
				rcpt: model.ReceiptData{
					Type:      model.VendorGoogle,
					ProductID: "invalid",
					ExtID:     "blob",
					ExpiresAt: time.Now().Add(15 * 24 * time.Hour),
				},
				paidt: time.Now(),
			},
			exp: tcExpected{err: model.ErrInvalidMobileProduct},
		},

		{
			name: "error_in_createOrderPremium",
			given: tcGiven{
				svc: &mockPaidOrderCreator{
					fnCreateOrderPremium: func(ctx context.Context, req *model.CreateOrderRequestNew, ordNew *model.OrderNew, items []model.OrderItem) (*model.Order, error) {
						return nil, model.Error("something_went_wrong")
					},
				},
				set:   newOrderItemReqNewMobileSet("development"),
				ppcfg: newPaymentProcessorConfig("development"),
				rcpt: model.ReceiptData{
					Type:      model.VendorGoogle,
					ProductID: "brave.leo.monthly",
					ExtID:     "blob",
					ExpiresAt: time.Now().Add(15 * 24 * time.Hour),
				},
				paidt: time.Now(),
			},
			exp: tcExpected{err: model.Error("something_went_wrong")},
		},

		{
			name: "error_in_renewOrderWithExpPaidTime",
			given: tcGiven{
				svc: &mockPaidOrderCreator{
					fnCreateOrderPremium: func(ctx context.Context, req *model.CreateOrderRequestNew, ordNew *model.OrderNew, items []model.OrderItem) (*model.Order, error) {
						return &model.Order{}, nil
					},

					fnRenewOrderWithExpPaidTime: func(ctx context.Context, id uuid.UUID, expt, paidt time.Time) error {
						return model.Error("something_went_wrong")
					},
				},
				set:   newOrderItemReqNewMobileSet("development"),
				ppcfg: newPaymentProcessorConfig("development"),
				rcpt: model.ReceiptData{
					Type:      model.VendorGoogle,
					ProductID: "brave.leo.monthly",
					ExtID:     "blob",
					ExpiresAt: time.Now().Add(15 * 24 * time.Hour),
				},
				paidt: time.Now(),
			},
			exp: tcExpected{err: model.Error("something_went_wrong")},
		},

		{
			name: "error_in_appendOrderMetadata",
			given: tcGiven{
				svc: &mockPaidOrderCreator{
					fnCreateOrderPremium: func(ctx context.Context, req *model.CreateOrderRequestNew, ordNew *model.OrderNew, items []model.OrderItem) (*model.Order, error) {
						return &model.Order{}, nil
					},

					fnAppendOrderMetadata: func(ctx context.Context, oid uuid.UUID, mdata datastore.Metadata) error {
						return model.Error("something_went_wrong")
					},
				},
				set:   newOrderItemReqNewMobileSet("development"),
				ppcfg: newPaymentProcessorConfig("development"),
				rcpt: model.ReceiptData{
					Type:      model.VendorGoogle,
					ProductID: "brave.leo.monthly",
					ExtID:     "blob",
					ExpiresAt: time.Now().Add(15 * 24 * time.Hour),
				},
				paidt: time.Now(),
			},
			exp: tcExpected{err: model.Error("something_went_wrong")},
		},

		{
			name: "successful_case_android_leo_monthly",
			given: tcGiven{
				svc: &mockPaidOrderCreator{
					fnCreateOrderPremium: func(ctx context.Context, req *model.CreateOrderRequestNew, ordNew *model.OrderNew, items []model.OrderItem) (*model.Order, error) {
						result := &model.Order{
							ID: uuid.Must(uuid.FromString("1b251573-a45a-4f57-89f7-93b7da538817")),
							Items: []model.OrderItem{
								{ID: uuid.Must(uuid.FromString("22482ad4-e43b-44bd-860e-99e617ad9f6d"))},
							},
						}

						return result, nil
					},

					fnRenewOrderWithExpPaidTime: func(ctx context.Context, id uuid.UUID, expt, paidt time.Time) error {
						if !expt.Equal(time.Date(2024, time.August, 1, 0, 0, 0, 0, time.UTC)) {
							return model.Error("unexpected_expt")
						}

						if !paidt.Equal(time.Date(2024, time.July, 1, 0, 0, 1, 0, time.UTC)) {
							return model.Error("unexpected_paidt")
						}

						return nil
					},

					fnAppendOrderMetadata: func(ctx context.Context, oid uuid.UUID, mdata datastore.Metadata) error {
						if mdata["externalID"] != "blob" {
							return model.Error("unexpected_externalID")
						}

						if mdata["paymentProcessor"] != "android" {
							return model.Error("unexpected_paymentProcessor")
						}

						if mdata["vendor"] != "android" {
							return model.Error("unexpected_vendor")
						}

						return nil
					},
				},
				set:   newOrderItemReqNewMobileSet("development"),
				ppcfg: newPaymentProcessorConfig("development"),
				rcpt: model.ReceiptData{
					Type:      model.VendorGoogle,
					ProductID: "brave.leo.monthly",
					ExtID:     "blob",
					ExpiresAt: time.Date(2024, time.August, 1, 0, 0, 0, 0, time.UTC),
				},
				paidt: time.Date(2024, time.July, 1, 0, 0, 1, 0, time.UTC),
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

		{
			name: "successful_case_android_vpn_monthly",
			given: tcGiven{
				svc: &mockPaidOrderCreator{
					fnCreateOrderPremium: func(ctx context.Context, req *model.CreateOrderRequestNew, ordNew *model.OrderNew, items []model.OrderItem) (*model.Order, error) {
						result := &model.Order{
							ID: uuid.Must(uuid.FromString("1b251573-a45a-4f57-89f7-93b7da538817")),
							Items: []model.OrderItem{
								{ID: uuid.Must(uuid.FromString("22482ad4-e43b-44bd-860e-99e617ad9f6d"))},
							},
						}

						return result, nil
					},

					fnRenewOrderWithExpPaidTime: func(ctx context.Context, id uuid.UUID, expt, paidt time.Time) error {
						if !expt.Equal(time.Date(2024, time.August, 1, 0, 0, 0, 0, time.UTC)) {
							return model.Error("unexpected_expt")
						}

						if !paidt.Equal(time.Date(2024, time.July, 1, 0, 0, 1, 0, time.UTC)) {
							return model.Error("unexpected_paidt")
						}

						return nil
					},

					fnAppendOrderMetadata: func(ctx context.Context, oid uuid.UUID, mdata datastore.Metadata) error {
						if mdata["externalID"] != "blob" {
							return model.Error("unexpected_externalID")
						}

						if mdata["paymentProcessor"] != "android" {
							return model.Error("unexpected_paymentProcessor")
						}

						if mdata["vendor"] != "android" {
							return model.Error("unexpected_vendor")
						}

						return nil
					},
				},
				set:   newOrderItemReqNewMobileSet("development"),
				ppcfg: newPaymentProcessorConfig("development"),
				rcpt: model.ReceiptData{
					Type:      model.VendorGoogle,
					ProductID: "brave.vpn.monthly",
					ExtID:     "blob",
					ExpiresAt: time.Date(2024, time.August, 1, 0, 0, 0, 0, time.UTC),
				},
				paidt: time.Date(2024, time.July, 1, 0, 0, 1, 0, time.UTC),
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
			actual, err := createOrderWithReceipt(context.Background(), tc.given.svc, tc.given.set, tc.given.ppcfg, tc.given.rcpt, tc.given.paidt)
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

func TestService_doTLV2ExistTxTime(t *testing.T) {
	type tcGiven struct {
		reqID      uuid.UUID
		item       *model.OrderItem
		firstBCred string
		from       time.Time
		to         time.Time
		repo       *repository.MockTLV2
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   error
	}

	tests := []testCase{
		{
			name: "invalid_credential_type",
			given: tcGiven{
				reqID: uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
				item: &model.OrderItem{
					ID:             uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
					OrderID:        uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
					CredentialType: "time-limited",
				},
				firstBCred: "cred_01",
				from:       time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				to:         time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				repo:       &repository.MockTLV2{},
			},
			exp: model.ErrUnsupportedCredType,
		},

		{
			name: "submission_report_error",
			given: tcGiven{
				reqID: uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
				item: &model.OrderItem{
					ID:             uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
					OrderID:        uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
					CredentialType: "time-limited-v2",
				},
				firstBCred: "cred_01",
				from:       time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				to:         time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				repo: &repository.MockTLV2{
					FnGetCredSubmissionReport: func(ctx context.Context, dbi sqlx.QueryerContext, orderID, itemID, reqID uuid.UUID, firstBCred string) (model.TLV2CredSubmissionReport, error) {
						return model.TLV2CredSubmissionReport{}, model.Error("something_went_wrong")
					},
				},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "submission_submitted",
			given: tcGiven{
				reqID: uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
				item: &model.OrderItem{
					ID:             uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
					OrderID:        uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
					CredentialType: "time-limited-v2",
				},
				firstBCred: "cred_01",
				from:       time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				to:         time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				repo: &repository.MockTLV2{
					FnGetCredSubmissionReport: func(ctx context.Context, dbi sqlx.QueryerContext, orderID, itemID, reqID uuid.UUID, firstBCred string) (model.TLV2CredSubmissionReport, error) {
						return model.TLV2CredSubmissionReport{Submitted: true}, nil
					},
				},
			},
			exp: errCredsAlreadySubmitted,
		},

		{
			name: "submission_mismatch",
			given: tcGiven{
				reqID: uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
				item: &model.OrderItem{
					ID:             uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
					OrderID:        uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
					CredentialType: "time-limited-v2",
				},
				firstBCred: "cred_01",
				from:       time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				to:         time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				repo: &repository.MockTLV2{
					FnGetCredSubmissionReport: func(ctx context.Context, dbi sqlx.QueryerContext, orderID, itemID, reqID uuid.UUID, firstBCred string) (model.TLV2CredSubmissionReport, error) {
						return model.TLV2CredSubmissionReport{ReqIDMismatch: true}, nil
					},
				},
			},
			exp: errCredsAlreadySubmittedMismatch,
		},

		{
			name: "uniq_batches_error",
			given: tcGiven{
				reqID: uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
				item: &model.OrderItem{
					ID:             uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
					OrderID:        uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
					CredentialType: "time-limited-v2",
				},
				firstBCred: "cred_01",
				from:       time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				to:         time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				repo: &repository.MockTLV2{
					FnUniqBatches: func(ctx context.Context, dbi sqlx.QueryerContext, orderID, itemID uuid.UUID, from, to time.Time) (int, error) {
						return 0, model.Error("something_went_wrong")
					},
				},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "uniq_batches_over_limit",
			given: tcGiven{
				reqID: uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
				item: &model.OrderItem{
					ID:             uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
					OrderID:        uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
					CredentialType: "time-limited-v2",
				},
				firstBCred: "cred_01",
				from:       time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				to:         time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				repo: &repository.MockTLV2{
					FnUniqBatches: func(ctx context.Context, dbi sqlx.QueryerContext, orderID, itemID uuid.UUID, from, to time.Time) (int, error) {
						return 10, nil
					},
				},
			},
			exp: ErrCredsAlreadyExist,
		},

		{
			name: "success",
			given: tcGiven{
				reqID: uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
				item: &model.OrderItem{
					ID:             uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
					OrderID:        uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
					CredentialType: "time-limited-v2",
				},
				firstBCred: "cred_01",
				from:       time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				to:         time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				repo:       &repository.MockTLV2{},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{tlv2Repo: tc.given.repo}

			ctx := context.Background()

			actual := svc.doTLV2ExistTxTime(ctx, nil, tc.given.reqID, tc.given.item, tc.given.firstBCred, tc.given.from, tc.given.to)
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestIsErrStripeNotFound(t *testing.T) {
	tests := []struct {
		name  string
		given error
		exp   bool
	}{
		{
			name:  "something_else",
			given: model.Error("something else"),
		},

		{
			name: "429_rate_limit",
			given: &stripe.Error{
				HTTPStatusCode: http.StatusTooManyRequests,
				Code:           stripe.ErrorCodeRateLimit,
			},
		},

		{
			name: "429_resource_missing",
			given: &stripe.Error{
				HTTPStatusCode: http.StatusTooManyRequests,
				Code:           stripe.ErrorCodeResourceMissing,
			},
		},

		{
			name: "404_rate_limit",
			given: &stripe.Error{
				HTTPStatusCode: http.StatusNotFound,
				Code:           stripe.ErrorCodeRateLimit,
			},
		},

		{
			name: "404_resource_missing",
			given: &stripe.Error{
				HTTPStatusCode: http.StatusNotFound,
				Code:           stripe.ErrorCodeResourceMissing,
			},
			exp: true,
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := isErrStripeNotFound(tc.given)

			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestReceiptValidError(t *testing.T) {
	tests := []struct {
		name  string
		given error
		exp   error
	}{
		{
			name:  "wrapped",
			given: &receiptValidError{err: model.Error("some_error")},
			exp:   model.Error("some_error"),
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			if rverr := new(receiptValidError); errors.As(tc.given, &rverr) {
				should.Equal(t, tc.exp, rverr.err)

				return
			}

			should.Fail(t, "unexpected")
		})
	}
}

func TestService_appendOrderMetadataTx(t *testing.T) {
	type tcGiven struct {
		oid   uuid.UUID
		mdata datastore.Metadata
		repo  orderStoreSvc
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   error
	}

	tests := []testCase{
		{
			name: "error_append_metadata",
			given: tcGiven{
				oid: uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
				mdata: datastore.Metadata{
					"string": "value",
				},
				repo: &repository.MockOrder{
					FnAppendMetadata: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error {
						return model.Error("something_went_wrong")
					},
				},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "error_append_metadata_int",
			given: tcGiven{
				oid: uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
				mdata: datastore.Metadata{
					"int": int(42),
				},
				repo: &repository.MockOrder{
					FnAppendMetadataInt: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key string, val int) error {
						return model.Error("something_went_wrong")
					},
				},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "error_append_metadata_int64",
			given: tcGiven{
				oid: uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
				mdata: datastore.Metadata{
					"int": int64(42),
				},
				repo: &repository.MockOrder{
					FnAppendMetadataInt64: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key string, val int64) error {
						return model.Error("something_went_wrong")
					},
				},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "error_append_metadata_float64",
			given: tcGiven{
				oid: uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
				mdata: datastore.Metadata{
					"float64": float64(42),
				},
				repo: &repository.MockOrder{
					FnAppendMetadataInt: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key string, val int) error {
						return model.Error("something_went_wrong")
					},
				},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "error_invalid_type",
			given: tcGiven{
				oid: uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
				mdata: datastore.Metadata{
					"bool": false,
				},
				repo: &repository.MockOrder{},
			},
			exp: model.ErrInvalidOrderMetadataType,
		},

		{
			name: "success",
			given: tcGiven{
				oid: uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
				mdata: datastore.Metadata{
					"string":  "value",
					"int":     int(42),
					"int64":   int64(42),
					"float64": float64(42),
				},
				repo: &repository.MockOrder{
					FnAppendMetadata: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error {
						if key != "string" {
							return model.Error("unexpected")
						}

						if val != "value" {
							return model.Error("unexpected")
						}

						return nil
					},

					FnAppendMetadataInt: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key string, val int) error {
						if key != "int" && key != "float64" {
							return model.Error("unexpected")
						}

						if val != 42 {
							return model.Error("unexpected")
						}

						return nil
					},

					FnAppendMetadataInt64: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key string, val int64) error {
						if key != "int64" {
							return model.Error("unexpected")
						}

						if val != 42 {
							return model.Error("unexpected")
						}

						return nil
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{orderRepo: tc.given.repo}

			ctx := context.Background()

			actual := svc.appendOrderMetadataTx(ctx, nil, tc.given.oid, tc.given.mdata)
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestService_processStripeNotificationTx(t *testing.T) {
	type tcGiven struct {
		ntf     *stripeNotification
		ordRepo orderStoreSvc
		phRepo  orderPayHistoryStore
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   error
	}

	tests := []testCase{
		{
			name: "skip",
			given: tcGiven{
				ntf: &stripeNotification{
					raw:     &stripe.Event{Type: "invoice.updated"},
					invoice: &stripe.Invoice{},
				},
				ordRepo: &repository.MockOrder{},
				phRepo:  &repository.MockOrderPayHistory{},
			},
		},

		{
			name: "renew_sub_id_error",
			given: tcGiven{
				ntf: &stripeNotification{
					raw:     &stripe.Event{Type: "invoice.paid"},
					invoice: &stripe.Invoice{},
				},
				ordRepo: &repository.MockOrder{},
				phRepo:  &repository.MockOrderPayHistory{},
			},
			exp: errStripeNoInvoiceSub,
		},

		{
			name: "renew_order_id_error_no_lines",
			given: tcGiven{
				ntf: &stripeNotification{
					raw: &stripe.Event{Type: "invoice.paid"},
					invoice: &stripe.Invoice{
						Subscription: &stripe.Subscription{ID: "sub_id"},
						Lines:        &stripe.InvoiceLineList{},
					},
				},
				ordRepo: &repository.MockOrder{},
				phRepo:  &repository.MockOrderPayHistory{},
			},
			exp: errStripeNoInvoiceLines,
		},

		{
			name: "renew_get_order_error",
			given: tcGiven{
				ntf: &stripeNotification{
					raw: &stripe.Event{Type: "invoice.paid"},
					invoice: &stripe.Invoice{
						Subscription: &stripe.Subscription{ID: "sub_id"},
						Lines: &stripe.InvoiceLineList{
							Data: []*stripe.InvoiceLine{
								{
									Metadata: map[string]string{
										"orderID": "facade00-0000-4000-a000-000000000000",
									},
								},
							},
						},
					},
				},
				ordRepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						return nil, model.Error("something_went_wrong")
					},
				},
				phRepo: &repository.MockOrderPayHistory{},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "renew_expires_time_error",
			given: tcGiven{
				ntf: &stripeNotification{
					raw: &stripe.Event{Type: "invoice.paid"},
					invoice: &stripe.Invoice{
						Subscription: &stripe.Subscription{ID: "sub_id"},
						Lines: &stripe.InvoiceLineList{
							Data: []*stripe.InvoiceLine{
								{
									Metadata: map[string]string{
										"orderID": "facade00-0000-4000-a000-000000000000",
									},
								},
							},
						},
					},
				},
				ordRepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID:       uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
							Metadata: datastore.Metadata{},
						}

						return result, nil
					},
				},
				phRepo: &repository.MockOrderPayHistory{},
			},
			exp: errStripeInvalidSubPeriod,
		},

		{
			name: "renew_should_update_sub_id_error",
			given: tcGiven{
				ntf: &stripeNotification{
					raw: &stripe.Event{Type: "invoice.paid"},
					invoice: &stripe.Invoice{
						Subscription: &stripe.Subscription{ID: "sub_id"},
						Lines: &stripe.InvoiceLineList{
							Data: []*stripe.InvoiceLine{
								{
									Metadata: map[string]string{
										"orderID": "facade00-0000-4000-a000-000000000000",
									},
									Period: &stripe.Period{
										Start: 1719792001,
										End:   1722470400,
									},
								},
							},
						},
					},
				},
				ordRepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
							Metadata: datastore.Metadata{
								"stripeSubscriptionId": "wrong_sub_id",
							},
						}

						return result, nil
					},
					FnAppendMetadata: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error {
						return model.Error("something_went_wrong")
					},
				},
				phRepo: &repository.MockOrderPayHistory{},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "renew_renew_error",
			given: tcGiven{
				ntf: &stripeNotification{
					raw: &stripe.Event{Type: "invoice.paid"},
					invoice: &stripe.Invoice{
						Subscription: &stripe.Subscription{ID: "sub_id"},
						Lines: &stripe.InvoiceLineList{
							Data: []*stripe.InvoiceLine{
								{
									Metadata: map[string]string{
										"orderID": "facade00-0000-4000-a000-000000000000",
									},
									Period: &stripe.Period{
										Start: 1719792001,
										End:   1722470400,
									},
								},
							},
						},
					},
				},
				ordRepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID:       uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
							Metadata: datastore.Metadata{},
						}

						return result, nil
					},

					FnSetStatus: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, status string) error {
						return model.Error("something_went_wrong")
					},
				},
				phRepo: &repository.MockOrderPayHistory{},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "renew_update_payment_proc_error",
			given: tcGiven{
				ntf: &stripeNotification{
					raw: &stripe.Event{Type: "invoice.paid"},
					invoice: &stripe.Invoice{
						Subscription: &stripe.Subscription{ID: "sub_id"},
						Lines: &stripe.InvoiceLineList{
							Data: []*stripe.InvoiceLine{
								{
									Metadata: map[string]string{
										"orderID": "facade00-0000-4000-a000-000000000000",
									},
									Period: &stripe.Period{
										Start: 1719792001,
										End:   1722470400,
									},
								},
							},
						},
					},
				},
				ordRepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID:       uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
							Metadata: datastore.Metadata{},
						}

						return result, nil
					},

					FnAppendMetadata: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error {
						if key == "paymentProcessor" && val == model.StripePaymentMethod {
							return model.Error("something_went_wrong")
						}

						return nil
					},
				},
				phRepo: &repository.MockOrderPayHistory{},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "renew_success",
			given: tcGiven{
				ntf: &stripeNotification{
					raw: &stripe.Event{Type: "invoice.paid"},
					invoice: &stripe.Invoice{
						Subscription: &stripe.Subscription{ID: "sub_id"},
						Lines: &stripe.InvoiceLineList{
							Data: []*stripe.InvoiceLine{
								{
									Metadata: map[string]string{
										"orderID": "facade00-0000-4000-a000-000000000000",
									},
									Period: &stripe.Period{
										Start: 1719792001,
										End:   1722470400,
									},
								},
							},
						},
					},
				},
				ordRepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID:       uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
							Metadata: datastore.Metadata{},
						}

						return result, nil
					},

					FnAppendMetadata: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error {
						if key == "stripeSubscriptionId" && val == "sub_id" {
							return nil
						}

						if key == "paymentProcessor" && val == model.StripePaymentMethod {
							return nil
						}

						return model.Error("unexpected_metadata_string")
					},

					FnAppendMetadataInt: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key string, val int) error {
						if key == "numPaymentFailed" && val == 0 {
							return nil
						}

						return model.Error("unexpected_metadata_int")
					},

					FnSetStatus: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, status string) error {
						if uuid.Equal(id, uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000"))) && status == model.OrderStatusPaid {
							return nil
						}

						return model.Error("unexpected_status")
					},

					FnSetExpiresAt: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
						if uuid.Equal(id, uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000"))) && when.Equal(time.Unix(1722470400, 0).UTC().Add(24*time.Hour)) {
							return nil
						}

						return model.Error("unexpected_expt")
					},
				},
				phRepo: &repository.MockOrderPayHistory{},
			},
		},

		{
			name: "cancel_order_id_error",
			given: tcGiven{
				ntf: &stripeNotification{
					raw: &stripe.Event{Type: "customer.subscription.deleted"},
					sub: &stripe.Subscription{
						ID: "sub_id",
					},
				},
				ordRepo: &repository.MockOrder{},
				phRepo:  &repository.MockOrderPayHistory{},
			},
			exp: errStripeOrderIDMissing,
		},

		{
			name: "cancel_update_status_error",
			given: tcGiven{
				ntf: &stripeNotification{
					raw: &stripe.Event{Type: "customer.subscription.deleted"},
					sub: &stripe.Subscription{
						ID: "sub_id",
						Metadata: map[string]string{
							"orderID": "facade00-0000-4000-a000-000000000000",
						},
					},
				},
				ordRepo: &repository.MockOrder{
					FnSetStatus: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, status string) error {
						return model.Error("something_went_wrong")
					},
				},
				phRepo: &repository.MockOrderPayHistory{},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "cancel_update_num_payment_failed_error",
			given: tcGiven{
				ntf: &stripeNotification{
					raw: &stripe.Event{Type: "customer.subscription.deleted"},
					sub: &stripe.Subscription{
						ID: "sub_id",
						Metadata: map[string]string{
							"orderID": "facade00-0000-4000-a000-000000000000",
						},
					},
				},
				ordRepo: &repository.MockOrder{
					FnAppendMetadataInt: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key string, val int) error {
						return model.Error("something_went_wrong")
					},
				},
				phRepo: &repository.MockOrderPayHistory{},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "cancel_success",
			given: tcGiven{
				ntf: &stripeNotification{
					raw: &stripe.Event{Type: "customer.subscription.deleted"},
					sub: &stripe.Subscription{
						ID: "sub_id",
						Metadata: map[string]string{
							"orderID": "facade00-0000-4000-a000-000000000000",
						},
					},
				},
				ordRepo: &repository.MockOrder{
					FnAppendMetadataInt: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key string, val int) error {
						if uuid.Equal(id, uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000"))) && val == 0 {
							return nil
						}

						return model.Error("unexpected_metadata_int")
					},

					FnSetStatus: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, status string) error {
						if uuid.Equal(id, uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000"))) && status == model.OrderStatusCanceled {
							return nil
						}

						return model.Error("unexpected_status")
					},
				},
				phRepo: &repository.MockOrderPayHistory{},
			},
		},

		{
			name: "record_pay_failure_sub_id_error",
			given: tcGiven{
				ntf: &stripeNotification{
					raw:     &stripe.Event{Type: "invoice.payment_failed"},
					invoice: &stripe.Invoice{},
				},
				ordRepo: &repository.MockOrder{},
				phRepo:  &repository.MockOrderPayHistory{},
			},
			exp: errStripeNoInvoiceSub,
		},

		{
			name: "record_pay_failure_order_id_error_no_lines",
			given: tcGiven{
				ntf: &stripeNotification{
					raw: &stripe.Event{Type: "invoice.payment_failed"},
					invoice: &stripe.Invoice{
						Subscription: &stripe.Subscription{ID: "sub_id"},
						Lines:        &stripe.InvoiceLineList{},
					},
				},
				ordRepo: &repository.MockOrder{},
				phRepo:  &repository.MockOrderPayHistory{},
			},
			exp: errStripeNoInvoiceLines,
		},

		{
			name: "record_pay_failure_get_order_error",
			given: tcGiven{
				ntf: &stripeNotification{
					raw: &stripe.Event{Type: "invoice.payment_failed"},
					invoice: &stripe.Invoice{
						Subscription: &stripe.Subscription{ID: "sub_id"},
						Lines: &stripe.InvoiceLineList{
							Data: []*stripe.InvoiceLine{
								{
									Metadata: map[string]string{
										"orderID": "facade00-0000-4000-a000-000000000000",
									},
								},
							},
						},
					},
				},
				ordRepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						return nil, model.Error("something_went_wrong")
					},
				},
				phRepo: &repository.MockOrderPayHistory{},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "record_pay_failure_record_pay_failure_stripe_error",
			given: tcGiven{
				ntf: &stripeNotification{
					raw: &stripe.Event{Type: "invoice.payment_failed"},
					invoice: &stripe.Invoice{
						Subscription: &stripe.Subscription{ID: "sub_id"},
						Lines: &stripe.InvoiceLineList{
							Data: []*stripe.InvoiceLine{
								{
									Metadata: map[string]string{
										"orderID": "facade00-0000-4000-a000-000000000000",
									},
								},
							},
						},
					},
				},
				ordRepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
							Metadata: datastore.Metadata{
								"numPaymentFailed": int(1),
							},
						}

						return result, nil
					},

					FnIncrementNumPayFailed: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID) error {
						return model.Error("something_went_wrong")
					},
				},
				phRepo: &repository.MockOrderPayHistory{},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "record_pay_failure_success",
			given: tcGiven{
				ntf: &stripeNotification{
					raw: &stripe.Event{Type: "invoice.payment_failed"},
					invoice: &stripe.Invoice{
						Subscription: &stripe.Subscription{ID: "sub_id"},
						Lines: &stripe.InvoiceLineList{
							Data: []*stripe.InvoiceLine{
								{
									Metadata: map[string]string{
										"orderID": "facade00-0000-4000-a000-000000000000",
									},
								},
							},
						},
					},
				},
				ordRepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
							Metadata: datastore.Metadata{
								"numPaymentFailed": int(1),
							},
						}

						return result, nil
					},

					FnAppendMetadata: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error {
						if key == "stripeSubscriptionId" && val == "sub_id" {
							return nil
						}

						if key == "paymentProcessor" && val == model.StripePaymentMethod {
							return nil
						}

						return model.Error("unexpected_metadata_string")
					},
				},
				phRepo: &repository.MockOrderPayHistory{},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{orderRepo: tc.given.ordRepo, payHistRepo: tc.given.phRepo}

			ctx := context.Background()

			actual := svc.processStripeNotificationTx(ctx, nil, tc.given.ntf)
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestShouldUpdateOrderStripeSubID(t *testing.T) {
	type tcGiven struct {
		ord   *model.Order
		subID string
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   bool
	}

	tests := []testCase{
		{
			name: "true_sub_id_not_set",
			given: tcGiven{
				ord:   &model.Order{},
				subID: "sub_id",
			},
			exp: true,
		},

		{
			name: "true_sub_id_empty",
			given: tcGiven{
				ord: &model.Order{
					Metadata: datastore.Metadata{
						"stripeSubscriptionId": "",
					},
				},
				subID: "sub_id",
			},
			exp: true,
		},

		{
			name: "true_sub_id_different",
			given: tcGiven{
				ord: &model.Order{
					Metadata: datastore.Metadata{
						"stripeSubscriptionId": "old_sub_id",
					},
				},
				subID: "sub_id",
			},
			exp: true,
		},

		{
			name: "false_sub_id_same",
			given: tcGiven{
				ord: &model.Order{
					Metadata: datastore.Metadata{
						"stripeSubscriptionId": "sub_id",
					},
				},
				subID: "sub_id",
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := shouldUpdateOrderStripeSubID(tc.given.ord, tc.given.subID)
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestShouldTransformStripeOrder(t *testing.T) {
	type testCase struct {
		name  string
		given *model.Order
		exp   bool
	}

	tests := []testCase{
		{
			name: "false_ios",
			given: &model.Order{
				Metadata: datastore.Metadata{
					"paymentProcessor": "ios",
					"vendor":           "ios",
				},
			},
		},

		{
			name: "false_android",
			given: &model.Order{
				Metadata: datastore.Metadata{
					"paymentProcessor": "android",
					"vendor":           "android",
				},
			},
		},

		{
			name:  "false_paid",
			given: &model.Order{Status: model.OrderStatusPaid},
		},

		{
			name:  "false_non_stripe",
			given: &model.Order{Status: model.OrderStatusPending},
		},

		{
			name: "true_unpaid_stripe",
			given: &model.Order{
				Status:                model.OrderStatusPending,
				AllowedPaymentMethods: pq.StringArray([]string{"stripe"}),
			},
			exp: true,
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := shouldTransformStripeOrder(tc.given)
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestChooseStripeSessID(t *testing.T) {
	type tcGiven struct {
		ord       *model.Order
		newSessID string
	}

	type tcExpected struct {
		val string
		ok  bool
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "new_sess_id_no_old_sess_id",
			given: tcGiven{
				ord:       &model.Order{},
				newSessID: "new_sess_id",
			},
			exp: tcExpected{
				val: "new_sess_id",
				ok:  true,
			},
		},

		{
			name: "new_sess_id_old_sess_id",
			given: tcGiven{
				ord: &model.Order{
					Metadata: datastore.Metadata{
						"stripeCheckoutSessionId": "sess_id",
					},
				},
				newSessID: "new_sess_id",
			},
			exp: tcExpected{
				val: "new_sess_id",
				ok:  true,
			},
		},

		{
			name: "no_new_sess_id_no_old_sess_id",
			given: tcGiven{
				ord: &model.Order{},
			},
			exp: tcExpected{},
		},

		{
			name: "new_sess_id_old_sess_id",
			given: tcGiven{
				ord: &model.Order{
					Metadata: datastore.Metadata{
						"stripeCheckoutSessionId": "sess_id",
					},
				},
			},
			exp: tcExpected{
				val: "sess_id",
				ok:  true,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, ok := chooseStripeSessID(tc.given.ord, tc.given.newSessID)
			should.Equal(t, tc.exp.ok, ok)

			should.Equal(t, tc.exp.val, actual)
		})
	}
}

func TestService_getTransformOrderTx(t *testing.T) {
	type tcGiven struct {
		ordRepo  *repository.MockOrder
		itemRepo *repository.MockOrderItem
		payRepo  *repository.MockOrderPayHistory
		cl       *xstripe.MockClient
		id       uuid.UUID
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
			name: "error_first_get_order_full",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						return nil, model.Error("something_went_wrong")
					},
				},
				itemRepo: &repository.MockOrderItem{},
				payRepo:  &repository.MockOrderPayHistory{},
				cl:       &xstripe.MockClient{},
				id:       uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
			},
			exp: tcExpected{
				err: model.Error("something_went_wrong"),
			},
		},

		{
			name: "skip_has_sub_id",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
							Metadata: datastore.Metadata{
								"stripeSubscriptionId": "sub_id",
							},
						}

						return result, nil
					},
				},
				itemRepo: &repository.MockOrderItem{
					FnFindByOrderID: func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) ([]model.OrderItem, error) {
						result := model.OrderItem{
							ID:      uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
							OrderID: uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
						}

						return []model.OrderItem{result}, nil
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				cl:      &xstripe.MockClient{},
				id:      uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
			},
			exp: tcExpected{
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					Metadata: datastore.Metadata{
						"stripeSubscriptionId": "sub_id",
					},
					Items: []model.OrderItem{
						{
							ID:      uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
							OrderID: uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
						},
					},
				},
			},
		},

		{
			name: "skip_no_transform",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID:                    uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
							Status:                model.OrderStatusPaid,
							AllowedPaymentMethods: pq.StringArray{"stripe"},
						}

						return result, nil
					},
				},
				itemRepo: &repository.MockOrderItem{
					FnFindByOrderID: func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) ([]model.OrderItem, error) {
						result := model.OrderItem{
							ID:      uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
							OrderID: uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
						}

						return []model.OrderItem{result}, nil
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				cl:      &xstripe.MockClient{},
				id:      uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
			},
			exp: tcExpected{
				ord: &model.Order{
					ID:                    uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					Status:                model.OrderStatusPaid,
					AllowedPaymentMethods: pq.StringArray{"stripe"},
					Items: []model.OrderItem{
						{
							ID:      uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
							OrderID: uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
						},
					},
				},
			},
		},

		{
			name: "error_update_stripe_session_failed",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID:                    uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
							AllowedPaymentMethods: pq.StringArray{"stripe"},
						}

						return result, nil
					},
					FnGetExpiredStripeCheckoutSessionID: func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) (string, error) {
						return "", model.Error("something_went_wrong")
					},
				},
				itemRepo: &repository.MockOrderItem{
					FnFindByOrderID: func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) ([]model.OrderItem, error) {
						result := model.OrderItem{
							ID:      uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
							OrderID: uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
						}

						return []model.OrderItem{result}, nil
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				cl:      &xstripe.MockClient{},
				id:      uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
			},
			exp: tcExpected{
				err: model.Error("something_went_wrong"),
			},
		},

		{
			name: "success_after_update_order_session",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
							Metadata: datastore.Metadata{
								"stripeCheckoutSessionId": "cs_test_id",
							},
							AllowedPaymentMethods: pq.StringArray{"stripe"},
						}

						return result, nil
					},

					FnGetExpiredStripeCheckoutSessionID: func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) (string, error) {
						return "", nil
					},
				},
				itemRepo: &repository.MockOrderItem{
					FnFindByOrderID: func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) ([]model.OrderItem, error) {
						result := model.OrderItem{
							ID:      uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
							OrderID: uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
						}

						return []model.OrderItem{result}, nil
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				cl:      &xstripe.MockClient{},
				id:      uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
			},
			exp: tcExpected{
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					Metadata: datastore.Metadata{
						"stripeCheckoutSessionId": "cs_test_id",
					},
					AllowedPaymentMethods: pq.StringArray{"stripe"},
					Items: []model.OrderItem{
						{
							ID:      uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
							OrderID: uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
						},
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{
				orderRepo:     tc.given.ordRepo,
				orderItemRepo: tc.given.itemRepo,
				payHistRepo:   tc.given.payRepo,
				stripeCl:      tc.given.cl,
			}

			ctx := context.Background()

			actual, err := svc.getTransformOrderTx(ctx, nil, tc.given.id)
			must.Equal(t, true, errors.Is(err, tc.exp.err))

			if tc.exp.err != nil {
				return
			}

			should.Equal(t, tc.exp.ord, actual)
		})
	}
}

func TestService_updateOrderStripeSession(t *testing.T) {
	type tcGiven struct {
		ordRepo *repository.MockOrder
		payRepo *repository.MockOrderPayHistory
		cl      *xstripe.MockClient
		ord     *model.Order
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   error
	}

	tests := []testCase{
		{
			name: "get_exp_session_error",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnGetExpiredStripeCheckoutSessionID: func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) (string, error) {
						return "", model.Error("something_went_wrong")
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				cl:      &xstripe.MockClient{},
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
				},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "recreate_session_error",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnGetExpiredStripeCheckoutSessionID: func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) (string, error) {
						return "cs_test_id_old", nil
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				cl: &xstripe.MockClient{
					FnSession: func(ctx context.Context, id string, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						if id == "cs_test_id_old" {
							return nil, model.Error("something_went_wrong")
						}

						return nil, model.Error("unexpected")
					},
				},
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					Metadata: datastore.Metadata{
						"stripeCheckoutSessionId": "cs_test_id_old",
					},
				},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "return_early_no_new_sess_id_no_old_sess_id",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnGetExpiredStripeCheckoutSessionID: func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) (string, error) {
						return "", model.ErrExpiredStripeCheckoutSessionIDNotFound
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				cl: &xstripe.MockClient{
					FnSession: func(ctx context.Context, id string, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						return nil, model.Error("unexpected")
					},
				},
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
				},
			},
		},

		{
			name: "return_early_no_new_sess_id_empty_old_sess_id",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnGetExpiredStripeCheckoutSessionID: func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) (string, error) {
						return "", nil
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				cl: &xstripe.MockClient{
					FnSession: func(ctx context.Context, id string, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						return nil, model.Error("unexpected")
					},
				},
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					Metadata: datastore.Metadata{
						"stripeCheckoutSessionId": "",
					},
				},
			},
		},

		{
			name: "error_fetch_new_session",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnGetExpiredStripeCheckoutSessionID: func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) (string, error) {
						return "cs_test_id_old", nil
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				cl: &xstripe.MockClient{
					FnSession: func(ctx context.Context, id string, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						if id == "cs_test_id_old" {
							result := &stripe.CheckoutSession{
								ID:         id,
								Customer:   &stripe.Customer{Email: "you@example.com"},
								SuccessURL: "https://example.com/success",
								CancelURL:  "https://example.com/cancel",
							}

							return result, nil
						}

						return nil, model.Error("something_went_wrong")
					},
				},
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					Metadata: datastore.Metadata{
						"stripeCheckoutSessionId": "cs_test_id_old",
					},
				},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "error_fetch_existing_session",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnGetExpiredStripeCheckoutSessionID: func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) (string, error) {
						return "", model.ErrExpiredStripeCheckoutSessionIDNotFound
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				cl: &xstripe.MockClient{
					FnSession: func(ctx context.Context, id string, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						if id == "cs_test_id_existing" {
							return nil, model.Error("something_went_wrong")
						}

						return nil, model.Error("unexpected")
					},
				},
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					Metadata: datastore.Metadata{
						"stripeCheckoutSessionId": "cs_test_id_existing",
					},
				},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "skip_unpaid_session",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnGetExpiredStripeCheckoutSessionID: func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) (string, error) {
						return "cs_test_id_old", nil
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				cl: &xstripe.MockClient{
					FnSession: func(ctx context.Context, id string, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						if id == "cs_test_id_old" {
							result := &stripe.CheckoutSession{
								ID:         id,
								Customer:   &stripe.Customer{Email: "you@example.com"},
								SuccessURL: "https://example.com/success",
								CancelURL:  "https://example.com/cancel",
							}

							return result, nil
						}

						if id == "cs_test_id" {
							result := &stripe.CheckoutSession{
								ID:                 "cs_test_id",
								PaymentMethodTypes: []string{"card"},
								Mode:               stripe.CheckoutSessionModeSubscription,
								SuccessURL:         "https://example.com/success",
								CancelURL:          "https://example.com/cancel",
								ClientReferenceID:  "facade00-0000-4000-a000-000000000000",
								Subscription: &stripe.Subscription{
									ID: "sub_id",
									Metadata: map[string]string{
										"orderID": "facade00-0000-4000-a000-000000000000",
									},
								},
								AllowPromotionCodes: true,
							}

							return result, nil
						}

						return nil, model.Error("unexpected")
					},
				},
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					Metadata: datastore.Metadata{
						"stripeCheckoutSessionId": "cs_test_id_old",
					},
				},
			},
		},

		{
			name: "error_handle_paid_fetch_sub",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnGetExpiredStripeCheckoutSessionID: func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) (string, error) {
						return "cs_test_id_old", nil
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				cl: &xstripe.MockClient{
					FnSession: func(ctx context.Context, id string, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						if id == "cs_test_id_old" {
							result := &stripe.CheckoutSession{
								ID:         id,
								Customer:   &stripe.Customer{Email: "you@example.com"},
								SuccessURL: "https://example.com/success",
								CancelURL:  "https://example.com/cancel",
							}

							return result, nil
						}

						if id == "cs_test_id" {
							result := &stripe.CheckoutSession{
								ID:                 "cs_test_id",
								PaymentStatus:      stripe.CheckoutSessionPaymentStatusPaid,
								PaymentMethodTypes: []string{"card"},
								Mode:               stripe.CheckoutSessionModeSubscription,
								SuccessURL:         "https://example.com/success",
								CancelURL:          "https://example.com/cancel",
								ClientReferenceID:  "facade00-0000-4000-a000-000000000000",
								Subscription: &stripe.Subscription{
									ID: "sub_id",
									Metadata: map[string]string{
										"orderID": "facade00-0000-4000-a000-000000000000",
									},
								},
								AllowPromotionCodes: true,
							}

							return result, nil
						}

						return nil, model.Error("unexpected")
					},
					FnSubscription: func(ctx context.Context, id string, params *stripe.SubscriptionParams) (*stripe.Subscription, error) {
						if id == "sub_id" {
							return nil, model.Error("something_went_wrong")
						}

						return nil, model.Error("unexpected")
					},
				},
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					Metadata: datastore.Metadata{
						"stripeCheckoutSessionId": "cs_test_id_old",
					},
				},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "success_handle_paid_new_session",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnGetExpiredStripeCheckoutSessionID: func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) (string, error) {
						return "cs_test_id_old", nil
					},

					FnSetExpiresAt: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
						if !when.Equal(time.Date(2024, time.August, 2, 0, 0, 0, 0, time.UTC)) {
							return model.Error("unexpected")
						}

						return nil
					},

					FnSetLastPaidAt: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
						if !when.Equal(time.Date(2024, time.July, 1, 0, 0, 0, 0, time.UTC)) {
							return model.Error("unexpected")
						}

						return nil
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				cl: &xstripe.MockClient{
					FnSession: func(ctx context.Context, id string, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						if id == "cs_test_id_old" {
							result := &stripe.CheckoutSession{
								ID:         id,
								Customer:   &stripe.Customer{Email: "you@example.com"},
								SuccessURL: "https://example.com/success",
								CancelURL:  "https://example.com/cancel",
							}

							return result, nil
						}

						if id == "cs_test_id" {
							result := &stripe.CheckoutSession{
								ID:                 "cs_test_id",
								PaymentStatus:      stripe.CheckoutSessionPaymentStatusPaid,
								PaymentMethodTypes: []string{"card"},
								Mode:               stripe.CheckoutSessionModeSubscription,
								SuccessURL:         "https://example.com/success",
								CancelURL:          "https://example.com/cancel",
								ClientReferenceID:  "facade00-0000-4000-a000-000000000000",
								Subscription: &stripe.Subscription{
									ID: "sub_id",
									Metadata: map[string]string{
										"orderID": "facade00-0000-4000-a000-000000000000",
									},
								},
								AllowPromotionCodes: true,
							}

							return result, nil
						}

						return nil, model.Error("unexpected")
					},
					FnSubscription: func(ctx context.Context, id string, params *stripe.SubscriptionParams) (*stripe.Subscription, error) {
						if id == "sub_id" {
							result := &stripe.Subscription{
								ID:                 id,
								CurrentPeriodEnd:   1722470400,
								CurrentPeriodStart: 1719792000,
							}

							return result, nil
						}

						return nil, model.Error("unexpected")
					},
				},
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					Metadata: datastore.Metadata{
						"stripeCheckoutSessionId": "cs_test_id_old",
					},
				},
			},
		},

		{
			name: "success_handle_paid_existing_session",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnGetExpiredStripeCheckoutSessionID: func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) (string, error) {
						return "", nil
					},

					FnSetExpiresAt: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
						if !when.Equal(time.Date(2024, time.August, 2, 0, 0, 0, 0, time.UTC)) {
							return model.Error("unexpected")
						}

						return nil
					},

					FnSetLastPaidAt: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
						if !when.Equal(time.Date(2024, time.July, 1, 0, 0, 0, 0, time.UTC)) {
							return model.Error("unexpected")
						}

						return nil
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				cl: &xstripe.MockClient{
					FnSession: func(ctx context.Context, id string, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						if id == "cs_test_id_existing" {
							result := &stripe.CheckoutSession{
								ID:                 "cs_test_id",
								PaymentStatus:      stripe.CheckoutSessionPaymentStatusPaid,
								PaymentMethodTypes: []string{"card"},
								Mode:               stripe.CheckoutSessionModeSubscription,
								SuccessURL:         "https://example.com/success",
								CancelURL:          "https://example.com/cancel",
								ClientReferenceID:  "facade00-0000-4000-a000-000000000000",
								Subscription: &stripe.Subscription{
									ID: "sub_id",
									Metadata: map[string]string{
										"orderID": "facade00-0000-4000-a000-000000000000",
									},
								},
								AllowPromotionCodes: true,
							}

							return result, nil
						}

						return nil, model.Error("unexpected")
					},
					FnSubscription: func(ctx context.Context, id string, params *stripe.SubscriptionParams) (*stripe.Subscription, error) {
						if id == "sub_id" {
							result := &stripe.Subscription{
								ID:                 id,
								CurrentPeriodEnd:   1722470400,
								CurrentPeriodStart: 1719792000,
							}

							return result, nil
						}

						return nil, model.Error("unexpected")
					},
				},
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					Metadata: datastore.Metadata{
						"stripeCheckoutSessionId": "cs_test_id_existing",
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{
				orderRepo:   tc.given.ordRepo,
				payHistRepo: tc.given.payRepo,
				stripeCl:    tc.given.cl,
			}

			ctx := context.Background()

			actual := svc.updateOrderStripeSession(ctx, nil, tc.given.ord)
			should.Equal(t, true, errors.Is(actual, tc.exp))
		})
	}
}

func TestService_renewOrderStripe(t *testing.T) {
	type tcGiven struct {
		ordRepo *repository.MockOrder
		payRepo *repository.MockOrderPayHistory
		ord     *model.Order
		subID   string
		expt    time.Time
		paidt   time.Time
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   error
	}

	tests := []testCase{
		{
			name: "error_should_update_sub_id",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnAppendMetadata: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error {
						return model.Error("something_went_wrong")
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				ord: &model.Order{
					Metadata: datastore.Metadata{
						"stripeSubscriptionId": "old_sub_id",
					},
				},
				subID: "sub_id",
				expt:  time.Date(2024, time.July, 1, 0, 0, 0, 0, time.UTC),
				paidt: time.Date(2024, time.June, 1, 0, 0, 1, 0, time.UTC),
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "error_renew_order",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnSetStatus: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, status string) error {
						return model.Error("something_went_wrong")
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				ord: &model.Order{
					Metadata: datastore.Metadata{
						"stripeSubscriptionId": "old_sub_id",
					},
				},
				subID: "sub_id",
				expt:  time.Date(2024, time.July, 1, 0, 0, 0, 0, time.UTC),
				paidt: time.Date(2024, time.June, 1, 0, 0, 1, 0, time.UTC),
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "error_save_payment_proc",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnAppendMetadata: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error {
						if key == "paymentProcessor" {
							return model.Error("something_went_wrong")
						}

						return nil
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				ord: &model.Order{
					Metadata: datastore.Metadata{
						"stripeSubscriptionId": "sub_id",
					},
				},
				subID: "sub_id",
				expt:  time.Date(2024, time.July, 1, 0, 0, 0, 0, time.UTC),
				paidt: time.Date(2024, time.June, 1, 0, 0, 1, 0, time.UTC),
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "success_should_update_sub_id_no_payment_proc_update",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnAppendMetadata: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error {
						if key == "stripeSubscriptionId" && val == "sub_id" {
							return nil
						}

						if key == "paymentProcessor" && val == "stripe" {
							return model.Error("unexpected")
						}

						return nil
					},

					FnSetExpiresAt: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
						if when.Equal(time.Date(2024, time.July, 1, 0, 0, 0, 0, time.UTC).Add(24 * time.Hour)) {
							return nil
						}

						return model.Error("unexpected")
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				ord: &model.Order{
					Metadata: datastore.Metadata{
						"stripeSubscriptionId": "old_sub_id",
						"paymentProcessor":     "stripe",
					},
				},
				subID: "sub_id",
				expt:  time.Date(2024, time.July, 1, 0, 0, 0, 0, time.UTC),
				paidt: time.Date(2024, time.June, 1, 0, 0, 1, 0, time.UTC),
			},
		},

		{
			name: "success_should_update_sub_id_should_update_payment_proc",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnAppendMetadata: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error {
						if key == "stripeSubscriptionId" && val == "sub_id" {
							return nil
						}

						if key == "paymentProcessor" && val == "stripe" {
							return nil
						}

						return model.Error("unexpected")
					},

					FnSetExpiresAt: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
						if when.Equal(time.Date(2024, time.July, 1, 0, 0, 0, 0, time.UTC).Add(24 * time.Hour)) {
							return nil
						}

						return model.Error("unexpected")
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				ord: &model.Order{
					Metadata: datastore.Metadata{
						"stripeSubscriptionId": "old_sub_id",
					},
				},
				subID: "sub_id",
				expt:  time.Date(2024, time.July, 1, 0, 0, 0, 0, time.UTC),
				paidt: time.Date(2024, time.June, 1, 0, 0, 1, 0, time.UTC),
			},
		},

		{
			name: "success_no_update_sub_id_should_update_payment_proc",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnAppendMetadata: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error {
						if key == "stripeSubscriptionId" {
							return model.Error("unexpected")
						}

						if key == "paymentProcessor" && val == "stripe" {
							return nil
						}

						return nil
					},

					FnSetExpiresAt: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
						if when.Equal(time.Date(2024, time.July, 1, 0, 0, 0, 0, time.UTC).Add(24 * time.Hour)) {
							return nil
						}

						return model.Error("unexpected")
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				ord: &model.Order{
					Metadata: datastore.Metadata{
						"stripeSubscriptionId": "sub_id",
					},
				},
				subID: "sub_id",
				expt:  time.Date(2024, time.July, 1, 0, 0, 0, 0, time.UTC),
				paidt: time.Date(2024, time.June, 1, 0, 0, 1, 0, time.UTC),
			},
		},

		{
			name: "success_no_update_sub_id_no_update_payment_proc",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnAppendMetadata: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error {
						return model.Error("unexpected")
					},

					FnSetExpiresAt: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
						if when.Equal(time.Date(2024, time.July, 1, 0, 0, 0, 0, time.UTC).Add(24 * time.Hour)) {
							return nil
						}

						return model.Error("unexpected")
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				ord: &model.Order{
					Metadata: datastore.Metadata{
						"stripeSubscriptionId": "sub_id",
						"paymentProcessor":     "stripe",
					},
				},
				subID: "sub_id",
				expt:  time.Date(2024, time.July, 1, 0, 0, 0, 0, time.UTC),
				paidt: time.Date(2024, time.June, 1, 0, 0, 1, 0, time.UTC),
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{orderRepo: tc.given.ordRepo, payHistRepo: tc.given.payRepo}

			ctx := context.Background()

			actual := svc.renewOrderStripe(ctx, nil, tc.given.ord, tc.given.subID, tc.given.expt, tc.given.paidt)
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestService_recordPayFailureStripe(t *testing.T) {
	type tcGiven struct {
		ordRepo *repository.MockOrder
		payRepo *repository.MockOrderPayHistory
		ord     *model.Order
		subID   string
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   error
	}

	tests := []testCase{
		{
			name: "error_should_update_sub_id",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnAppendMetadata: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error {
						return model.Error("something_went_wrong")
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				ord: &model.Order{
					Metadata: datastore.Metadata{
						"stripeSubscriptionId": "old_sub_id",
					},
				},
				subID: "sub_id",
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "error_increment_num_payment_failed",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnIncrementNumPayFailed: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID) error {
						return model.Error("something_went_wrong")
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				ord: &model.Order{
					Metadata: datastore.Metadata{
						"stripeSubscriptionId": "old_sub_id",
					},
				},
				subID: "sub_id",
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "error_save_payment_proc",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnAppendMetadata: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error {
						if key == "paymentProcessor" {
							return model.Error("something_went_wrong")
						}

						return nil
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				ord: &model.Order{
					Metadata: datastore.Metadata{
						"stripeSubscriptionId": "sub_id",
					},
				},
				subID: "sub_id",
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "success_should_update_sub_id_no_payment_proc_update",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnAppendMetadata: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error {
						if key == "stripeSubscriptionId" && val == "sub_id" {
							return nil
						}

						if key == "paymentProcessor" && val == "stripe" {
							return model.Error("unexpected")
						}

						return nil
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				ord: &model.Order{
					Metadata: datastore.Metadata{
						"stripeSubscriptionId": "old_sub_id",
						"paymentProcessor":     "stripe",
					},
				},
				subID: "sub_id",
			},
		},

		{
			name: "success_should_update_sub_id_should_update_payment_proc",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnAppendMetadata: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error {
						if key == "stripeSubscriptionId" && val == "sub_id" {
							return nil
						}

						if key == "paymentProcessor" && val == "stripe" {
							return nil
						}

						return model.Error("unexpected_metadata_string")
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				ord: &model.Order{
					Metadata: datastore.Metadata{
						"stripeSubscriptionId": "old_sub_id",
					},
				},
				subID: "sub_id",
			},
		},

		{
			name: "success_no_update_sub_id_should_update_payment_proc",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnAppendMetadata: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error {
						if key == "stripeSubscriptionId" {
							return model.Error("unexpected")
						}

						if key == "paymentProcessor" && val == "stripe" {
							return nil
						}

						return nil
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				ord: &model.Order{
					Metadata: datastore.Metadata{
						"stripeSubscriptionId": "sub_id",
					},
				},
				subID: "sub_id",
			},
		},

		{
			name: "success_no_update_sub_id_no_update_payment_proc",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnAppendMetadata: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error {
						return model.Error("unexpected")
					},
				},
				payRepo: &repository.MockOrderPayHistory{},
				ord: &model.Order{
					Metadata: datastore.Metadata{
						"stripeSubscriptionId": "sub_id",
						"paymentProcessor":     "stripe",
					},
				},
				subID: "sub_id",
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{orderRepo: tc.given.ordRepo, payHistRepo: tc.given.payRepo}

			ctx := context.Background()

			actual := svc.recordPayFailureStripe(ctx, nil, tc.given.ord, tc.given.subID)
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestService_createStripeSession(t *testing.T) {
	type tcGiven struct {
		cl  *xstripe.MockClient
		req *model.CreateOrderRequestNew
		ord *model.Order
	}

	type tcExpected struct {
		val string
		err error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "invalid_success_url",
			given: tcGiven{
				cl: &xstripe.MockClient{},
				req: &model.CreateOrderRequestNew{
					Email:    "you@example.com",
					Currency: "USD",
					StripeMetadata: &model.OrderStripeMetadata{
						SuccessURI: "://example.com/success",
						CancelURI:  "https://example.com/cancel",
					},
					PaymentMethods: []string{"stripe"},
				},
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
				},
			},
			exp: tcExpected{
				err: &url.Error{
					Op:  "parse",
					URL: "://example.com/success",
					Err: errors.New("missing protocol scheme"),
				},
			},
		},

		{
			name: "invalid_cancel_url",
			given: tcGiven{
				cl: &xstripe.MockClient{},
				req: &model.CreateOrderRequestNew{
					Email:    "you@example.com",
					Currency: "USD",
					StripeMetadata: &model.OrderStripeMetadata{
						SuccessURI: "https://example.com/success",
						CancelURI:  "://example.com/cancel",
					},
					PaymentMethods: []string{"stripe"},
				},
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
				},
			},
			exp: tcExpected{
				err: &url.Error{
					Op:  "parse",
					URL: "://example.com/cancel",
					Err: errors.New("missing protocol scheme"),
				},
			},
		},

		{
			name: "success",
			given: tcGiven{
				cl: &xstripe.MockClient{},
				req: &model.CreateOrderRequestNew{
					Email:    "you@example.com",
					Currency: "USD",
					StripeMetadata: &model.OrderStripeMetadata{
						SuccessURI: "https://example.com/success",
						CancelURI:  "https://example.com/cancel",
					},
					PaymentMethods: []string{"stripe"},
				},
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					Items: []model.OrderItem{
						{
							ID:       uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
							OrderID:  uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
							Quantity: 1,
							Metadata: datastore.Metadata{
								"stripe_item_id": "stripe_item_id",
							},
						},
					},
				},
			},
			exp: tcExpected{
				val: "cs_test_id",
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{stripeCl: tc.given.cl}

			ctx := context.Background()

			actual, err := svc.createStripeSession(ctx, tc.given.req, tc.given.ord)
			must.Equal(t, tc.exp.err, err)

			should.Equal(t, tc.exp.val, actual)
		})
	}
}

func TestService_recreateStripeSession(t *testing.T) {
	type tcGiven struct {
		ordRepo   *repository.MockOrder
		cl        *xstripe.MockClient
		ord       *model.Order
		oldSessID string
		email     string
	}

	type tcExpected struct {
		val string
		err error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "unable_fetch_old_session",
			given: tcGiven{
				ordRepo: &repository.MockOrder{},
				cl: &xstripe.MockClient{
					FnSession: func(ctx context.Context, id string, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						return nil, model.Error("something_went_wrong")
					},
				},
				ord:       &model.Order{},
				oldSessID: "cs_test_id_old",
			},
			exp: tcExpected{
				err: model.Error("something_went_wrong"),
			},
		},

		{
			name: "unable_create_session",
			given: tcGiven{
				ordRepo: &repository.MockOrder{},
				cl: &xstripe.MockClient{
					FnSession: func(ctx context.Context, id string, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						result := &stripe.CheckoutSession{
							ID:         "cs_test_id_old",
							SuccessURL: "https://example.com/success",
							CancelURL:  "https://example.com/cancel",
							Customer:   &stripe.Customer{Email: "you@example.com"},
						}

						return result, nil
					},

					FnCreateSession: func(ctx context.Context, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						return nil, model.Error("something_went_wrong")
					},
				},
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					Items: []model.OrderItem{
						{
							Quantity: 1,
							Metadata: datastore.Metadata{"stripe_item_id": "stripe_item_id"},
						},
					},
				},
				oldSessID: "cs_test_id_old",
			},
			exp: tcExpected{
				err: model.Error("something_went_wrong"),
			},
		},

		{
			name: "unable_append_metadata",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnAppendMetadata: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error {
						return model.Error("something_went_wrong")
					},
				},
				cl: &xstripe.MockClient{
					FnSession: func(ctx context.Context, id string, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						result := &stripe.CheckoutSession{
							ID:         "cs_test_id_old",
							SuccessURL: "https://example.com/success",
							CancelURL:  "https://example.com/cancel",
							Customer:   &stripe.Customer{Email: "you@example.com"},
						}

						return result, nil
					},
				},
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					Items: []model.OrderItem{
						{
							Quantity: 1,
							Metadata: datastore.Metadata{"stripe_item_id": "stripe_item_id"},
						},
					},
				},
				oldSessID: "cs_test_id_old",
			},
			exp: tcExpected{
				err: model.Error("something_went_wrong"),
			},
		},

		{
			name: "success_email_cust_from_session",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnAppendMetadata: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error {
						if key == "stripeCheckoutSessionId" && val == "cs_test_id" {
							return nil
						}

						return model.Error("unexpected")
					},
				},
				cl: &xstripe.MockClient{
					FnSession: func(ctx context.Context, id string, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						result := &stripe.CheckoutSession{
							ID:         "cs_test_id_old",
							SuccessURL: "https://example.com/success",
							CancelURL:  "https://example.com/cancel",
							Customer:   &stripe.Customer{ID: "cus_id", Email: "you@example.com"},
						}

						return result, nil
					},

					FnCreateSession: func(ctx context.Context, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						if *params.Customer != "cus_id" {
							return nil, model.Error("unexpected_customer")
						}

						if params.CustomerEmail != nil {
							return nil, model.Error("unexpected_customer_email")
						}

						result := &stripe.CheckoutSession{
							ID:                 "cs_test_id",
							PaymentMethodTypes: []string{"card"},
							Mode:               stripe.CheckoutSessionModeSubscription,
							SuccessURL:         *params.SuccessURL,
							CancelURL:          *params.CancelURL,
							ClientReferenceID:  *params.ClientReferenceID,
							Subscription: &stripe.Subscription{
								ID: "sub_id",
								Metadata: map[string]string{
									"orderID": *params.ClientReferenceID,
								},
							},
							AllowPromotionCodes: true,
						}

						return result, nil
					},
				},
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					Items: []model.OrderItem{
						{
							Quantity: 1,
							Metadata: datastore.Metadata{"stripe_item_id": "stripe_item_id"},
						},
					},
				},
				oldSessID: "cs_test_id_old",
			},
			exp: tcExpected{
				val: "cs_test_id",
			},
		},

		{
			name: "success_email_from_request_cust_without_email",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnAppendMetadata: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error {
						if key == "stripeCheckoutSessionId" && val == "cs_test_id" {
							return nil
						}

						return model.Error("unexpected")
					},
				},
				cl: &xstripe.MockClient{
					FnSession: func(ctx context.Context, id string, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						result := &stripe.CheckoutSession{
							ID:         "cs_test_id_old",
							SuccessURL: "https://example.com/success",
							CancelURL:  "https://example.com/cancel",
							Customer:   &stripe.Customer{ID: "cus_id"},
						}

						return result, nil
					},

					FnFindCustomer: func(ctx context.Context, email string) (*stripe.Customer, bool) {
						return nil, false
					},

					FnCreateSession: func(ctx context.Context, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						if params.Customer != nil {
							return nil, model.Error("unexpected_customer")
						}

						if *params.CustomerEmail != "request@example.com" {
							return nil, model.Error("unexpected_customer_email")
						}

						result := &stripe.CheckoutSession{
							ID:                 "cs_test_id",
							PaymentMethodTypes: []string{"card"},
							Mode:               stripe.CheckoutSessionModeSubscription,
							SuccessURL:         *params.SuccessURL,
							CancelURL:          *params.CancelURL,
							ClientReferenceID:  *params.ClientReferenceID,
							Subscription: &stripe.Subscription{
								ID: "sub_id",
								Metadata: map[string]string{
									"orderID": *params.ClientReferenceID,
								},
							},
							AllowPromotionCodes: true,
						}

						return result, nil
					},
				},
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					Items: []model.OrderItem{
						{
							Quantity: 1,
							Metadata: datastore.Metadata{"stripe_item_id": "stripe_item_id"},
						},
					},
				},
				oldSessID: "cs_test_id_old",
				email:     "request@example.com",
			},
			exp: tcExpected{
				val: "cs_test_id",
			},
		},

		{
			name: "success_email_from_request",
			given: tcGiven{
				ordRepo: &repository.MockOrder{
					FnAppendMetadata: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error {
						if key == "stripeCheckoutSessionId" && val == "cs_test_id" {
							return nil
						}

						return model.Error("unexpected")
					},
				},
				cl: &xstripe.MockClient{
					FnSession: func(ctx context.Context, id string, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						result := &stripe.CheckoutSession{
							ID:         "cs_test_id_old",
							SuccessURL: "https://example.com/success",
							CancelURL:  "https://example.com/cancel",
						}

						return result, nil
					},

					FnFindCustomer: func(ctx context.Context, email string) (*stripe.Customer, bool) {
						return nil, false
					},

					FnCreateSession: func(ctx context.Context, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						if params.Customer != nil {
							return nil, model.Error("unexpected_customer")
						}

						if *params.CustomerEmail != "request@example.com" {
							return nil, model.Error("unexpected_customer_email")
						}

						result := &stripe.CheckoutSession{
							ID:                 "cs_test_id",
							PaymentMethodTypes: []string{"card"},
							Mode:               stripe.CheckoutSessionModeSubscription,
							SuccessURL:         *params.SuccessURL,
							CancelURL:          *params.CancelURL,
							ClientReferenceID:  *params.ClientReferenceID,
							Subscription: &stripe.Subscription{
								ID: "sub_id",
								Metadata: map[string]string{
									"orderID": *params.ClientReferenceID,
								},
							},
							AllowPromotionCodes: true,
						}

						return result, nil
					},
				},
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					Items: []model.OrderItem{
						{
							Quantity: 1,
							Metadata: datastore.Metadata{"stripe_item_id": "stripe_item_id"},
						},
					},
				},
				oldSessID: "cs_test_id_old",
				email:     "request@example.com",
			},
			exp: tcExpected{
				val: "cs_test_id",
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{orderRepo: tc.given.ordRepo, stripeCl: tc.given.cl}

			ctx := context.Background()

			actual, err := svc.recreateStripeSession(ctx, nil, tc.given.ord, tc.given.oldSessID, tc.given.email)
			must.Equal(t, tc.exp.err, err)

			should.Equal(t, tc.exp.val, actual)
		})
	}
}

func TestCreateStripeSession(t *testing.T) {
	type tcGiven struct {
		cl  *xstripe.MockClient
		req createStripeSessionRequest
	}

	type tcExpected struct {
		val string
		err error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "success_discounts_metadata",
			given: tcGiven{
				cl: &xstripe.MockClient{
					FnCreateSession: func(ctx context.Context, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						if len(params.Discounts) != 1 {
							return nil, model.Error("unexpected_discounts")
						}

						if coup := params.Discounts[0].Coupon; coup == nil || *coup != "coup_id_01" {
							return nil, model.Error("unexpected_discount_val")
						}

						if val, ok := params.SubscriptionData.Params.Metadata["key_01"]; !ok || val != "val_01" {
							fmt.Println(params.SubscriptionData.Metadata)
							return nil, model.Error("unexpected_metadata_val")
						}

						if params.Extra != nil && params.Extra.Get("allow_promotion_codes") == "true" {
							return nil, model.Error("unexpected_extra_allow_promotion_codes")
						}

						result := &stripe.CheckoutSession{ID: "cs_test_id"}

						return result, nil
					},

					FnFindCustomer: func(ctx context.Context, email string) (*stripe.Customer, bool) {
						panic("unexpected_find_customer")
					},
				},

				req: createStripeSessionRequest{
					orderID:    "facade00-0000-4000-a000-000000000000",
					customerID: "cus_id",
					successURL: "https://example.com/success",
					cancelURL:  "https://example.com/cancel",
					trialDays:  7,
					items: []*stripe.CheckoutSessionLineItemParams{
						{
							Quantity: ptrTo[int64](1),
							Price:    ptrTo("stripe_item_id"),
						},
					},
					discounts: []*stripe.CheckoutSessionDiscountParams{
						{
							Coupon: ptrTo("coup_id_01"),
						},
					},
					metadata: map[string]string{
						"key_01": "val_01",
					},
				},
			},
			exp: tcExpected{
				val: "cs_test_id",
			},
		},

		{
			name: "success_cust_id",
			given: tcGiven{
				cl: &xstripe.MockClient{
					FnCreateSession: func(ctx context.Context, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						if params.Customer == nil || *params.Customer != "cus_id" {
							return nil, model.Error("unexpected")
						}

						result := &stripe.CheckoutSession{ID: "cs_test_id"}

						return result, nil
					},

					FnFindCustomer: func(ctx context.Context, email string) (*stripe.Customer, bool) {
						panic("unexpected_find_customer")
					},
				},

				req: createStripeSessionRequest{
					orderID:    "facade00-0000-4000-a000-000000000000",
					customerID: "cus_id",
					successURL: "https://example.com/success",
					cancelURL:  "https://example.com/cancel",
					trialDays:  7,
					items: []*stripe.CheckoutSessionLineItemParams{
						{
							Quantity: ptrTo[int64](1),
							Price:    ptrTo("stripe_item_id"),
						},
					},
				},
			},
			exp: tcExpected{
				val: "cs_test_id",
			},
		},

		{
			name: "success_cust_id_email",
			given: tcGiven{
				cl: &xstripe.MockClient{
					FnCreateSession: func(ctx context.Context, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						if params.Customer == nil || *params.Customer != "cus_id" {
							return nil, model.Error("unexpected")
						}

						result := &stripe.CheckoutSession{ID: "cs_test_id"}

						return result, nil
					},

					FnFindCustomer: func(ctx context.Context, email string) (*stripe.Customer, bool) {
						panic("unexpected_find_customer")
					},
				},

				req: createStripeSessionRequest{
					orderID:    "facade00-0000-4000-a000-000000000000",
					customerID: "cus_id",
					email:      "you@example.com",
					successURL: "https://example.com/success",
					cancelURL:  "https://example.com/cancel",
					trialDays:  7,
					items: []*stripe.CheckoutSessionLineItemParams{
						{
							Quantity: ptrTo[int64](1),
							Price:    ptrTo("stripe_item_id"),
						},
					},
				},
			},
			exp: tcExpected{
				val: "cs_test_id",
			},
		},

		{
			name: "success_email_found_customer",
			given: tcGiven{
				cl: &xstripe.MockClient{
					FnCreateSession: func(ctx context.Context, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						if params.Customer == nil || *params.Customer != "cus_id" {
							return nil, model.Error("unexpected")
						}

						result := &stripe.CheckoutSession{ID: "cs_test_id"}

						return result, nil
					},
				},

				req: createStripeSessionRequest{
					orderID:    "facade00-0000-4000-a000-000000000000",
					email:      "you@example.com",
					successURL: "https://example.com/success",
					cancelURL:  "https://example.com/cancel",
					trialDays:  7,
					items: []*stripe.CheckoutSessionLineItemParams{
						{
							Quantity: ptrTo[int64](1),
							Price:    ptrTo("stripe_item_id"),
						},
					},
				},
			},
			exp: tcExpected{
				val: "cs_test_id",
			},
		},

		{
			name: "success_email_customer_not_found",
			given: tcGiven{
				cl: &xstripe.MockClient{
					FnFindCustomer: func(ctx context.Context, email string) (*stripe.Customer, bool) {
						return nil, false
					},

					FnCreateSession: func(ctx context.Context, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						if params.CustomerEmail == nil || *params.CustomerEmail != "you@example.com" {
							return nil, model.Error("unexpected")
						}

						result := &stripe.CheckoutSession{ID: "cs_test_id"}

						return result, nil
					},
				},

				req: createStripeSessionRequest{
					orderID:    "facade00-0000-4000-a000-000000000000",
					email:      "you@example.com",
					successURL: "https://example.com/success",
					cancelURL:  "https://example.com/cancel",
					trialDays:  7,
					items: []*stripe.CheckoutSessionLineItemParams{
						{
							Quantity: ptrTo[int64](1),
							Price:    ptrTo("stripe_item_id"),
						},
					},
				},
			},
			exp: tcExpected{
				val: "cs_test_id",
			},
		},

		{
			name: "success_email_no_customer_email",
			given: tcGiven{
				cl: &xstripe.MockClient{
					FnFindCustomer: func(ctx context.Context, email string) (*stripe.Customer, bool) {
						return nil, false
					},
				},

				req: createStripeSessionRequest{
					orderID:    "facade00-0000-4000-a000-000000000000",
					successURL: "https://example.com/success",
					cancelURL:  "https://example.com/cancel",
					trialDays:  7,
					items: []*stripe.CheckoutSessionLineItemParams{
						{
							Quantity: ptrTo[int64](1),
							Price:    ptrTo("stripe_item_id"),
						},
					},
				},
			},
			exp: tcExpected{
				val: "cs_test_id",
			},
		},

		{
			name: "success_email_no_trial_days",
			given: tcGiven{
				cl: &xstripe.MockClient{},

				req: createStripeSessionRequest{
					orderID:    "facade00-0000-4000-a000-000000000000",
					email:      "you@example.com",
					successURL: "https://example.com/success",
					cancelURL:  "https://example.com/cancel",
					items: []*stripe.CheckoutSessionLineItemParams{
						{
							Quantity: ptrTo[int64](1),
							Price:    ptrTo("stripe_item_id"),
						},
					},
				},
			},
			exp: tcExpected{
				val: "cs_test_id",
			},
		},

		{
			name: "success_allow_promotion_codes",
			given: tcGiven{
				cl: &xstripe.MockClient{
					FnCreateSession: func(ctx context.Context, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						if params.Extra.Get("allow_promotion_codes") != "true" {
							return nil, model.Error("unexpected_extra_allow_promotion_codes")
						}

						result := &stripe.CheckoutSession{ID: "cs_test_id"}

						return result, nil
					},
				},

				req: createStripeSessionRequest{
					orderID:    "facade00-0000-4000-a000-000000000000",
					email:      "you@example.com",
					successURL: "https://example.com/success",
					cancelURL:  "https://example.com/cancel",
					items: []*stripe.CheckoutSessionLineItemParams{
						{
							Quantity: ptrTo[int64](1),
							Price:    ptrTo("stripe_item_id"),
						},
					},
				},
			},
			exp: tcExpected{
				val: "cs_test_id",
			},
		},

		{
			name: "create_error",
			given: tcGiven{
				cl: &xstripe.MockClient{
					FnCreateSession: func(ctx context.Context, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						return nil, model.Error("something_went_wrong")
					},
				},

				req: createStripeSessionRequest{
					orderID:    "facade00-0000-4000-a000-000000000000",
					email:      "you@example.com",
					successURL: "https://example.com/success",
					cancelURL:  "https://example.com/cancel",
					trialDays:  7,
					items: []*stripe.CheckoutSessionLineItemParams{
						{
							Quantity: ptrTo[int64](1),
							Price:    ptrTo("stripe_item_id"),
						},
					},
				},
			},
			exp: tcExpected{
				err: model.Error("something_went_wrong"),
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			actual, err := createStripeSession(ctx, tc.given.cl, tc.given.req)
			must.Equal(t, tc.exp.err, err)

			should.Equal(t, tc.exp.val, actual)
		})
	}
}

func TestBuildStripeLineItems(t *testing.T) {
	tests := []struct {
		name  string
		given []model.OrderItem
		exp   []*stripe.CheckoutSessionLineItemParams
	}{
		{
			name: "nil",
		},

		{
			name:  "empty_nil",
			given: []model.OrderItem{},
		},

		{
			name: "empty_no_price_id",
			given: []model.OrderItem{
				{
					Metadata: datastore.Metadata{"key": "value"},
				},
			},
		},

		{
			name: "one_item",
			given: []model.OrderItem{
				{
					Quantity: 1,
					Metadata: datastore.Metadata{"stripe_item_id": "stripe_item_id"},
				},
			},
			exp: []*stripe.CheckoutSessionLineItemParams{
				{
					Price:    ptrTo("stripe_item_id"),
					Quantity: ptrTo[int64](1),
				},
			},
		},

		{
			name: "two_items",
			given: []model.OrderItem{
				{
					Quantity: 1,
					Metadata: datastore.Metadata{"stripe_item_id": "stripe_item_id_01"},
				},

				{
					Quantity: 1,
					Metadata: datastore.Metadata{"stripe_item_id": "stripe_item_id_02"},
				},
			},
			exp: []*stripe.CheckoutSessionLineItemParams{
				{
					Price:    ptrTo("stripe_item_id_01"),
					Quantity: ptrTo[int64](1),
				},

				{
					Price:    ptrTo("stripe_item_id_02"),
					Quantity: ptrTo[int64](1),
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := buildStripeLineItems(tc.given)
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestBuildStripeDiscounts(t *testing.T) {
	tests := []struct {
		name  string
		given []string
		exp   []*stripe.CheckoutSessionDiscountParams
	}{
		{
			name: "nil",
		},

		{
			name:  "empty_nil",
			given: []string{},
		},

		{
			name:  "one",
			given: []string{"coup_id_01"},
			exp: []*stripe.CheckoutSessionDiscountParams{
				{Coupon: ptrTo("coup_id_01")},
			},
		},

		{
			name:  "two",
			given: []string{"coup_id_01", "coup_id_02"},
			exp: []*stripe.CheckoutSessionDiscountParams{
				{Coupon: ptrTo("coup_id_01")},
				{Coupon: ptrTo("coup_id_02")},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := buildStripeDiscounts(tc.given)
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestService_createRadomSessID(t *testing.T) {
	type tcExpected struct {
		sessionID string
		mustErr   must.ErrorAssertionFunc
	}

	type tcGiven struct {
		req   *model.CreateOrderRequestNew
		order *model.Order
		radCl radomClient
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "err_success_url",
			given: tcGiven{
				order: &model.Order{},
				req: &model.CreateOrderRequestNew{
					RadomMetadata: &model.OrderRadomMetadata{
						SuccessURI: "https://invalid%.com",
					},
				},
			},
			exp: tcExpected{
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.NotNil(t, err)
				},
			},
		},

		{
			name: "err_cancel_url",
			given: tcGiven{
				order: &model.Order{},
				req: &model.CreateOrderRequestNew{
					RadomMetadata: &model.OrderRadomMetadata{
						SuccessURI: "https://example.com",
						CancelURI:  "https://invalid%.com",
					},
				},
			},
			exp: tcExpected{
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.NotNil(t, err)
				},
			},
		},

		{
			name: "err_order_items_to_line_items",
			given: tcGiven{
				order: &model.Order{
					Items: []OrderItem{{}},
				},
				req: &model.CreateOrderRequestNew{
					RadomMetadata: &model.OrderRadomMetadata{
						SuccessURI: "https://example.com",
						CancelURI:  "https://example.com",
					},
				},
			},
			exp: tcExpected{
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.ErrorIs(t, err, errRadomProductIDNotFound)
				},
			},
		},

		{
			name: "err_create_checkout_session",
			given: tcGiven{
				order: &model.Order{
					Items: []OrderItem{
						{
							Metadata: datastore.Metadata{
								"radom_product_id": "product_1",
							},
						},
					},
				},
				req: &model.CreateOrderRequestNew{
					RadomMetadata: &model.OrderRadomMetadata{
						SuccessURI: "https://example.com",
						CancelURI:  "https://example.com",
					},
				},
				radCl: &mockRadomClient{
					fnCreateCheckoutSession: func(ctx context.Context, creq *radom.CreateCheckoutSessionRequest) (radom.CreateCheckoutSessionResponse, error) {
						return radom.CreateCheckoutSessionResponse{}, model.Error("some error")
					},
				},
			},
			exp: tcExpected{
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.ErrorIs(t, err, model.Error("some error"))
				},
			},
		},

		{
			name: "success",
			given: tcGiven{
				order: &model.Order{
					Items: []OrderItem{
						{
							Metadata: datastore.Metadata{
								"radom_product_id": "product_1",
							},
						},
					},
				},
				req: &model.CreateOrderRequestNew{
					RadomMetadata: &model.OrderRadomMetadata{
						SuccessURI:    "https://example.com",
						CancelURI:     "https://example.com",
						SubBackBtnURL: "https://example-back-button.com",
					},
				},
				radCl: &mockRadomClient{
					fnCreateCheckoutSession: func(ctx context.Context, creq *radom.CreateCheckoutSessionRequest) (radom.CreateCheckoutSessionResponse, error) {
						if creq.SubBackBtnURL != "https://example-back-button.com" {
							return radom.CreateCheckoutSessionResponse{}, model.Error("unexpected_response")
						}

						return radom.CreateCheckoutSessionResponse{SessionID: "session_id"}, nil
					},
				},
			},
			exp: tcExpected{
				sessionID: "session_id",
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.Nil(t, err)
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			s := Service{
				radomClient: tc.given.radCl,
			}
			ctx := context.Background()

			actual, err := s.createRadomSession(ctx, tc.given.req, tc.given.order)
			tc.exp.mustErr(t, err)

			should.Equal(t, tc.exp.sessionID, actual)
		})
	}
}

func Test_orderItemsToLineItems(t *testing.T) {
	type tcGiven struct {
		orderItems []model.OrderItem
	}

	type tcExpected struct {
		lineItems []radom.LineItem
		err       error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "product_id_not_found",
			given: tcGiven{
				orderItems: []OrderItem{{}},
			},
			exp: tcExpected{
				err: errRadomProductIDNotFound,
			},
		},

		{
			name: "success",
			given: tcGiven{
				orderItems: []OrderItem{
					{
						Metadata: map[string]interface{}{
							"radom_product_id": "product_1",
						},
					},
					{
						Metadata: map[string]interface{}{
							"radom_product_id": "product_2",
						},
					},
				},
			},
			exp: tcExpected{
				lineItems: []radom.LineItem{
					{
						ProductID: "product_1",
					},
					{
						ProductID: "product_2",
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, err := orderItemsToRadomLineItems(tc.given.orderItems)
			must.Equal(t, tc.exp.err, err)

			should.Equal(t, tc.exp.lineItems, actual)
		})
	}
}

func TestNewRadomGateway(t *testing.T) {
	type tcGiven struct {
		env string
	}

	type tcExpected struct {
		gateway *radom.Gateway
		err     error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "development",
			given: tcGiven{
				"development",
			},
			exp: tcExpected{
				gateway: &radom.Gateway{
					Managed: radom.Managed{
						Methods: []radom.Method{
							{
								Network:            "SepoliaTestnet",
								Token:              "0x5D684d37922dAf7Aa2013E65A22880a11C475e25",
								DiscountPercentOff: 0.20,
							},
							{
								Network:            "PolygonTestnet",
								Token:              "0xd445cAAbb9eA6685D3A512439256866563a16E93",
								DiscountPercentOff: 0.20,
							},
						},
					},
				},
			},
		},

		{
			name: "staging",
			given: tcGiven{
				"staging",
			},
			exp: tcExpected{
				gateway: &radom.Gateway{
					Managed: radom.Managed{
						Methods: []radom.Method{
							{
								Network:            "SepoliaTestnet",
								Token:              "0x5D684d37922dAf7Aa2013E65A22880a11C475e25",
								DiscountPercentOff: 0.20,
							},
							{
								Network:            "PolygonTestnet",
								Token:              "0xd445cAAbb9eA6685D3A512439256866563a16E93",
								DiscountPercentOff: 0.20,
							},
						},
					},
				},
			},
		},

		{
			name: "production",
			given: tcGiven{
				"production",
			},
			exp: tcExpected{
				gateway: &radom.Gateway{
					Managed: radom.Managed{
						Methods: []radom.Method{
							{
								Network:            "Polygon",
								Token:              "0x3cef98bb43d732e2f285ee605a8158cde967d219",
								DiscountPercentOff: 0.20,
							},

							{
								Network:            "Ethereum",
								Token:              "0x0d8775f648430679a709e98d2b0cb6250d2887ef",
								DiscountPercentOff: 0.20,
							},
						},
					},
				},
			},
		},

		{
			name: "unknown",
			given: tcGiven{
				"random_env",
			},
			exp: tcExpected{
				err: model.Error("skus: unknown environment"),
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, err := newRadomGateway(tc.given.env)
			must.Equal(t, tc.exp.err, err)

			should.Equal(t, tc.exp.gateway, actual)
		})
	}
}

func TestService_processRadomNotification(t *testing.T) {
	type tcGiven struct {
		event *radom.Notification
	}

	type tcExpected struct {
		err error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "should_not_process",
			given: tcGiven{
				event: &radom.Notification{},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			s := Service{}

			actual := s.processRadomNotification(context.Background(), tc.given.event)

			should.ErrorIs(t, actual, tc.exp.err)
		})
	}
}

func TestService_processRadomNotificationTx(t *testing.T) {
	type tcGiven struct {
		event           *radom.Notification
		orderRepo       orderStoreSvc
		orderPayHistory orderPayHistoryStore
		radomCl         radomClient
	}

	type tcExpected struct {
		shouldErr should.ErrorAssertionFunc
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "new_subscription",
			given: tcGiven{
				event: &radom.Notification{
					EventData: &radom.EventData{
						New: &radom.NewSubscription{
							SubscriptionID: uuid.NewV4(),
						},
					},
					RadomData: &radom.Data{
						CheckoutSession: &radom.CheckoutSession{
							Metadata: []radom.Metadata{
								{
									Key:   "brave_order_id",
									Value: "d14c5b2e-b719-4504-b034-86e74a932295",
								},
							},
						},
					},
				},
				orderRepo:       &repository.MockOrder{},
				orderPayHistory: &repository.MockOrderPayHistory{},
				radomCl: &mockRadomClient{
					fnGetSubscription: func(ctx context.Context, subID string) (*radom.SubscriptionResponse, error) {
						return &radom.SubscriptionResponse{
							NextBillingDateAt: "2023-06-12T09:38:13.604410Z",
							Payments: []radom.Payment{
								{
									Date: "2023-06-12T09:38:13.604410Z",
								},
							},
						}, nil
					},
				},
			},
			exp: tcExpected{
				shouldErr: func(t should.TestingT, err error, i ...interface{}) bool {
					return should.NoError(t, err)
				},
			},
		},

		{
			name: "subscription_payment",
			given: tcGiven{
				event: &radom.Notification{
					EventData: &radom.EventData{
						Payment: &radom.SubscriptionPayment{
							RadomData: &radom.Data{
								Subscription: &radom.Subscription{
									SubscriptionID: uuid.NewV4(),
								},
							},
						},
					},
				},
				orderRepo:       &repository.MockOrder{},
				orderPayHistory: &repository.MockOrderPayHistory{},
				radomCl: &mockRadomClient{
					fnGetSubscription: func(ctx context.Context, subID string) (*radom.SubscriptionResponse, error) {
						return &radom.SubscriptionResponse{
							NextBillingDateAt: "2023-06-12T09:38:13.604410Z",
							Payments: []radom.Payment{
								{
									Date: "2023-06-12T09:38:13.604410Z",
								},
							},
						}, nil
					},
				},
			},
			exp: tcExpected{
				shouldErr: func(t should.TestingT, err error, i ...interface{}) bool {
					return should.NoError(t, err)
				},
			},
		},

		{
			name: "subscription_payment_reset_payment_failed_error",
			given: tcGiven{
				event: &radom.Notification{
					EventData: &radom.EventData{
						Payment: &radom.SubscriptionPayment{
							RadomData: &radom.Data{
								Subscription: &radom.Subscription{
									SubscriptionID: uuid.NewV4(),
								},
							},
						},
					},
				},
				orderRepo: &repository.MockOrder{
					FnAppendMetadataInt: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key string, val int) error {
						return model.Error("reset_payment_failed_error")
					},
				},
				orderPayHistory: &repository.MockOrderPayHistory{},
				radomCl: &mockRadomClient{
					fnGetSubscription: func(ctx context.Context, subID string) (*radom.SubscriptionResponse, error) {
						return &radom.SubscriptionResponse{
							NextBillingDateAt: "2023-06-12T09:38:13.604410Z",
							Payments: []radom.Payment{
								{
									Date: "2023-06-12T09:38:13.604410Z",
								},
							},
						}, nil
					},
				},
			},
			exp: tcExpected{
				shouldErr: func(t should.TestingT, err error, i ...interface{}) bool {
					return should.ErrorIs(t, err, model.Error("reset_payment_failed_error"))
				},
			},
		},

		{
			name: "subscription_cancelled",
			given: tcGiven{
				event: &radom.Notification{
					EventData: &radom.EventData{
						Cancelled: &radom.SubscriptionCancelled{
							SubscriptionID: uuid.NewV4(),
						},
					},
				},
				orderRepo: &repository.MockOrder{},
			},
			exp: tcExpected{
				shouldErr: func(t should.TestingT, err error, i ...interface{}) bool {
					return should.NoError(t, err)
				},
			},
		},

		{
			name: "subscription_cancelled_reset_payment_failed_error",
			given: tcGiven{
				event: &radom.Notification{
					EventData: &radom.EventData{
						Cancelled: &radom.SubscriptionCancelled{
							SubscriptionID: uuid.NewV4(),
						},
					},
				},
				orderRepo: &repository.MockOrder{
					FnAppendMetadataInt: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key string, val int) error {
						return model.Error("reset_payment_failed_error")
					},
				},
			},
			exp: tcExpected{
				shouldErr: func(t should.TestingT, err error, i ...interface{}) bool {
					return should.ErrorIs(t, err, model.Error("reset_payment_failed_error"))
				},
			},
		},

		{
			name: "subscription_expired",
			given: tcGiven{
				event: &radom.Notification{
					EventData: &radom.EventData{
						Expired: &radom.SubscriptionExpired{
							SubscriptionID: uuid.NewV4(),
						},
					},
				},
				orderRepo: &repository.MockOrder{},
			},
			exp: tcExpected{
				shouldErr: func(t should.TestingT, err error, i ...interface{}) bool {
					return should.NoError(t, err)
				},
			},
		},

		{
			name: "subscription_payment_failure",
			given: tcGiven{
				event: &radom.Notification{
					EventData: &radom.EventData{
						PaymentAttemptFailure: &radom.SubscriptionPaymentAttemptFailure{
							SubscriptionID: uuid.NewV4(),
						},
					},
				},
				orderRepo: &repository.MockOrder{},
			},
			exp: tcExpected{
				shouldErr: func(t should.TestingT, err error, i ...interface{}) bool {
					return should.NoError(t, err)
				},
			},
		},

		{
			name: "subscription_payment_overdue",
			given: tcGiven{
				event: &radom.Notification{
					EventData: &radom.EventData{
						PaymentOverdue: &radom.SubscriptionPaymentOverdue{
							SubscriptionID: uuid.NewV4(),
						},
					},
				},
				orderRepo: &repository.MockOrder{},
			},
			exp: tcExpected{
				shouldErr: func(t should.TestingT, err error, i ...interface{}) bool {
					return should.NoError(t, err)
				},
			},
		},

		{
			name: "unknown_action",
			given: tcGiven{
				event: &radom.Notification{},
			},
			exp: tcExpected{
				shouldErr: func(t should.TestingT, err error, i ...interface{}) bool {
					return should.ErrorIs(t, err, errRadomUnknownAction)
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{orderRepo: tc.given.orderRepo, payHistRepo: tc.given.orderPayHistory, radomClient: tc.given.radomCl}

			ctx := context.Background()

			actual := svc.processRadomNotificationTx(ctx, nil, tc.given.event)
			tc.exp.shouldErr(t, actual)
		})
	}
}

func TestService_cancelOrderTx(t *testing.T) {
	type tcGiven struct {
		orepo *repository.MockOrder
		id    uuid.UUID
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   error
	}

	tests := []testCase{
		{
			name: "error",
			given: tcGiven{
				orepo: &repository.MockOrder{
					FnSetStatus: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, status string) error {
						return model.Error("something_went_wrong")
					},
				},
				id: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "success",
			given: tcGiven{
				orepo: &repository.MockOrder{
					FnSetStatus: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, status string) error {
						if !uuid.Equal(id, uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000"))) {
							return model.Error("unexpected_id")
						}

						if status != model.OrderStatusCanceled {
							return model.Error("unexpected_status")
						}

						return nil
					},
				},
				id: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{orderRepo: tc.given.orepo}

			ctx := context.Background()

			actual := svc.cancelOrderTx(ctx, nil, tc.given.id)
			should.ErrorIs(t, actual, tc.exp)
		})
	}
}

func TestService_updateNumPaymentFailed(t *testing.T) {
	type tcGiven struct {
		orepo *repository.MockOrder
		id    uuid.UUID
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   error
	}

	tests := []testCase{
		{
			name: "error",
			given: tcGiven{
				orepo: &repository.MockOrder{
					FnAppendMetadataInt: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key string, val int) error {
						return model.Error("something_went_wrong")
					},
				},
				id: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "success",
			given: tcGiven{
				orepo: &repository.MockOrder{
					FnAppendMetadataInt: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key string, val int) error {
						if !uuid.Equal(id, uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000"))) {
							return model.Error("unexpected_id")
						}

						if val != 0 {
							return model.Error("unexpected_val")
						}

						return nil
					},
				},
				id: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{orderRepo: tc.given.orepo}

			ctx := context.Background()

			actual := svc.resetNumPaymentFailed(ctx, nil, tc.given.id)
			should.ErrorIs(t, actual, tc.exp)
		})
	}
}

func TestService_setOrderTrialDaysTx(t *testing.T) {
	type tcGiven struct {
		orepo  *repository.MockOrder
		oirepo *repository.MockOrderItem
		scl    stripeClient

		id  uuid.UUID
		req *model.SetTrialDaysRequest
		now time.Time
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   error
	}

	tests := []testCase{
		{
			name: "error_set_trial_days",
			given: tcGiven{
				orepo: &repository.MockOrder{
					FnSetTrialDays: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, ndays int64) error {
						return model.Error("something_went_wrong")
					},
				},
				oirepo: &repository.MockOrderItem{},
				scl:    &xstripe.MockClient{},
				id:     uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000"),
				req:    &model.SetTrialDaysRequest{Email: "you@example.com", TrialDays: 7},
				now:    time.Date(2024, time.November, 1, 0, 0, 0, 0, time.UTC),
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "error_get_order_full",
			given: tcGiven{
				orepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						return nil, model.Error("something_went_wrong")
					},
				},
				oirepo: &repository.MockOrderItem{},
				scl:    &xstripe.MockClient{},
				id:     uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000"),
				req:    &model.SetTrialDaysRequest{Email: "you@example.com", TrialDays: 7},
				now:    time.Date(2024, time.November, 1, 0, 0, 0, 0, time.UTC),
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "success_no_new_session",
			given: tcGiven{
				orepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID:                    uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000"),
							Status:                model.OrderStatusPaid,
							ExpiresAt:             ptrTo(time.Date(2024, time.December, 1, 0, 0, 0, 0, time.UTC)),
							AllowedPaymentMethods: pq.StringArray{"stripe"},
						}

						return result, nil
					},
				},
				oirepo: &repository.MockOrderItem{},
				scl:    &xstripe.MockClient{},
				id:     uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000"),
				req:    &model.SetTrialDaysRequest{Email: "you@example.com", TrialDays: 7},
				now:    time.Date(2024, time.November, 1, 0, 0, 0, 0, time.UTC),
			},
		},

		{
			name: "error_no_session_id",
			given: tcGiven{
				orepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID:                    uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000"),
							Status:                model.OrderStatusPending,
							AllowedPaymentMethods: pq.StringArray{"stripe"},
						}

						return result, nil
					},
				},
				oirepo: &repository.MockOrderItem{},
				scl:    &xstripe.MockClient{},
				id:     uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000"),
				req:    &model.SetTrialDaysRequest{Email: "you@example.com", TrialDays: 7},
				now:    time.Date(2024, time.November, 1, 0, 0, 0, 0, time.UTC),
			},
			exp: model.ErrNoStripeCheckoutSessID,
		},

		{
			name: "error_recreate_stripe_session",
			given: tcGiven{
				orepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID:                    uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000"),
							Status:                model.OrderStatusPending,
							AllowedPaymentMethods: pq.StringArray{"stripe"},
							Metadata: datastore.Metadata{
								"stripeCheckoutSessionId": "sess_id",
							},
						}

						return result, nil
					},
				},
				oirepo: &repository.MockOrderItem{},
				scl: &xstripe.MockClient{
					FnSession: func(ctx context.Context, id string, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						return nil, model.Error("something_went_wrong")
					},
				},

				id:  uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000"),
				req: &model.SetTrialDaysRequest{Email: "you@example.com", TrialDays: 7},
				now: time.Date(2024, time.November, 1, 0, 0, 0, 0, time.UTC),
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "success",
			given: tcGiven{
				orepo: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						result := &model.Order{
							ID:                    uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000"),
							Status:                model.OrderStatusPending,
							AllowedPaymentMethods: pq.StringArray{"stripe"},
							Metadata: datastore.Metadata{
								"stripeCheckoutSessionId": "sess_id",
							},
							TrialDays: ptrTo(int64(7)),
						}

						return result, nil
					},

					FnAppendMetadata: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error {
						if key == "stripeCheckoutSessionId" && val == "cs_test_id" {
							return nil
						}

						return model.Error("unexpected_append_metadata")
					},
				},
				oirepo: &repository.MockOrderItem{},
				scl: &xstripe.MockClient{
					FnCreateSession: func(ctx context.Context, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
						if *params.SubscriptionData.TrialPeriodDays != 7 {
							return nil, model.Error("unexpected_trial_period_days")
						}

						result := &stripe.CheckoutSession{
							ID: "cs_test_id",
						}

						return result, nil
					},
				},
				id:  uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000"),
				req: &model.SetTrialDaysRequest{Email: "you@example.com", TrialDays: 7},
				now: time.Date(2024, time.November, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{
				orderRepo:     tc.given.orepo,
				orderItemRepo: tc.given.oirepo,
				stripeCl:      tc.given.scl,
			}

			ctx := context.Background()

			actual := svc.setOrderTrialDaysTx(ctx, nil, tc.given.id, tc.given.req, tc.given.now)
			should.ErrorIs(t, actual, tc.exp)
		})
	}
}

func TestHandleRedeemFnError(t *testing.T) {
	type tcGiven struct {
		kind string
		cred *cbr.CredentialRedemption
		err  error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   *handlers.AppError
	}

	tests := []testCase{
		{
			name: "500_tlv2_not_found",
			given: tcGiven{
				kind: timeLimitedV2,
				cred: &cbr.CredentialRedemption{TokenPreimage: "token_preimage"},
				err: berrs.New(model.Error("not found"), "cbr route not found", berrs.Codified{
					ErrCode: "cbr_path_not_found",
					Retry:   true,
				}),
			},
			exp: &handlers.AppError{
				Code: http.StatusInternalServerError,
				Cause: berrs.New(model.Error("not found"), "cbr route not found", berrs.Codified{
					ErrCode: "cbr_path_not_found",
					Retry:   true,
				}),
				Message: "Error verifying credentials",
			},
		},

		{
			name: "200_tlv2_duplicate",
			given: tcGiven{
				kind: timeLimitedV2,
				cred: &cbr.CredentialRedemption{TokenPreimage: "token_preimage"},
				err: berrs.New(model.Error("duplicate redemption"), "cbr duplicate redemption", berrs.Codified{
					ErrCode: "cbr_dup_redeem",
				}),
			},
		},

		{
			name: "403_tlv2_bad_request",
			given: tcGiven{
				kind: timeLimitedV2,
				cred: &cbr.CredentialRedemption{TokenPreimage: "token_preimage"},
				err: berrs.New(model.Error("bad request"), "cbr bad request", berrs.Codified{
					ErrCode: "cbr_bad_request",
				}),
			},
			exp: &handlers.AppError{
				Code: http.StatusForbidden,
				Cause: berrs.New(model.Error("bad request"), "cbr bad request", berrs.Codified{
					ErrCode: "cbr_bad_request",
				}),
				Message: "invalid credentials",
			},
		},

		{
			name: "403_tlv_duplicate",
			given: tcGiven{
				kind: timeLimited,
				cred: &cbr.CredentialRedemption{TokenPreimage: "token_preimage"},
				err: berrs.New(model.Error("duplicate redemption"), "cbr duplicate redemption", berrs.Codified{
					ErrCode: "cbr_dup_redeem",
				}),
			},
			exp: &handlers.AppError{
				Code: http.StatusForbidden,
				Cause: berrs.New(model.Error("duplicate redemption"), "cbr duplicate redemption", berrs.Codified{
					ErrCode: "cbr_dup_redeem",
				}),
				Message: "invalid credentials",
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			rw := httptest.NewRecorder()
			rw.Header().Set("content-type", "application/json")

			actual := handleRedeemFnError(ctx, rw, tc.given.kind, tc.given.cred, tc.given.err)
			if actual != nil {
				should.Equal(t, tc.exp.Code, actual.Code)
				should.Equal(t, tc.exp.Cause, actual.Cause)
				should.Equal(t, tc.exp.Message, actual.Message)

				return
			}

			must.Equal(t, http.StatusOK, rw.Code)

			resp := &blindedCredVrfResult{}
			err := json.Unmarshal(rw.Body.Bytes(), resp)
			must.NoError(t, err)

			should.Equal(t, tc.given.cred.TokenPreimage, resp.ID)
			should.True(t, resp.Duplicate)
		})
	}
}

func TestService_processStripeMtoA(t *testing.T) {
	type tcGiven struct {
		repo     *repository.MockOrder
		stripeCl *xstripe.MockClient
		ntf      *stripeNotification
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   error
	}

	tests := []testCase{
		{
			name: "skip_incomplete_uma_data",
			given: tcGiven{
				repo:     &repository.MockOrder{},
				stripeCl: &xstripe.MockClient{},
				ntf: &stripeNotification{
					invoice: &stripe.Invoice{
						Lines: &stripe.InvoiceLineList{
							Data: []*stripe.InvoiceLine{
								{Metadata: map[string]string{}},
							},
						},
					},
				},
			},
		},

		{
			name: "error_invalid_data",
			given: tcGiven{
				repo:     &repository.MockOrder{},
				stripeCl: &xstripe.MockClient{},
				ntf: &stripeNotification{
					invoice: &stripe.Invoice{
						Lines: &stripe.InvoiceLineList{},
					},
				},
			},
			exp: errStripeNoInvoiceLines,
		},

		{
			name: "error_invalid_order_id",
			given: tcGiven{
				repo:     &repository.MockOrder{},
				stripeCl: &xstripe.MockClient{},
				ntf: &stripeNotification{
					invoice: &stripe.Invoice{
						Lines: &stripe.InvoiceLineList{
							Data: []*stripe.InvoiceLine{
								{
									Metadata: map[string]string{
										"uma__st_sub_id": "st_sub_id_01",
										"uma__sub_id":    "facade00-0000-4000-a000-000000000000",
										"uma__order_id":  "decade00-0000-4000-a000-00000000000",
									},
								},
							},
						},
					},
				},
			},
			exp: errors.New("uuid: incorrect UUID length: decade00-0000-4000-a000-00000000000"),
		},

		{
			name: "error_cancel_order",
			given: tcGiven{
				repo: &repository.MockOrder{
					FnSetStatus: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, status string) error {
						return model.Error("something_went_wrong")
					},
				},
				stripeCl: &xstripe.MockClient{},
				ntf: &stripeNotification{
					invoice: &stripe.Invoice{
						Lines: &stripe.InvoiceLineList{
							Data: []*stripe.InvoiceLine{
								{
									Metadata: map[string]string{
										"uma__st_sub_id": "st_sub_id_01",
										"uma__sub_id":    "facade00-0000-4000-a000-000000000000",
										"uma__order_id":  "decade00-0000-4000-a000-000000000000",
									},
								},
							},
						},
					},
				},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "error_cancel_sub",
			given: tcGiven{
				repo: &repository.MockOrder{},
				stripeCl: &xstripe.MockClient{
					FnCancelSub: func(ctx context.Context, id string, params *stripe.SubscriptionCancelParams) error {
						return model.Error("something_went_wrong")
					},
				},
				ntf: &stripeNotification{
					invoice: &stripe.Invoice{
						Lines: &stripe.InvoiceLineList{
							Data: []*stripe.InvoiceLine{
								{
									Metadata: map[string]string{
										"uma__st_sub_id": "st_sub_id_01",
										"uma__sub_id":    "facade00-0000-4000-a000-000000000000",
										"uma__order_id":  "decade00-0000-4000-a000-000000000000",
									},
								},
							},
						},
					},
				},
			},
			exp: model.Error("something_went_wrong"),
		},

		{
			name: "success_cancel_sub_not_found_no_coupon",
			given: tcGiven{
				repo: &repository.MockOrder{},
				stripeCl: &xstripe.MockClient{
					FnCancelSub: func(ctx context.Context, id string, params *stripe.SubscriptionCancelParams) error {
						rerr := &stripe.Error{
							HTTPStatusCode: http.StatusNotFound,
							Code:           stripe.ErrorCodeResourceMissing,
						}

						return rerr
					},
				},
				ntf: &stripeNotification{
					invoice: &stripe.Invoice{
						Lines: &stripe.InvoiceLineList{
							Data: []*stripe.InvoiceLine{
								{
									Metadata: map[string]string{
										"uma__st_sub_id": "st_sub_id_01",
										"uma__sub_id":    "facade00-0000-4000-a000-000000000000",
										"uma__order_id":  "decade00-0000-4000-a000-000000000000",
									},
								},
							},
						},
					},
				},
			},
		},

		{
			name: "success_no_coupon",
			given: tcGiven{
				repo: &repository.MockOrder{
					FnSetStatus: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, status string) error {
						if !uuid.Equal(id, uuid.Must(uuid.FromString("decade00-0000-4000-a000-000000000000"))) {
							return model.Error("unexpected_cancel_order_id")
						}

						if status != model.OrderStatusCanceled {
							return model.Error("unexpected_cancel_order_status")
						}

						return nil
					},
				},
				stripeCl: &xstripe.MockClient{
					FnCancelSub: func(ctx context.Context, id string, params *stripe.SubscriptionCancelParams) error {
						if id != "st_sub_id_01" {
							return model.Error("unexpected_cancel_sub_id")
						}

						return nil
					},
				},
				ntf: &stripeNotification{
					invoice: &stripe.Invoice{
						Lines: &stripe.InvoiceLineList{
							Data: []*stripe.InvoiceLine{
								{
									Metadata: map[string]string{
										"uma__st_sub_id": "st_sub_id_01",
										"uma__sub_id":    "facade00-0000-4000-a000-000000000000",
										"uma__order_id":  "decade00-0000-4000-a000-000000000000",
									},
								},
							},
						},
					},
				},
			},
		},

		{
			name: "success",
			given: tcGiven{
				repo: &repository.MockOrder{
					FnSetStatus: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, status string) error {
						if !uuid.Equal(id, uuid.Must(uuid.FromString("decade00-0000-4000-a000-000000000000"))) {
							return model.Error("unexpected_cancel_order_id")
						}

						if status != model.OrderStatusCanceled {
							return model.Error("unexpected_cancel_order_status")
						}

						return nil
					},
				},
				stripeCl: &xstripe.MockClient{
					FnCancelSub: func(ctx context.Context, id string, params *stripe.SubscriptionCancelParams) error {
						if id != "st_sub_id_01" {
							return model.Error("unexpected_cancel_sub_id")
						}

						return nil
					},
				},
				ntf: &stripeNotification{
					invoice: &stripe.Invoice{
						Lines: &stripe.InvoiceLineList{
							Data: []*stripe.InvoiceLine{
								{
									Metadata: map[string]string{
										"uma__st_sub_id": "st_sub_id_01",
										"uma__sub_id":    "facade00-0000-4000-a000-000000000000",
										"uma__order_id":  "decade00-0000-4000-a000-000000000000",
										"uma__coupon_id": "coup_id_01",
									},
								},
							},
						},
						Discount: &stripe.Discount{
							Coupon: &stripe.Coupon{ID: "coup_id_01"},
						},
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{
				orderRepo: tc.given.repo,
				stripeCl:  tc.given.stripeCl,
			}

			ctx := context.Background()

			actual := svc.processStripeMtoA(ctx, nil, tc.given.ntf)
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestService_updateOrderRadomSession(t *testing.T) {
	type tcGiven struct {
		ord *model.Order
		oss orderStoreSvc
		ois orderItemStore
		oph orderPayHistoryStore
		rcl radomClient
	}

	type tcExpected struct {
		err error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "get_order_full_tx_error",
			given: tcGiven{
				ord: &model.Order{},
				oss: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						return nil, model.Error("get_order_full_tx_error")
					},
				},
			},
			exp: tcExpected{
				err: model.Error("get_order_full_tx_error"),
			},
		},

		{
			name: "radom_sesh_id_error",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000"),
				},
				oss: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						oid := uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000")

						if id != oid {
							return nil, model.Error("wrong order id")
						}

						return &model.Order{ID: oid}, nil
					},
				},
				ois: &repository.MockOrderItem{},
			},
			exp: tcExpected{
				err: model.ErrNoRadomCheckoutSessionID,
			},
		},

		{
			name: "get_checkout_session_error",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000"),
				},
				oss: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						oid := uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000")

						if id != oid {
							return nil, model.Error("wrong order id")
						}

						ord := &model.Order{
							Metadata: datastore.Metadata{"radomCheckoutSessionId": "sesh_id"},
						}

						return ord, nil
					},
				},
				ois: &repository.MockOrderItem{},
				rcl: &mockRadomClient{
					fnGetCheckoutSession: func(ctx context.Context, seshID string) (radom.GetCheckoutSessionResponse, error) {
						return radom.GetCheckoutSessionResponse{}, model.Error("get_checkout_session_error")
					},
				},
			},
			exp: tcExpected{
				err: model.Error("get_checkout_session_error"),
			},
		},

		{
			name: "session_success_invalid_assoc_subs_error",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000"),
				},
				oss: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						oid := uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000")

						if id != oid {
							return nil, model.Error("wrong order id")
						}

						ord := &model.Order{
							Metadata: datastore.Metadata{"radomCheckoutSessionId": "sesh_id"},
						}

						return ord, nil
					},
				},
				ois: &repository.MockOrderItem{},
				rcl: &mockRadomClient{
					fnGetCheckoutSession: func(ctx context.Context, seshID string) (radom.GetCheckoutSessionResponse, error) {
						return radom.GetCheckoutSessionResponse{
							SessionStatus: "success",
						}, nil
					},
				},
			},
			exp: tcExpected{
				err: model.ErrRadomInvalidNumAssocSubs,
			},
		},

		{
			name: "session_success_get_radom_subscription_error",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000"),
				},
				oss: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						oid := uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000")

						if id != oid {
							return nil, model.Error("wrong order id")
						}

						ord := &model.Order{
							Metadata: datastore.Metadata{"radomCheckoutSessionId": "sesh_id"},
						}

						return ord, nil
					},
				},
				ois: &repository.MockOrderItem{},
				rcl: &mockRadomClient{
					fnGetCheckoutSession: func(ctx context.Context, seshID string) (radom.GetCheckoutSessionResponse, error) {
						return radom.GetCheckoutSessionResponse{
							SessionStatus: "success",
							AssocSubscriptions: []radom.SubscriptionResponse{
								{
									ID: "radom_sub_id",
								},
							},
						}, nil
					},
					fnGetSubscription: func(ctx context.Context, subID string) (*radom.SubscriptionResponse, error) {
						return nil, model.Error("session_success_get_radom_subscription_error")
					},
				},
			},
			exp: tcExpected{
				err: model.Error("session_success_get_radom_subscription_error"),
			},
		},

		{
			name: "session_success_radom_subscription_not_active_error",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000"),
				},
				oss: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						oid := uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000")

						if id != oid {
							return nil, model.Error("wrong order id")
						}

						ord := &model.Order{
							Metadata: datastore.Metadata{"radomCheckoutSessionId": "sesh_id"},
						}

						return ord, nil
					},
				},
				ois: &repository.MockOrderItem{},
				rcl: &mockRadomClient{
					fnGetCheckoutSession: func(ctx context.Context, seshID string) (radom.GetCheckoutSessionResponse, error) {
						return radom.GetCheckoutSessionResponse{
							SessionStatus: "success",
							AssocSubscriptions: []radom.SubscriptionResponse{
								{
									ID: "radom_sub_id",
								},
							},
						}, nil
					},
					fnGetSubscription: func(ctx context.Context, subID string) (*radom.SubscriptionResponse, error) {
						if subID != "radom_sub_id" {
							return nil, model.Error("wrong_radom_sub_id")
						}

						return &radom.SubscriptionResponse{
							Status: "not_active",
						}, nil
					},
				},
			},
			exp: tcExpected{
				err: model.ErrRadomSubNotActive,
			},
		},

		{
			name: "session_success",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000"),
				},
				oss: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						oid := uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000")

						if id != oid {
							return nil, model.Error("wrong order id")
						}

						ord := &model.Order{
							Metadata: datastore.Metadata{"radomCheckoutSessionId": "sesh_id"},
						}

						return ord, nil
					},
				},
				ois: &repository.MockOrderItem{
					FnFindByOrderID: func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) ([]model.OrderItem, error) {
						if orderID != uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000") {
							return nil, model.Error("wrong order id")
						}

						items := []model.OrderItem{
							{
								Metadata: datastore.Metadata{"radom_product_id": "product_id"},
							},
						}

						return items, nil
					},
				},
				oph: &repository.MockOrderPayHistory{},
				rcl: &mockRadomClient{
					fnGetCheckoutSession: func(ctx context.Context, seshID string) (radom.GetCheckoutSessionResponse, error) {
						return radom.GetCheckoutSessionResponse{
							SessionStatus: "success",
							AssocSubscriptions: []radom.SubscriptionResponse{
								{
									ID: "radom_sub_id",
								},
							},
						}, nil
					},
					fnGetSubscription: func(ctx context.Context, subID string) (*radom.SubscriptionResponse, error) {
						if subID != "radom_sub_id" {
							return nil, model.Error("wrong_radom_sub_id")
						}

						return &radom.SubscriptionResponse{
							Status:            "active",
							NextBillingDateAt: "2019-08-24T14:15:22Z",
							Payments: []radom.Payment{
								{
									Date: "2019-08-24T14:15:22Z",
								},
							},
						}, nil
					},
				},
			},
		},

		{
			name: "session_expired_create_radom_session_error",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000"),
				},
				oss: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						oid := uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000")

						if id != oid {
							return nil, model.Error("wrong order id")
						}

						ord := &model.Order{
							Metadata: datastore.Metadata{"radomCheckoutSessionId": "sesh_id"},
						}

						return ord, nil
					},
				},
				ois: &repository.MockOrderItem{},
				rcl: &mockRadomClient{
					fnGetCheckoutSession: func(ctx context.Context, seshID string) (radom.GetCheckoutSessionResponse, error) {
						return radom.GetCheckoutSessionResponse{
							SessionStatus: "expired",
						}, nil
					},
				},
			},
			exp: tcExpected{
				err: errRadomProductIDNotFound,
			},
		},

		{
			name: "session_expired_append_metadata_error",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000"),
				},
				oss: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						oid := uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000")

						if id != oid {
							return nil, model.Error("wrong order id")
						}

						ord := &model.Order{
							Metadata: datastore.Metadata{"radomCheckoutSessionId": "sesh_id"},
						}

						return ord, nil
					},
					FnAppendMetadata: func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error {
						return model.Error("session_expired_append_metadata_error")
					},
				},
				ois: &repository.MockOrderItem{
					FnFindByOrderID: func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) ([]model.OrderItem, error) {
						if orderID != uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000") {
							return nil, model.Error("wrong order id")
						}

						items := []model.OrderItem{
							{
								Metadata: datastore.Metadata{"radom_product_id": "product_id"},
							},
						}

						return items, nil
					},
				},
				rcl: &mockRadomClient{
					fnGetCheckoutSession: func(ctx context.Context, seshID string) (radom.GetCheckoutSessionResponse, error) {
						return radom.GetCheckoutSessionResponse{
							SessionStatus: "expired",
						}, nil
					},
				},
			},
			exp: tcExpected{
				err: model.Error("session_expired_append_metadata_error"),
			},
		},

		{
			name: "session_expired_success",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000"),
				},
				oss: &repository.MockOrder{
					FnGet: func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
						oid := uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000")

						if id != oid {
							return nil, model.Error("wrong order id")
						}

						ord := &model.Order{
							Metadata: datastore.Metadata{"radomCheckoutSessionId": "sesh_id"},
						}

						return ord, nil
					},
				},
				ois: &repository.MockOrderItem{
					FnFindByOrderID: func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) ([]model.OrderItem, error) {
						if orderID != uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000") {
							return nil, model.Error("wrong order id")
						}

						items := []model.OrderItem{
							{
								Metadata: datastore.Metadata{"radom_product_id": "product_id"},
							},
						}

						return items, nil
					},
				},
				rcl: &mockRadomClient{
					fnGetCheckoutSession: func(ctx context.Context, seshID string) (radom.GetCheckoutSessionResponse, error) {
						return radom.GetCheckoutSessionResponse{
							SessionStatus: "expired",
						}, nil
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{
				orderRepo:     tc.given.oss,
				orderItemRepo: tc.given.ois,
				payHistRepo:   tc.given.oph,
				radomClient:   tc.given.rcl,
			}

			ctx := context.Background()

			actual := svc.updateOrderRadomSession(ctx, nil, tc.given.ord)
			should.Equal(t, tc.exp.err, actual)
		})
	}
}

type mockRadomClient struct {
	fnCreateCheckoutSession func(ctx context.Context, creq *radom.CreateCheckoutSessionRequest) (radom.CreateCheckoutSessionResponse, error)
	fnGetCheckoutSession    func(ctx context.Context, seshID string) (radom.GetCheckoutSessionResponse, error)
	fnGetSubscription       func(ctx context.Context, subID string) (*radom.SubscriptionResponse, error)
}

func (m *mockRadomClient) CreateCheckoutSession(ctx context.Context, creq *radom.CreateCheckoutSessionRequest) (radom.CreateCheckoutSessionResponse, error) {
	if m.fnCreateCheckoutSession == nil {
		return radom.CreateCheckoutSessionResponse{}, nil
	}

	return m.fnCreateCheckoutSession(ctx, creq)
}

func (m *mockRadomClient) GetCheckoutSession(ctx context.Context, seshID string) (radom.GetCheckoutSessionResponse, error) {
	if m.fnGetCheckoutSession == nil {
		return radom.GetCheckoutSessionResponse{}, nil
	}

	return m.fnGetCheckoutSession(ctx, seshID)
}

func (m *mockRadomClient) GetSubscription(ctx context.Context, subID string) (*radom.SubscriptionResponse, error) {
	if m.fnGetSubscription == nil {
		return &radom.SubscriptionResponse{}, nil
	}

	return m.fnGetSubscription(ctx, subID)
}

type mockPaidOrderCreator struct {
	fnCreateOrderPremium        func(ctx context.Context, req *model.CreateOrderRequestNew, ordNew *model.OrderNew, items []model.OrderItem) (*model.Order, error)
	fnRenewOrderWithExpPaidTime func(ctx context.Context, id uuid.UUID, expt, paidt time.Time) error
	fnAppendOrderMetadata       func(ctx context.Context, oid uuid.UUID, mdata datastore.Metadata) error
}

func (s *mockPaidOrderCreator) createOrderPremium(ctx context.Context, req *model.CreateOrderRequestNew, ordNew *model.OrderNew, items []model.OrderItem) (*model.Order, error) {
	if s.fnCreateOrderPremium == nil {
		return &model.Order{}, nil
	}

	return s.fnCreateOrderPremium(ctx, req, ordNew, items)
}

func (s *mockPaidOrderCreator) renewOrderWithExpPaidTime(ctx context.Context, id uuid.UUID, expt, paidt time.Time) error {
	if s.fnRenewOrderWithExpPaidTime == nil {
		return nil
	}

	return s.fnRenewOrderWithExpPaidTime(ctx, id, expt, paidt)
}

func (s *mockPaidOrderCreator) appendOrderMetadata(ctx context.Context, oid uuid.UUID, mdata datastore.Metadata) error {
	if s.fnAppendOrderMetadata == nil {
		return nil
	}

	return s.fnAppendOrderMetadata(ctx, oid, mdata)
}
