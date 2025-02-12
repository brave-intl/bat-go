//go:build integration

package storage

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/libs/datastore"
	"github.com/brave-intl/bat-go/services/wallet/model"
)

func TestChallenge_Get(t *testing.T) {
	dbi, err := setupDBI()
	must.NoError(t, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE TABLE challenge;")
	}()

	type tcGiven struct {
		paymentID uuid.UUID
		chal      model.Challenge
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
				paymentID: uuid.FromStringOrNil("f6d43f0c-db24-4d65-9f02-b000ff8ec782"),
				chal: model.Challenge{
					PaymentID: uuid.FromStringOrNil("f6d43f0c-db24-4d65-9f02-b000ff8ec782"),
					CreatedAt: time.Date(2024, 1, 1, 1, 1, 1, 0, time.UTC),
					Nonce:     "nonce-1",
				}},
			exp: exp{
				chal: model.Challenge{
					PaymentID: uuid.FromStringOrNil("f6d43f0c-db24-4d65-9f02-b000ff8ec782"),
					CreatedAt: time.Date(2024, 1, 1, 1, 1, 1, 0, time.UTC),
					Nonce:     "nonce-1",
				},
				err: nil,
			},
		},
		{
			name: "challenge_not_found",
			given: tcGiven{
				paymentID: uuid.FromStringOrNil("1b8c218f-2585-49c1-90cd-b82006eb9865"),
				chal: model.Challenge{
					PaymentID: uuid.FromStringOrNil("54e4a78e-2c69-4fb2-8d72-47921bb0b374"),
					CreatedAt: time.Date(2024, 1, 1, 1, 1, 1, 0, time.UTC),
					Nonce:     "nonce-2",
				}},
			exp: exp{
				err: model.ErrChallengeNotFound,
			},
		},
	}

	const q = `insert into challenge (payment_id, created_at, nonce) values($1, $2, $3)`

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			_, err1 := dbi.ExecContext(ctx, q, tc.given.chal.PaymentID, tc.given.chal.CreatedAt, tc.given.chal.Nonce)
			must.NoError(t, err1)

			c := Challenge{}
			actual, err2 := c.Get(ctx, dbi, tc.given.paymentID)

			should.Equal(t, tc.exp.err, err2)
			should.Equal(t, tc.exp.chal.PaymentID, actual.PaymentID)
			should.Equal(t, tc.exp.chal.CreatedAt, actual.CreatedAt)
			should.Equal(t, tc.exp.chal.Nonce, actual.Nonce)
		})
	}
}

func TestChallenge_Upsert(t *testing.T) {
	dbi, err := setupDBI()
	must.NoError(t, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE TABLE challenge;")
	}()

	const q = `select * from challenge where payment_id = $1`

	chlRepo := &Challenge{}

	t.Run("insert", func(t *testing.T) {
		exp := model.Challenge{
			PaymentID: uuid.FromStringOrNil("66e4751f-cd72-4bb0-aebd-66c50a2e8c45"),
			CreatedAt: time.Date(2024, 1, 1, 1, 1, 1, 0, time.UTC),
			Nonce:     "a",
		}

		err1 := chlRepo.Upsert(context.TODO(), dbi, exp)
		must.NoError(t, err1)

		var actual model.Challenge
		err2 := sqlx.GetContext(context.TODO(), dbi, &actual, q, exp.PaymentID)
		must.NoError(t, err2)

		should.Equal(t, exp.PaymentID, actual.PaymentID)
		should.Equal(t, exp.CreatedAt, actual.CreatedAt)
		should.Equal(t, exp.Nonce, actual.Nonce)
	})

	t.Run("upsert", func(t *testing.T) {
		ctx := context.Background()

		exp := model.Challenge{
			PaymentID: uuid.FromStringOrNil("66e4751f-cd72-4bb0-aebd-66c50a2e8c45"),
			CreatedAt: time.Date(2024, 12, 1, 1, 1, 1, 0, time.UTC),
			Nonce:     "b",
		}

		err1 := chlRepo.Upsert(ctx, dbi, exp)
		must.NoError(t, err1)

		var actual model.Challenge
		err2 := sqlx.GetContext(ctx, dbi, &actual, q, exp.PaymentID)
		must.NoError(t, err2)

		should.Equal(t, exp.PaymentID, actual.PaymentID)
		should.Equal(t, exp.CreatedAt, actual.CreatedAt)
		should.Equal(t, exp.Nonce, actual.Nonce)
	})
}

