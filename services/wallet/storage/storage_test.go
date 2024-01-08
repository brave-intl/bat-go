//go:build integration

package storage

import (
	"context"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/libs/datastore"
	"github.com/brave-intl/bat-go/services/wallet/model"
	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"
	must "github.com/stretchr/testify/require"
)

func TestChallenge_Get(t *testing.T) {
	dbi, err := setupDBI()
	must.NoError(t, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE_TABLE challenge;")
	}()

	type tcGiven struct {
		id   string
		chal model.Challenge
	}

	type exp struct {
		chal model.Challenge
		err  error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   exp
	}

	tests := []testCase{
		{
			name: "get",
			given: tcGiven{
				id: "1",
				chal: model.Challenge{
					ID:        "1",
					CreatedAt: time.Date(2024, 1, 1, 1, 1, 1, 0, time.UTC),
					Nonce:     "nonce-1",
				}},
			exp: exp{
				chal: model.Challenge{
					ID:        "1",
					CreatedAt: time.Date(2024, 1, 1, 1, 1, 1, 0, time.UTC),
					Nonce:     "nonce-1",
				},
				err: nil,
			},
		},
		{
			name: "not_found",
			given: tcGiven{
				id: "some-random-id",
				chal: model.Challenge{
					ID:        "2",
					CreatedAt: time.Date(2024, 1, 1, 1, 1, 1, 0, time.UTC),
					Nonce:     "nonce-2",
				}},
			exp: exp{
				err: model.ErrNotFound,
			},
		},
	}

	const q = `insert into challenge (id, created_at, nonce) values($1, $2, $3)`

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			_, err1 := dbi.ExecContext(ctx, q, tc.given.chal.ID, tc.given.chal.CreatedAt, tc.given.chal.Nonce)
			must.NoError(t, err1)

			c := Challenge{}
			actual, err2 := c.Get(ctx, dbi, tc.given.id)
			must.Equal(t, tc.exp.err, err2)

			must.Equal(t, tc.exp.chal.ID, actual.ID)
			must.Equal(t, tc.exp.chal.CreatedAt, actual.CreatedAt)
			must.Equal(t, tc.exp.chal.Nonce, actual.Nonce)
		})
	}
}

func TestChallenge_Upsert(t *testing.T) {
	dbi, err := setupDBI()
	must.NoError(t, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE_TABLE challenge;")
	}()

	const q = `select * from challenge where id = $1`

	chlRepo := &Challenge{}

	t.Run("insert", func(t *testing.T) {
		exp := model.Challenge{
			ID:        "1",
			CreatedAt: time.Date(2024, 1, 1, 1, 1, 1, 0, time.UTC),
			Nonce:     "a",
		}

		err1 := chlRepo.Upsert(context.TODO(), dbi, exp)
		must.NoError(t, err1)

		var actual model.Challenge
		err2 := sqlx.GetContext(context.TODO(), dbi, &actual, q, exp.ID)
		must.NoError(t, err2)

		must.Equal(t, exp.ID, actual.ID)
		must.Equal(t, exp.CreatedAt, actual.CreatedAt)
		must.Equal(t, exp.Nonce, actual.Nonce)
	})

	t.Run("upsert", func(t *testing.T) {
		ctx := context.Background()

		exp := model.Challenge{
			ID:        "1",
			CreatedAt: time.Date(2024, 12, 1, 1, 1, 1, 0, time.UTC),
			Nonce:     "b",
		}

		err1 := chlRepo.Upsert(ctx, dbi, exp)
		must.NoError(t, err1)

		var actual model.Challenge
		err2 := sqlx.GetContext(ctx, dbi, &actual, q, exp.ID)
		must.NoError(t, err2)

		must.Equal(t, exp.ID, actual.ID)
		must.Equal(t, exp.CreatedAt, actual.CreatedAt)
		must.Equal(t, exp.Nonce, actual.Nonce)
	})
}

