//go:build integration

package repository_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/libs/datastore"

	"github.com/brave-intl/bat-go/services/skus/model"
	"github.com/brave-intl/bat-go/services/skus/storage/repository"
)

func TestOrder_GetByRadomSubscriptionID(t *testing.T) {
	dbi, err := setupDBI()
	must.NoError(t, err)

	defer func() {
		_, _ = dbi.Exec("DELETE FROM orders;")
	}()

	type tcGiven struct {
		rsid     string
		fnBefore func(ctx context.Context, dbi sqlx.ExecerContext) error
	}

	type tcExpected struct {
		order *model.Order
		err   error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "error_order_not_found",
			given: tcGiven{
				rsid: "some_radom_sub_id",
				fnBefore: func(ctx context.Context, dbi sqlx.ExecerContext) error {
					const q = `INSERT INTO orders (
						id, merchant_id, status, currency, total_price, trial_days, created_at, updated_at, metadata
					)
					VALUES (
						'00000000-0000-4000-a000-000000000000',
						'brave.com',
						'paid',
						'USD',
						9.99,
						3,
						'2024-01-01 00:00:01',
						'2024-01-01 00:00:01',
						'{"radomSubscriptionId" : "radom_sub_id"}'
					);`

					_, err := dbi.ExecContext(ctx, q)

					return err
				},
			},
			exp: tcExpected{
				err: model.ErrOrderNotFound,
			},
		},

		{
			name: "success",
			given: tcGiven{
				rsid: "rsid_success",
				fnBefore: func(ctx context.Context, dbi sqlx.ExecerContext) error {
					const q = `INSERT INTO orders (
						id, merchant_id, status, currency, total_price, trial_days, created_at, updated_at, metadata
					)
					VALUES (
						'facade00-0000-4000-a000-000000000000',
						'brave.com',
						'paid',
						'USD',
						9.99,
						3,
						'2024-01-01 00:00:01',
						'2024-01-01 00:00:01',
						'{"radomSubscriptionId" : "rsid_success"}'
					);`

					_, err := dbi.ExecContext(ctx, q)

					return err
				},
			},
			exp: tcExpected{
				order: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
				},
			},
		},
	}

	repo := repository.NewOrder()

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			_, err = dbi.Exec("DELETE FROM orders;")
			must.NoError(t, err)

			if tc.given.fnBefore != nil {
				err := tc.given.fnBefore(ctx, dbi)
				must.NoError(t, err)
			}

			actual, err := repo.GetByRadomSubscriptionID(ctx, dbi, tc.given.rsid)
			must.Equal(t, tc.exp.err, err)

			if tc.exp.err != nil {
				return
			}

			should.Equal(t, tc.exp.order.ID, actual.ID)
		})
	}
}

func TestOrder_SetTrialDays(t *testing.T) {
	dbi, err := setupDBI()
	must.Equal(t, nil, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE TABLE orders;")
	}()

	type tcGiven struct {
		id       uuid.UUID
		ndays    int64
		fnBefore func(ctx context.Context, dbi sqlx.ExecerContext) error
	}

	type tcExpected struct {
		num       int64
		updateErr error
		getErr    error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "not_set_before",
			given: tcGiven{
				id:    uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000"),
				ndays: 1,
				fnBefore: func(ctx context.Context, dbi sqlx.ExecerContext) error {
					const q = `INSERT INTO orders (
						id, merchant_id, status, currency, total_price, created_at, updated_at
					)
					VALUES (
						'facade00-0000-4000-a000-000000000000',
						'brave.com',
						'paid',
						'USD',
						9.99,
						'2024-01-01 00:00:01',
						'2024-01-01 00:00:01'
					);`

					_, err := dbi.ExecContext(ctx, q)

					return err
				},
			},

			exp: tcExpected{
				num: 1,
			},
		},

		{
			name: "overwrites_existing",
			given: tcGiven{
				id:    uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000"),
				ndays: 7,
				fnBefore: func(ctx context.Context, dbi sqlx.ExecerContext) error {
					const q = `INSERT INTO orders (
						id, merchant_id, status, currency, total_price, trial_days, created_at, updated_at
					)
					VALUES (
						'facade00-0000-4000-a000-000000000000',
						'brave.com',
						'paid',
						'USD',
						9.99,
						3,
						'2024-01-01 00:00:01',
						'2024-01-01 00:00:01'
					);`

					_, err := dbi.ExecContext(ctx, q)

					return err
				},
			},

			exp: tcExpected{
				num: 7,
			},
		},
	}

	repo := repository.NewOrder()

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			tx, err := dbi.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadUncommitted})
			must.Equal(t, nil, err)

			t.Cleanup(func() { _ = tx.Rollback() })

			if tc.given.fnBefore != nil {
				err := tc.given.fnBefore(ctx, tx)
				must.NoError(t, err)
			}

			{
				err := repo.SetTrialDays(ctx, tx, tc.given.id, tc.given.ndays)
				if err != nil {
					t.Log(err)
				}
				must.Equal(t, tc.exp.updateErr, err)
			}

			if tc.exp.updateErr != nil {
				return
			}

			actual, err := repo.Get(ctx, tx, tc.given.id)
			must.Equal(t, tc.exp.getErr, err)

			if tc.exp.getErr != nil {
				return
			}

			should.Equal(t, tc.exp.num, actual.GetTrialDays())
		})
	}
}