func TestChallenge_Delete(t *testing.T) {
	dbi, err := setupDBI()
	must.NoError(t, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE TABLE challenge;")
	}()

	type tcGiven struct {
		paymentID uuid.UUID
		chal      model.Challenge
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
				paymentID: uuid.FromStringOrNil("66e4751f-cd72-4bb0-aebd-66c50a2e8c45"),
				chal: model.Challenge{
					PaymentID: uuid.FromStringOrNil("66e4751f-cd72-4bb0-aebd-66c50a2e8c45"),
					Nonce:     "nonce-1",
				}},
			exp: exp{
				err: nil,
			},
		},
		{
			name: "no_rows_deleted",
			given: tcGiven{
				paymentID: uuid.FromStringOrNil("34fe675b-aebf-4209-90b6-a7ba4452087a"),
				chal: model.Challenge{
					PaymentID: uuid.FromStringOrNil("f6d43f0c-db24-4d65-9f02-b000ff8ec782"),
					CreatedAt: time.Date(2024, 1, 1, 1, 1, 1, 0, time.UTC),
					Nonce:     "nonce-2",
				}},
			exp: exp{
				err: model.ErrNoRowsDeleted,
			},
		},
	}

	const q = `insert into challenge (payment_id, created_at, nonce) values($1, $2, $3)`

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			_, err1 := dbi.ExecContext(ctx, q, tc.given.chal.PaymentID, tc.given.chal.CreatedAt, tc.given.chal.Nonce)
			must.NoError(t, err1)

			c := Challenge{}
			err2 := c.Delete(ctx, dbi, tc.given.paymentID)
			should.Equal(t, tc.exp.err, err2)
		})
	}
}

func TestChallenge_DeleteAfter(t *testing.T) {
	dbi, err := setupDBI()
	must.NoError(t, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE TABLE challenge;")
	}()

	type tcGiven struct {
		interval   time.Duration
		challenges []model.Challenge
	}

	type exp struct {
		errDel error
		errGet error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   exp
	}

	tests := []testCase{
		{
			name: "delete_single",
			given: tcGiven{
				interval: 1,
				challenges: []model.Challenge{
					{
						PaymentID: uuid.NewV4(),
						CreatedAt: time.Now().Add(-6 * time.Minute),
						Nonce:     "nonce-1",
					},
				},
			},
			exp: exp{
				errDel: nil,
				errGet: model.ErrChallengeNotFound,
			},
		},
		{
			name: "delete_multiple",
			given: tcGiven{
				interval: 1,
				challenges: []model.Challenge{
					{
						PaymentID: uuid.NewV4(),
						CreatedAt: time.Now().Add(-6 * time.Minute),
						Nonce:     "nonce-1",
					},
					{
						PaymentID: uuid.NewV4(),
						CreatedAt: time.Now().Add(-10 * time.Minute),
						Nonce:     "nonce-2",
					},
				},
			},
			exp: exp{
				errDel: nil,
				errGet: model.ErrChallengeNotFound,
			},
		},
		{
			name: "delete_none",
			given: tcGiven{
				interval: 1,
				challenges: []model.Challenge{
					{
						PaymentID: uuid.NewV4(),
						CreatedAt: time.Now(),
						Nonce:     "nonce-1",
					},
					{
						PaymentID: uuid.NewV4(),
						CreatedAt: time.Now(),
						Nonce:     "nonce-2",
					},
				},
			},
			exp: exp{
				errDel: model.ErrNoRowsDeleted,
				errGet: nil,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			c := Challenge{}

			for j := range tc.given.challenges {
				err := c.Upsert(ctx, dbi, tc.given.challenges[j])
				must.NoError(t, err)
			}

			err := c.DeleteAfter(ctx, dbi, tc.given.interval)
			must.Equal(t, tc.exp.errDel, err)

			for j := range tc.given.challenges {
				_, err := c.Get(ctx, dbi, tc.given.challenges[j].PaymentID)
				must.Equal(t, tc.exp.errGet, err)
			}
		})
	}
}

func TestAllowList_GetAllowListEntry(t *testing.T) {
	dbi, err := setupDBI()
	must.NoError(t, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE TABLE allow_list;")
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

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			_, err1 := dbi.ExecContext(ctx, q, tc.given.allow.PaymentID, tc.given.allow.CreatedAt)
			must.NoError(t, err1)

			a := &AllowList{}
			actual, err2 := a.GetAllowListEntry(ctx, dbi, tc.given.paymentID)
			should.Equal(t, tc.exp.err, err2)
			should.Equal(t, tc.exp.allow, actual)
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
