//go:build integration

package storage

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
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

const (
	solInsert = `INSERT INTO solana_waitlist (payment_id, joined_at) VALUES($1, $2)`
	solSelect = `SELECT * FROM solana_waitlist WHERE payment_id = $1`
)

type expWaitlistEntry struct {
	PaymentID uuid.UUID `db:"payment_id"`
	JoinedAt  time.Time `db:"joined_at"`
}

func TestSolanaWaitlist_Insert(t *testing.T) {
	dbi, err := setupDBI()
	must.NoError(t, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE TABLE solana_waitlist;")
	}()

	type tcGiven struct {
		paymentID uuid.UUID
		joinedAt  time.Time
		fnBefore  func(ctx context.Context, dbi sqlx.ExecerContext) error
	}

	type tcExpected struct {
		result expWaitlistEntry
		err    error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "pq_constraint_violation",
			given: tcGiven{
				paymentID: uuid.FromStringOrNil("a6c1f1f9-7b24-4244-84e2-7ec696c55965"),
				joinedAt:  time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC),
				fnBefore: func(ctx context.Context, dbi sqlx.ExecerContext) error {
					pid := uuid.FromStringOrNil("a6c1f1f9-7b24-4244-84e2-7ec696c55965")
					jnd := time.Date(2025, time.February, 13, 0, 0, 0, 0, time.UTC)

					_, err := dbi.ExecContext(ctx, solInsert, pid, jnd)

					return err
				},
			},
			exp: tcExpected{
				err: model.ErrSolAlreadyWaitlisted,
			},
		},

		{
			name: "insert_success",
			given: tcGiven{
				paymentID: uuid.FromStringOrNil("57697d78-5498-4b56-baca-7621487ee876"),
				joinedAt:  time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC),
			},
			exp: tcExpected{
				result: expWaitlistEntry{
					PaymentID: uuid.FromStringOrNil("57697d78-5498-4b56-baca-7621487ee876"),
					JoinedAt:  time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC),
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			if tc.given.fnBefore != nil {
				err := tc.given.fnBefore(ctx, dbi)
				must.NoError(t, err)
			}

			repo := NewSolanaWaitlist()

			actual := repo.Insert(ctx, dbi, tc.given.paymentID, tc.given.joinedAt)
			should.Equal(t, tc.exp.err, actual)

			if tc.exp.err == nil {
				result := &expWaitlistEntry{}

				err := sqlx.GetContext(ctx, dbi, result, solSelect, tc.given.paymentID)
				must.NoError(t, err)

				should.Equal(t, tc.exp.result.PaymentID, result.PaymentID)
				should.Equal(t, tc.exp.result.JoinedAt, result.JoinedAt.UTC())
			}
		})
	}
}

func TestSolanaWaitlist_Delete(t *testing.T) {
	dbi, err := setupDBI()
	must.NoError(t, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE TABLE solana_waitlist;")
	}()

	type tcGiven struct {
		paymentID uuid.UUID
		joinedAt  time.Time
		fnBefore  func(ctx context.Context, dbi sqlx.ExecerContext) error
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
			name: "delete_success",
			given: tcGiven{
				paymentID: uuid.FromStringOrNil("13e9416d-b1ff-44c2-87d2-e6d2c24a9d1e"),
				fnBefore: func(ctx context.Context, dbi sqlx.ExecerContext) error {
					pid := uuid.FromStringOrNil("13e9416d-b1ff-44c2-87d2-e6d2c24a9d1e")
					jnd := time.Date(2025, time.February, 13, 0, 0, 0, 0, time.UTC)

					_, err := dbi.ExecContext(ctx, solInsert, pid, jnd)

					return err
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			if tc.given.fnBefore != nil {
				err := tc.given.fnBefore(ctx, dbi)
				must.NoError(t, err)
			}

			repo := NewSolanaWaitlist()

			actual := repo.Delete(ctx, dbi, tc.given.paymentID)
			must.NoError(t, actual)

			result := &expWaitlistEntry{}

			err := sqlx.GetContext(ctx, dbi, result, solSelect, tc.given.paymentID)
			should.Equal(t, sql.ErrNoRows, err)
		})
	}
}

func TestIsUniqueConstraintViolation(t *testing.T) {
	type tcGiven struct {
		err error
	}

	type tcExpected struct {
		result bool
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "not_pq_error",
			given: tcGiven{
				err: model.Error("not_pq_error"),
			},
		},

		{
			name: "not_constraint_error",
			given: tcGiven{
				err: &pq.Error{
					Code: "0",
				},
			},
		},

		{
			name: "constraint_error",
			given: tcGiven{
				err: &pq.Error{
					Code: "23505",
				},
			},
			exp: tcExpected{
				result: true,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := isUniqueConstraintViolation(tc.given.err)
			should.Equal(t, tc.exp.result, actual)
		})
	}
}

func setupDBI() (*sqlx.DB, error) {
	pg, err := datastore.NewPostgres("", true, "")
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