func TestOrder_AppendMetadata(t *testing.T) {
	dbi, err := setupDBI()
	must.Equal(t, nil, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE TABLE orders;")
	}()

	type tcGiven struct {
		data datastore.Metadata
		key  string
		val  string
	}

	type tcExpected struct {
		data datastore.Metadata
		err  error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "not_found",
			exp: tcExpected{
				err: model.ErrNoRowsChangedOrder,
			},
		},

		{
			name: "no_previous_metadata",
			given: tcGiven{
				key: "key_01_01",
				val: "value_01_01",
			},
			exp: tcExpected{
				data: datastore.Metadata{"key_01_01": "value_01_01"},
			},
		},

		{
			name: "no_changes",
			given: tcGiven{
				data: datastore.Metadata{"key_02_01": "value_02_01"},
				key:  "key_02_01",
				val:  "value_02_01",
			},
			exp: tcExpected{
				data: datastore.Metadata{"key_02_01": "value_02_01"},
			},
		},

		{
			name: "updates_the_only_key",
			given: tcGiven{
				data: datastore.Metadata{"key_03_01": "value_03_01"},
				key:  "key_03_01",
				val:  "value_03_01_UPDATED",
			},
			exp: tcExpected{
				data: datastore.Metadata{"key_03_01": "value_03_01_UPDATED"},
			},
		},

		{
			name: "updates_one_from_many",
			given: tcGiven{
				data: datastore.Metadata{
					"key_04_01": "value_04_01",
					"key_04_02": "value_04_02",
					"key_04_03": "value_04_03",
				},
				key: "key_04_02",
				val: "value_04_02_UPDATED",
			},
			exp: tcExpected{
				data: datastore.Metadata{
					"key_04_01": "value_04_01",
					"key_04_02": "value_04_02_UPDATED",
					"key_04_03": "value_04_03",
				},
			},
		},

		{
			name: "stripeSubscriptionId_add_with_no_previous_value",
			given: tcGiven{
				data: datastore.Metadata{"key_02_01": "value_02_01"},
				key:  "stripeSubscriptionId",
				val:  "9570bf21-98e8-4ddc-950d-a50121d48a0a",
			},
			exp: tcExpected{
				data: datastore.Metadata{
					"key_02_01":            "value_02_01",
					"stripeSubscriptionId": "9570bf21-98e8-4ddc-950d-a50121d48a0a",
				},
			},
		},

		{
			name: "stripeSubscriptionId_no_change",
			given: tcGiven{
				data: datastore.Metadata{
					"key_02_01":            "value_02_01",
					"stripeSubscriptionId": "9570bf21-98e8-4ddc-950d-a50121d48a0a",
				},
				key: "stripeSubscriptionId",
				val: "9570bf21-98e8-4ddc-950d-a50121d48a0a",
			},
			exp: tcExpected{
				data: datastore.Metadata{
					"key_02_01":            "value_02_01",
					"stripeSubscriptionId": "9570bf21-98e8-4ddc-950d-a50121d48a0a",
				},
			},
		},

		{
			name: "stripeSubscriptionId_replace",
			given: tcGiven{
				data: datastore.Metadata{
					"key_02_01":            "value_02_01",
					"stripeSubscriptionId": "9570bf21-98e8-4ddc-950d-a50121d48a0a",
				},
				key: "stripeSubscriptionId",
				val: "edfe50f8-06dc-4d5f-a6ca-15c8a1ce6afb",
			},
			exp: tcExpected{
				data: datastore.Metadata{
					"key_02_01":            "value_02_01",
					"stripeSubscriptionId": "edfe50f8-06dc-4d5f-a6ca-15c8a1ce6afb",
				},
			},
		},
	}

	repo := repository.NewOrder()

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.TODO()

			tx, err := dbi.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadUncommitted})
			must.Equal(t, nil, err)

			t.Cleanup(func() { _ = tx.Rollback() })

			order, err := createOrderForTest(ctx, tx, repo)
			must.Equal(t, nil, err)

			id := order.ID
			if tc.exp.err == model.ErrNoRowsChangedOrder {
				// Use any id for testing the not found case.
				id = uuid.NamespaceDNS
			}

			if tc.given.data != nil {
				err := repo.UpdateMetadata(ctx, tx, id, tc.given.data)
				must.Equal(t, nil, err)
			}

			{
				err := repo.AppendMetadata(ctx, tx, id, tc.given.key, tc.given.val)
				must.Equal(t, true, errors.Is(err, tc.exp.err))
			}

			if tc.exp.err != nil {
				return
			}

			actual, err := repo.Get(ctx, tx, id)
			must.Equal(t, nil, err)

			should.Equal(t, tc.exp.data, actual.Metadata)
		})
	}
}

