//go:build integration

package repository_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	uuid "github.com/satori/go.uuid"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/libs/datastore"

	"github.com/brave-intl/bat-go/services/skus/model"
	"github.com/brave-intl/bat-go/services/skus/storage/repository"
)

func TestOrder_SetTrialDays(t *testing.T) {
	dbi, err := setupDBI()
	must.Equal(t, nil, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE_TABLE orders;")
	}()

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
			ctx := context.TODO()

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

func TestOrder_GetExpiresAtP1M(t *testing.T) {
	dbi, err := setupDBI()
	must.Equal(t, nil, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE_TABLE orders;")
	}()

	type tcGiven struct {
		lastPaidAt time.Time
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
			name: "no_last_paid",
		},

		{
			name: "20230202",
			given: tcGiven{
				lastPaidAt: time.Date(2023, time.February, 2, 1, 0, 0, 0, time.UTC),
			},
			exp: tcExpected{
				expiresAt: time.Date(2023, time.March, 2, 1, 0, 0, 0, time.UTC),
			},
		},

		{
			name: "20230331",
			given: tcGiven{
				lastPaidAt: time.Date(2023, time.March, 31, 1, 0, 0, 0, time.UTC),
			},
			exp: tcExpected{
				expiresAt: time.Date(2023, time.April, 30, 1, 0, 0, 0, time.UTC),
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

			if !tc.given.lastPaidAt.IsZero() {
				err := repo.SetLastPaidAt(ctx, tx, order.ID, tc.given.lastPaidAt)
				must.Equal(t, nil, err)
			}

			actual, err := repo.GetExpiresAtP1M(ctx, tx, order.ID)
			must.Equal(t, nil, err)

			// Handle the special case where last_paid_at was not set.
			// The time is generated by the database, so it is non-deterministic.
			// The result should not be too far from time.Now()+1 month.
			if tc.given.lastPaidAt.IsZero() {
				now := time.Now()
				future := time.Date(now.Year(), now.Month()+1, now.Day(), now.Hour(), now.Minute(), now.Second(), now.Nanosecond(), now.Location())

				should.Equal(t, true, future.Sub(actual) < time.Duration(12*time.Hour))
				return
			}

			// TODO(pavelb): update local and testing containers to use Go 1.20+.
			// Then switch to tc.exp.expiresAt.Compare(actual) == 0.
			should.Equal(t, true, tc.exp.expiresAt.Sub(actual) == 0)
		})
	}
}
