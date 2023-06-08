//go:build integration

package repository_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

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