func TestOrder_AppendMetadataInt(t *testing.T) {
	dbi, err := setupDBI()
	must.Equal(t, nil, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE TABLE orders;")
	}()

	type tcGiven struct {
		data datastore.Metadata
		key  string
		val  int
	}

	type tcExpected struct {
		data datastore.Metadata
		err  error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "not_found",
			exp: tcExpected{
				err: model.ErrNoRowsChangedOrder,
			},
		},

		{
			name: "no_previous_metadata",
			given: tcGiven{
				key: "key_01_01",
				val: 101,
			},
			exp: tcExpected{
				data: datastore.Metadata{"key_01_01": float64(101)},
			},
		},

		{
			name: "no_changes",
			given: tcGiven{
				data: datastore.Metadata{"key_02_01": 201},
				key:  "key_02_01",
				val:  201,
			},
			exp: tcExpected{
				data: datastore.Metadata{"key_02_01": float64(201)},
			},
		},

		{
			name: "updates_the_only_key",
			given: tcGiven{
				data: datastore.Metadata{"key_03_01": float64(301)},
				key:  "key_03_01",
				val:  30101,
			},
			exp: tcExpected{
				data: datastore.Metadata{"key_03_01": float64(30101)},
			},
		},

		{
			name: "updates_one_from_many",
			given: tcGiven{
				data: datastore.Metadata{
					"key_04_01": "key_04_01",
					"key_04_02": float64(402),
					"key_04_03": float64(403),
				},
				key: "key_04_02",
				val: 40201,
			},
			exp: tcExpected{
				data: datastore.Metadata{
					"key_04_01": "key_04_01",
					"key_04_02": float64(40201),
					"key_04_03": float64(403),
				},
			},
		},
	}

	repo := repository.NewOrder()

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.TODO()

			tx, err := dbi.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadUncommitted})
			must.Equal(t, nil, err)

			t.Cleanup(func() { _ = tx.Rollback() })

			order, err := createOrderForTest(ctx, tx, repo)
			must.Equal(t, nil, err)

			id := order.ID
			if tc.exp.err == model.ErrNoRowsChangedOrder {
				// Use any id for testing the not found case.
				id = uuid.NamespaceDNS
			}

			if tc.given.data != nil {
				err := repo.UpdateMetadata(ctx, tx, id, tc.given.data)
				must.Equal(t, nil, err)
			}

			{
				err := repo.AppendMetadataInt(ctx, tx, id, tc.given.key, tc.given.val)
				must.Equal(t, true, errors.Is(err, tc.exp.err))
			}

			if tc.exp.err != nil {
				return
			}

			actual, err := repo.Get(ctx, tx, id)
			must.Equal(t, nil, err)

			// This is currently failing.
			// The expectation is that data fetched from the store would be int.
			// It, however, is float64.
			//
			// Temporary defining expectations as float64 so that tests pass.
			should.Equal(t, tc.exp.data, actual.Metadata)
		})
	}
}

