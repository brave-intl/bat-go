package skus

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
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

	"github.com/brave-intl/bat-go/libs/datastore"

	"github.com/brave-intl/bat-go/services/skus/model"
	"github.com/brave-intl/bat-go/services/skus/storage/repository"
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
				orepo: &repository.MockOrder{},
				prepo: &repository.MockOrderPayHistory{},
				pscl:  &mockPSClient{},
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
		txn   *appstore.JWSTransactionDecodedPayload
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
				txn: &appstore.JWSTransactionDecodedPayload{OriginalTransactionId: "123456789000001"},
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
				txn: &appstore.JWSTransactionDecodedPayload{
					OriginalTransactionId: "123456789000001",
					ExpiresDate:           1704067201000,
				},

				orepo: &repository.MockOrder{},
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
				txn: &appstore.JWSTransactionDecodedPayload{
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
				txn: &appstore.JWSTransactionDecodedPayload{
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
				set:   newOrderItemReqNewMobileSet("development"),
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
					fnCreateOrderPremium: func(ctx context.Context, req *model.CreateOrderRequestNew, ordNew *model.OrderNew, items []model.OrderItem) (*model.Order, error) {
						return nil, model.Error("something went wrong")
					},
				},
				set:   newOrderItemReqNewMobileSet("development"),
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
					fnCreateOrderPremium: func(ctx context.Context, req *model.CreateOrderRequestNew, ordNew *model.OrderNew, items []model.OrderItem) (*model.Order, error) {
						return &model.Order{}, nil
					},

					fnUpdateOrderStatusPaidWithMetadata: func(ctx context.Context, oid *uuid.UUID, mdata datastore.Metadata) error {
						return model.Error("something went wrong")
					},
				},
				set:   newOrderItemReqNewMobileSet("development"),
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
				},
				set:   newOrderItemReqNewMobileSet("development"),
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
				},
				set:   newOrderItemReqNewMobileSet("development"),
				ppcfg: newPaymentProcessorConfig("development"),
				req: model.ReceiptRequest{
					Type:           model.VendorGoogle,
					Blob:           "blob",
					Package:        "package",
					SubscriptionID: "brave.vpn.monthly",
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

func TestService_doTLV2ExistTxTime(t *testing.T) {
	type tcGiven struct {
		reqID uuid.UUID
		item  *model.OrderItem
		creds []string
		from  time.Time
		to    time.Time
		repo  *repository.MockTLV2
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
				creds: []string{"cred_01", "cred_02"},
				from:  time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				to:    time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				repo:  &repository.MockTLV2{},
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
				creds: []string{"cred_01", "cred_02"},
				from:  time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				to:    time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				repo: &repository.MockTLV2{
					FnGetCredSubmissionReport: func(ctx context.Context, dbi sqlx.QueryerContext, reqID uuid.UUID, creds ...string) (model.TLV2CredSubmissionReport, error) {
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
				creds: []string{"cred_01", "cred_02"},
				from:  time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				to:    time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				repo: &repository.MockTLV2{
					FnGetCredSubmissionReport: func(ctx context.Context, dbi sqlx.QueryerContext, reqID uuid.UUID, creds ...string) (model.TLV2CredSubmissionReport, error) {
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
				creds: []string{"cred_01", "cred_02"},
				from:  time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				to:    time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				repo: &repository.MockTLV2{
					FnGetCredSubmissionReport: func(ctx context.Context, dbi sqlx.QueryerContext, reqID uuid.UUID, creds ...string) (model.TLV2CredSubmissionReport, error) {
						return model.TLV2CredSubmissionReport{ReqIDMistmatch: true}, nil
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
				creds: []string{"cred_01", "cred_02"},
				from:  time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				to:    time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
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
				creds: []string{"cred_01", "cred_02"},
				from:  time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				to:    time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
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
				creds: []string{"cred_01", "cred_02"},
				from:  time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				to:    time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
				repo:  &repository.MockTLV2{},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{tlv2Repo: tc.given.repo}

			ctx := context.Background()

			actual := svc.doTLV2ExistTxTime(ctx, nil, tc.given.reqID, tc.given.item, tc.given.creds, tc.given.from, tc.given.to)
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

type mockPaidOrderCreator struct {
	fnCreateOrderPremium                func(ctx context.Context, req *model.CreateOrderRequestNew, ordNew *model.OrderNew, items []model.OrderItem) (*model.Order, error)
	fnRenewOrderWithExpPaidTime         func(ctx context.Context, id uuid.UUID, expt, paidt time.Time) error
	fnAppendOrderMetadata               func(ctx context.Context, oid uuid.UUID, mdata datastore.Metadata) error
	fnUpdateOrderStatusPaidWithMetadata func(ctx context.Context, oid *uuid.UUID, mdata datastore.Metadata) error
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

func (s *mockPaidOrderCreator) updateOrderStatusPaidWithMetadata(ctx context.Context, oid *uuid.UUID, mdata datastore.Metadata) error {
	if s.fnUpdateOrderStatusPaidWithMetadata == nil {
		return nil
	}

	return s.fnUpdateOrderStatusPaidWithMetadata(ctx, oid, mdata)
}
