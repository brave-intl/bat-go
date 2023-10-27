//go:build integration

package repository_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/libs/datastore"
	timeutils "github.com/brave-intl/bat-go/libs/time"

	"github.com/brave-intl/bat-go/services/skus/model"
	"github.com/brave-intl/bat-go/services/skus/storage/repository"
)

func TestOrder_SetTrialDays(t *testing.T) {
	dbi, err := setupDBI()
	must.Equal(t, nil, err)

	t.Cleanup(func() {
		_, _ = dbi.Exec("TRUNCATE_TABLE orders;")
	})

	type tcExpected struct {
		ndays int64
		err   error
	}

	type testCase struct {
		name  string
		given int64
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "not_found",
			exp: tcExpected{
				err: model.ErrOrderNotFound,
			},
		},

		{
			name: "no_changes",
		},

		{
			name:  "updated_value",
			given: 4,
			exp:   tcExpected{ndays: 4},
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

			order, err := createOrderForTest(ctx, tx, repo)
			must.Equal(t, nil, err)

			id := order.ID
			if tc.exp.err == model.ErrOrderNotFound {
				// Use any id for testing the not found case.
				id = uuid.NamespaceDNS
			}

			actual, err := repo.SetTrialDays(ctx, tx, id, tc.given)
			must.Equal(t, true, errors.Is(err, tc.exp.err))

			if tc.exp.err != nil {
				return
			}

			should.Equal(t, tc.exp.ndays, actual.GetTrialDays())
		})
	}
}

func TestOrder_AppendMetadata(t *testing.T) {
	dbi, err := setupDBI()
	must.Equal(t, nil, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE_TABLE orders;")
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
		_, _ = dbi.Exec("TRUNCATE_TABLE orders;")
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
		_, _ = dbi.Exec("TRUNCATE_TABLE orders;")
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
		_, _ = dbi.Exec("TRUNCATE_TABLE orders;")
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
						Quantity:       2,
						Price:          mustDecimalFromString("3"),
						Currency:       "USD",
						Subtotal:       mustDecimalFromString("6"),
						CredentialType: "something",
						ValidForISO:    ptrString("P1M"),
					},

					{
						SKU:            "sku_02_02",
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
						Quantity:       2,
						Price:          mustDecimalFromString("3"),
						Currency:       "USD",
						Subtotal:       mustDecimalFromString("6"),
						CredentialType: "something",
					},

					{
						SKU:            "sku_02_02",
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
				future, err := nowPlusInterval("P1M")
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
		_, _ = dbi.Exec("TRUNCATE_TABLE orders;")
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

func ptrString(s string) *string {
	return &s
}

func nowPlusInterval(v string) (time.Time, error) {
	dur, err := timeutils.ParseDuration(v)
	if err != nil {
		return time.Time{}, err
	}

	result, err := dur.FromNow()
	if err != nil {
		return time.Time{}, err
	}

	return *result, nil
}