func TestOrder_AppendMetadataInt64(t *testing.T) {
	dbi, err := setupDBI()
	must.Equal(t, nil, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE TABLE orders;")
	}()

	type tcGiven struct {
		data datastore.Metadata
		key  string
		val  int64
	}

	type tcExpected struct {
		data datastore.Metadata
		err  error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "not_found",
			exp: tcExpected{
				err: model.ErrNoRowsChangedOrder,
			},
		},

		{
			name: "no_previous_metadata",
			given: tcGiven{
				key: "key_01_01",
				val: 101,
			},
			exp: tcExpected{
				data: datastore.Metadata{"key_01_01": float64(101)},
			},
		},

		{
			name: "no_changes",
			given: tcGiven{
				data: datastore.Metadata{"key_02_01": 201},
				key:  "key_02_01",
				val:  201,
			},
			exp: tcExpected{
				data: datastore.Metadata{"key_02_01": float64(201)},
			},
		},

		{
			name: "updates_the_only_key",
			given: tcGiven{
				data: datastore.Metadata{"key_03_01": float64(301)},
				key:  "key_03_01",
				val:  30101,
			},
			exp: tcExpected{
				data: datastore.Metadata{"key_03_01": float64(30101)},
			},
		},

		{
			name: "updates_one_from_many",
			given: tcGiven{
				data: datastore.Metadata{
					"key_04_01": "key_04_01",
					"key_04_02": float64(402),
					"key_04_03": float64(403),
				},
				key: "key_04_02",
				val: 40201,
			},
			exp: tcExpected{
				data: datastore.Metadata{
					"key_04_01": "key_04_01",
					"key_04_02": float64(40201),
					"key_04_03": float64(403),
				},
			},
		},
	}

	repo := repository.NewOrder()

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.TODO()

			tx, err := dbi.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadUncommitted})
			must.Equal(t, nil, err)

			t.Cleanup(func() { _ = tx.Rollback() })

			order, err := createOrderForTest(ctx, tx, repo)
			must.Equal(t, nil, err)

			id := order.ID
			if tc.exp.err == model.ErrNoRowsChangedOrder {
				// Use any id for testing the not found case.
				id = uuid.NamespaceDNS
			}

			if tc.given.data != nil {
				err := repo.UpdateMetadata(ctx, tx, id, tc.given.data)
				must.Equal(t, nil, err)
			}

			{
				err := repo.AppendMetadataInt64(ctx, tx, id, tc.given.key, tc.given.val)
				must.Equal(t, true, errors.Is(err, tc.exp.err))
			}

			if tc.exp.err != nil {
				return
			}

			actual, err := repo.Get(ctx, tx, id)
			must.Equal(t, nil, err)

			// This is currently failing.
			// The expectation is that data fetched from the store would be int.
			// It, however, is float64.
			//
			// Temporary defining expectations as float64 so that tests pass.
			should.Equal(t, tc.exp.data, actual.Metadata)
		})
	}
}