func TestChallenge_Delete(t *testing.T) {
	dbi, err := setupDBI()
	must.NoError(t, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE_TABLE challenge;")
	}()

	type tcGiven struct {
		id   string
		chal model.Challenge
	}

	type exp struct {
		err error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   exp
	}

	tests := []testCase{
		{
			name: "delete",
			given: tcGiven{
				id: "1",
				chal: model.Challenge{
					ID:    "1",
					Nonce: "nonce-1",
				}},
			exp: exp{
				err: nil,
			},
		},
		{
			name: "no_rows_deleted",
			given: tcGiven{
				id: "some-random-id",
				chal: model.Challenge{
					ID:        "2",
					CreatedAt: time.Date(2024, 1, 1, 1, 1, 1, 0, time.UTC),
					Nonce:     "nonce-2",
				}},
			exp: exp{
				err: model.ErrNoRowsDeleted,
			},
		},
	}

	const q = `insert into challenge (id, created_at, nonce) values($1, $2, $3)`

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			_, err1 := dbi.ExecContext(ctx, q, tc.given.chal.ID, tc.given.chal.CreatedAt, tc.given.chal.Nonce)
			must.NoError(t, err1)

			c := Challenge{}
			err2 := c.Delete(ctx, dbi, tc.given.id)
			must.Equal(t, tc.exp.err, err2)
		})
	}
}

func TestAllowList_GetAllowListEntry(t *testing.T) {
	dbi, err := setupDBI()
	must.NoError(t, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE_TABLE allow_list;")
	}()

	type tcGiven struct {
		paymentID uuid.UUID
		allow     model.AllowListEntry
	}

	type exp struct {
		allow model.AllowListEntry
		err   error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   exp
	}

	tests := []testCase{
		{
			name: "success",
			given: tcGiven{
				paymentID: uuid.FromStringOrNil("6d85a314-0fa8-4594-9cb9-c9141b61a887"),
				allow: model.AllowListEntry{
					PaymentID: uuid.FromStringOrNil("6d85a314-0fa8-4594-9cb9-c9141b61a887"),
					CreatedAt: time.Date(2024, 1, 1, 1, 1, 1, 0, time.UTC),
				},
			},
			exp: exp{
				allow: model.AllowListEntry{
					PaymentID: uuid.FromStringOrNil("6d85a314-0fa8-4594-9cb9-c9141b61a887"),
					CreatedAt: time.Date(2024, 1, 1, 1, 1, 1, 0, time.UTC),
				},
				err: nil,
			},
		},
		{
			name: "not_found",
			given: tcGiven{
				paymentID: uuid.NewV4(),
			},
			exp: exp{
				err: model.ErrNotFound,
			},
		},
	}

	const q = `insert into allow_list (payment_id, created_at) values($1, $2)`

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			_, err1 := dbi.ExecContext(ctx, q, tt.given.allow.PaymentID, tt.given.allow.CreatedAt)
			must.NoError(t, err1)

			a := &AllowList{}
			actual, err2 := a.GetAllowListEntry(ctx, dbi, tt.given.paymentID)
			must.Equal(t, tt.exp.err, err2)
			must.Equal(t, tt.exp.allow, actual)
		})
	}
}

func setupDBI() (*sqlx.DB, error) {
	pg, err := datastore.NewPostgres("", false, "")
	if err != nil {
		return nil, err
	}

	mg, err := pg.NewMigrate()
	if err != nil {
		return nil, err
	}

	ver, dirty, err := mg.Version()
	if err != nil {
		return nil, err
	}

	if dirty {
		if err := mg.Force(int(ver)); err != nil {
			return nil, err
		}
	}

	if ver > 0 {
		if err := mg.Down(); err != nil {
			return nil, err
		}
	}

	if err := pg.Migrate(); err != nil {
		return nil, err
	}

	return pg.RawDB(), nil
}