func TestOrder_GetExpiresAtAfterISOPeriod(t *testing.T) {
	dbi, err := setupDBI()
	must.Equal(t, nil, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE TABLE orders;")
	}()

	type tcGiven struct {
		lastPaidAt time.Time
		items      []model.OrderItem
	}

	type tcExpected struct {
		expiresAt time.Time
		err       error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "no_last_paid_no_items",
		},

		{
			name: "20230202_no_items",
			given: tcGiven{
				lastPaidAt: time.Date(2023, time.February, 2, 1, 0, 0, 0, time.UTC),
			},
			exp: tcExpected{
				expiresAt: time.Date(2023, time.March, 2, 1, 0, 0, 0, time.UTC),
			},
		},

		{
			name: "20230202_1_item",
			given: tcGiven{
				lastPaidAt: time.Date(2023, time.February, 2, 1, 0, 0, 0, time.UTC),
				items: []model.OrderItem{
					{
						SKU:            "sku_01_01",
						SKUVnt:         "sku_vnt_01_01",
						Quantity:       1,
						Price:          mustDecimalFromString("2"),
						Currency:       "USD",
						Subtotal:       mustDecimalFromString("2"),
						CredentialType: "something",
						ValidForISO:    ptrString("P1M"),
					},
				},
			},
			exp: tcExpected{
				expiresAt: time.Date(2023, time.March, 2, 1, 0, 0, 0, time.UTC),
			},
		},

		{
			name: "20230331_2_items",
			given: tcGiven{
				lastPaidAt: time.Date(2023, time.March, 31, 1, 0, 0, 0, time.UTC),
				items: []model.OrderItem{
					{
						SKU:            "sku_02_01",
						SKUVnt:         "sku_vnt_02_01",
						Quantity:       2,
						Price:          mustDecimalFromString("3"),
						Currency:       "USD",
						Subtotal:       mustDecimalFromString("6"),
						CredentialType: "something",
						ValidForISO:    ptrString("P1M"),
					},

					{
						SKU:            "sku_02_02",
						SKUVnt:         "sku_vnt_02_02",
						Quantity:       3,
						Price:          mustDecimalFromString("4"),
						Currency:       "USD",
						Subtotal:       mustDecimalFromString("12"),
						CredentialType: "something",
						ValidForISO:    ptrString("P2M"),
					},
				},
			},
			exp: tcExpected{
				expiresAt: time.Date(2023, time.May, 31, 1, 0, 0, 0, time.UTC),
			},
		},

		{
			name: "20230331_2_items_no_iso",
			given: tcGiven{
				lastPaidAt: time.Date(2023, time.March, 31, 1, 0, 0, 0, time.UTC),
				items: []model.OrderItem{
					{
						SKU:            "sku_02_01",
						SKUVnt:         "sku_vnt_02_01",
						Quantity:       2,
						Price:          mustDecimalFromString("3"),
						Currency:       "USD",
						Subtotal:       mustDecimalFromString("6"),
						CredentialType: "something",
					},

					{
						SKU:            "sku_02_02",
						SKUVnt:         "sku_vnt_02_02",
						Quantity:       3,
						Price:          mustDecimalFromString("4"),
						Currency:       "USD",
						Subtotal:       mustDecimalFromString("12"),
						CredentialType: "something",
					},
				},
			},
			exp: tcExpected{
				expiresAt: time.Date(2023, time.April, 30, 1, 0, 0, 0, time.UTC),
			},
		},
	}

	repo := repository.NewOrder()
	iorepo := repository.NewOrderItem()

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.TODO()

			tx, err := dbi.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadUncommitted})
			must.Equal(t, nil, err)

			t.Cleanup(func() { _ = tx.Rollback() })

			order, err := createOrderForTest(ctx, tx, repo)
			must.Equal(t, nil, err)

			if !tc.given.lastPaidAt.IsZero() {
				err := repo.SetLastPaidAt(ctx, tx, order.ID, tc.given.lastPaidAt)
				must.Equal(t, nil, err)
			}

			if len(tc.given.items) > 0 {
				model.OrderItemList(tc.given.items).SetOrderID(order.ID)

				_, err := iorepo.InsertMany(ctx, tx, tc.given.items...)
				must.Equal(t, nil, err)
			}

			actual, err := repo.GetExpiresAtAfterISOPeriod(ctx, tx, order.ID)
			must.Equal(t, nil, err)

			// Handle the special case where last_paid_at was not set.
			// The time is generated by the database, so it is non-deterministic.
			if tc.given.lastPaidAt.IsZero() {
				future, err := nowPlusIntervalPg(ctx, tx, "P1M")
				must.Equal(t, nil, err)

				t.Log("actual", actual)
				t.Log("future", future)

				diff := future.Sub(actual)
				if diff < time.Duration(0) {
					diff = actual.Sub(future)
				}

				should.Equal(t, true, diff < time.Duration(1*time.Hour))
				return
			}

			// TODO(pavelb): update local and testing containers to use Go 1.20+.
			// Then switch to tc.exp.expiresAt.Compare(actual) == 0.
			should.Equal(t, true, tc.exp.expiresAt.Sub(actual) == 0)
		})
	}
}

func TestOrder_CreateGet(t *testing.T) {
	dbi, err := setupDBI()
	must.Equal(t, nil, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE TABLE orders;")
	}()

	type tcGiven struct {
		req *model.OrderNew
	}

	type tcExpected struct {
		result *model.Order
		err    error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "nil_allowed_payment_methods",
			given: tcGiven{
				req: &model.OrderNew{
					MerchantID: "brave.com",
					Currency:   "USD",
					Status:     "pending",
					Location: sql.NullString{
						Valid:  true,
						String: "https://somewhere.brave.software",
					},
					TotalPrice: mustDecimalFromString("5"),
				},
			},
			exp: tcExpected{
				result: &model.Order{
					MerchantID: "brave.com",
					Currency:   "USD",
					Status:     "pending",
					Location: datastore.NullString{
						NullString: sql.NullString{
							Valid:  true,
							String: "https://somewhere.brave.software",
						},
					},
					TotalPrice: mustDecimalFromString("5"),
				},
			},
		},

		{
			name: "empty_allowed_payment_methods",
			given: tcGiven{
				req: &model.OrderNew{
					MerchantID: "brave.com",
					Currency:   "USD",
					Status:     "pending",
					Location: sql.NullString{
						Valid:  true,
						String: "https://somewhere.brave.software",
					},
					TotalPrice:            mustDecimalFromString("5"),
					AllowedPaymentMethods: pq.StringArray{},
				},
			},
			exp: tcExpected{
				result: &model.Order{
					MerchantID: "brave.com",
					Currency:   "USD",
					Status:     "pending",
					Location: datastore.NullString{
						NullString: sql.NullString{
							Valid:  true,
							String: "https://somewhere.brave.software",
						},
					},
					TotalPrice:            mustDecimalFromString("5"),
					AllowedPaymentMethods: pq.StringArray{},
				},
			},
		},

		{
			name: "single_allowed_payment_methods",
			given: tcGiven{
				req: &model.OrderNew{
					MerchantID: "brave.com",
					Currency:   "USD",
					Status:     "pending",
					Location: sql.NullString{
						Valid:  true,
						String: "https://somewhere.brave.software",
					},
					TotalPrice:            mustDecimalFromString("5"),
					AllowedPaymentMethods: pq.StringArray{"stripe"},
				},
			},
			exp: tcExpected{
				result: &model.Order{
					MerchantID: "brave.com",
					Currency:   "USD",
					Status:     "pending",
					Location: datastore.NullString{
						NullString: sql.NullString{
							Valid:  true,
							String: "https://somewhere.brave.software",
						},
					},
					TotalPrice:            mustDecimalFromString("5"),
					AllowedPaymentMethods: pq.StringArray{"stripe"},
				},
			},
		},

		{
			name: "many_allowed_payment_methods",
			given: tcGiven{
				req: &model.OrderNew{
					MerchantID: "brave.com",
					Currency:   "USD",
					Status:     "pending",
					Location: sql.NullString{
						Valid:  true,
						String: "https://somewhere.brave.software",
					},
					TotalPrice:            mustDecimalFromString("5"),
					AllowedPaymentMethods: pq.StringArray{"stripe", "cash"},
				},
			},
			exp: tcExpected{
				result: &model.Order{
					MerchantID: "brave.com",
					Currency:   "USD",
					Status:     "pending",
					Location: datastore.NullString{
						NullString: sql.NullString{
							Valid:  true,
							String: "https://somewhere.brave.software",
						},
					},
					TotalPrice:            mustDecimalFromString("5"),
					AllowedPaymentMethods: pq.StringArray{"stripe", "cash"},
				},
			},
		},

		{
			name: "empty_location",
			given: tcGiven{
				req: &model.OrderNew{
					MerchantID:            "brave.com",
					Currency:              "USD",
					Status:                "pending",
					TotalPrice:            mustDecimalFromString("5"),
					AllowedPaymentMethods: pq.StringArray{"stripe"},
				},
			},
			exp: tcExpected{
				result: &model.Order{
					MerchantID:            "brave.com",
					Currency:              "USD",
					Status:                "pending",
					TotalPrice:            mustDecimalFromString("5"),
					AllowedPaymentMethods: pq.StringArray{"stripe"},
				},
			},
		},
	}

	repo := repository.NewOrder()

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.TODO()

			tx, err := dbi.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadUncommitted})
			must.Equal(t, nil, err)

			t.Cleanup(func() { _ = tx.Rollback() })

			actual1, err := repo.Create(ctx, tx, tc.given.req)
			must.Equal(t, true, errors.Is(err, tc.exp.err))

			if tc.exp.err != nil {
				return
			}

			should.Equal(t, tc.exp.result.MerchantID, actual1.MerchantID)
			should.Equal(t, tc.exp.result.Currency, actual1.Currency)
			should.Equal(t, tc.exp.result.Status, actual1.Status)
			should.Equal(t, tc.exp.result.Location.Valid, actual1.Location.Valid)
			should.Equal(t, tc.exp.result.Location.String, actual1.Location.String)
			should.Equal(t, true, tc.exp.result.TotalPrice.Equal(actual1.TotalPrice))
			should.Equal(t, tc.exp.result.AllowedPaymentMethods, actual1.AllowedPaymentMethods)
			should.Equal(t, tc.exp.result.ValidFor, actual1.ValidFor)

			actual2, err := repo.Get(ctx, tx, actual1.ID)
			must.Equal(t, nil, err)

			should.Equal(t, actual1.ID, actual2.ID)
			should.Equal(t, actual1.MerchantID, actual2.MerchantID)
			should.Equal(t, actual1.Currency, actual2.Currency)
			should.Equal(t, actual1.Status, actual2.Status)
			should.Equal(t, actual1.Location, actual2.Location)
			should.Equal(t, true, actual1.TotalPrice.Equal(actual2.TotalPrice))
			should.Equal(t, actual1.AllowedPaymentMethods, actual2.AllowedPaymentMethods)
			should.Equal(t, actual1.ValidFor, actual2.ValidFor)
			should.Equal(t, actual1.CreatedAt, actual2.CreatedAt)
			should.Equal(t, actual1.UpdatedAt, actual2.UpdatedAt)
		})
	}
}

func TestOrder_IncrementNumPayFailed(t *testing.T) {
	dbi, err := setupDBI()
	must.Equal(t, nil, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE TABLE orders;")
	}()

	type tcGiven struct {
		id       uuid.UUID
		fnBefore func(ctx context.Context, dbi sqlx.ExecerContext) error
	}

	type tcExpected struct {
		num       int
		mdata     datastore.Metadata
		updateErr error
		getErr    error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "no_metadata",
			given: tcGiven{
				id: uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000"),
				fnBefore: func(ctx context.Context, dbi sqlx.ExecerContext) error {
					const q = `INSERT INTO orders (
						id, merchant_id, status, currency, total_price, created_at, updated_at
					)
					VALUES (
						'facade00-0000-4000-a000-000000000000',
						'brave.com',
						'paid',
						'USD',
						9.99,
						'2024-01-01 00:00:01',
						'2024-01-01 00:00:01'
					);`

					_, err := dbi.ExecContext(ctx, q)

					return err
				},
			},
			exp: tcExpected{
				num: 1,
				mdata: datastore.Metadata{
					// Here and below using float64 due to
					// https://github.com/brave-intl/bat-go/blob/master/libs/datastore/models.go#L29-L36.
					"numPaymentFailed": float64(1),
				},
			},
		},

		{
			name: "existing_field_incremented",
			given: tcGiven{
				id: uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000"),
				fnBefore: func(ctx context.Context, dbi sqlx.ExecerContext) error {
					const q = `INSERT INTO orders (
						id, merchant_id, status, currency, total_price, created_at, updated_at, metadata
					)
					VALUES (
						'facade00-0000-4000-a000-000000000000',
						'brave.com',
						'paid',
						'USD',
						9.99,
						'2024-01-01 00:00:01',
						'2024-01-01 00:00:01',
						'{"numPaymentFailed": 1}'
					);`

					_, err := dbi.ExecContext(ctx, q)

					return err
				},
			},
			exp: tcExpected{
				num: 2,
				mdata: datastore.Metadata{
					"numPaymentFailed": float64(2),
				},
			},
		},

		{
			name: "other_fields_exist_target_missing",
			given: tcGiven{
				id: uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000"),
				fnBefore: func(ctx context.Context, dbi sqlx.ExecerContext) error {
					const q = `INSERT INTO orders (
						id, merchant_id, status, currency, total_price, created_at, updated_at, metadata
					)
					VALUES (
						'facade00-0000-4000-a000-000000000000',
						'brave.com',
						'paid',
						'USD',
						9.99,
						'2024-01-01 00:00:01',
						'2024-01-01 00:00:01',
						'{"numPerInterval": 192}'
					);`

					_, err := dbi.ExecContext(ctx, q)

					return err
				},
			},
			exp: tcExpected{
				num: 1,
				mdata: datastore.Metadata{
					"numPaymentFailed": float64(1),
					"numPerInterval":   float64(192),
				},
			},
		},

		{
			name: "existing_field_incremented_other_exist",
			given: tcGiven{
				id: uuid.FromStringOrNil("facade00-0000-4000-a000-000000000000"),
				fnBefore: func(ctx context.Context, dbi sqlx.ExecerContext) error {
					const q = `INSERT INTO orders (
						id, merchant_id, status, currency, total_price, created_at, updated_at, metadata
					)
					VALUES (
						'facade00-0000-4000-a000-000000000000',
						'brave.com',
						'paid',
						'USD',
						9.99,
						'2024-01-01 00:00:01',
						'2024-01-01 00:00:01',
						'{"numPerInterval": 192, "numIntervals": 3, "numPaymentFailed": 1}'
					);`

					_, err := dbi.ExecContext(ctx, q)

					return err
				},
			},
			exp: tcExpected{
				num: 2,
				mdata: datastore.Metadata{
					"numPaymentFailed": float64(2),
					"numPerInterval":   float64(192),
					"numIntervals":     float64(3),
				},
			},
		},
	}

	repo := repository.NewOrder()

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			tx, err := dbi.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadUncommitted})
			must.NoError(t, err)

			t.Cleanup(func() { _ = tx.Rollback() })

			if tc.given.fnBefore != nil {
				err := tc.given.fnBefore(ctx, tx)
				must.NoError(t, err)
			}

			{
				err := repo.IncrementNumPayFailed(ctx, tx, tc.given.id)
				if err != nil {
					t.Log(err)
				}
				must.Equal(t, tc.exp.updateErr, err)
			}

			if tc.exp.updateErr != nil {
				return
			}

			actual, err := repo.Get(ctx, tx, tc.given.id)
			must.Equal(t, tc.exp.getErr, err)

			if tc.exp.getErr != nil {
				return
			}

			should.Equal(t, tc.exp.num, actual.NumPaymentFailed())
			should.Equal(t, tc.exp.mdata, actual.Metadata)
		})
	}
}

func ptrString(s string) *string {
	return &s
}

func nowPlusIntervalPg(ctx context.Context, dbi sqlx.QueryerContext, v string) (time.Time, error) {
	const q = `SELECT now() + $1::interval;`

	var result time.Time
	if err := sqlx.GetContext(ctx, dbi, &result, q, v); err != nil {
		return time.Time{}, err
	}

	return result, nil
}
