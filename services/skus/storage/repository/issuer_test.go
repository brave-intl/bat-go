//go:build integration

package repository_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/services/skus/model"
	"github.com/brave-intl/bat-go/services/skus/storage/repository"
)

func TestIssuer_GetByMerchID(t *testing.T) {
	dbi, err := setupDBI()
	must.Equal(t, nil, err)

	t.Cleanup(func() {
		_, _ = dbi.Exec("TRUNCATE_TABLE order_cred_issuers;")
	})

	type tcGiven struct {
		merchID string
		mid     string
		pkey    string
	}

	type tcExpected struct {
		result *model.Issuer
		err    error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "not_found",
			given: tcGiven{
				merchID: "not_found",
			},
			exp: tcExpected{
				err: model.ErrIssuerNotFound,
			},
		},

		{
			name: "result_1",
			given: tcGiven{
				merchID: "merch_id",
				mid:     "merch_id",
				pkey:    "public_key",
			},
			exp: tcExpected{
				result: &model.Issuer{
					MerchantID: "merch_id",
					PublicKey:  "public_key",
				},
			},
		},
	}

	repo := repository.NewIssuer()

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			tx, err := dbi.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadUncommitted})
			must.Equal(t, nil, err)

			t.Cleanup(func() { _ = tx.Rollback() })

			if tc.given.mid != "" {
				err := seedIssuerForTest(ctx, tx, tc.given.mid, tc.given.pkey)
				must.Equal(t, nil, err)
			}

			actual, err := repo.GetByMerchID(ctx, tx, tc.given.merchID)
			must.Equal(t, true, errors.Is(err, tc.exp.err))

			if tc.exp.err != nil {
				return
			}

			should.Equal(t, tc.exp.result.MerchantID, actual.MerchantID)
			should.Equal(t, tc.exp.result.PublicKey, actual.PublicKey)
		})
	}
}

func TestIssuer_GetByPubKey(t *testing.T) {
	dbi, err := setupDBI()
	must.Equal(t, nil, err)

	t.Cleanup(func() {
		_, _ = dbi.Exec("TRUNCATE_TABLE order_cred_issuers;")
	})

	type tcGiven struct {
		pubKey string
		mid    string
		pkey   string
	}

	type tcExpected struct {
		result *model.Issuer
		err    error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "not_found",
			given: tcGiven{
				pubKey: "not_found",
			},
			exp: tcExpected{
				err: model.ErrIssuerNotFound,
			},
		},

		{
			name: "result_1",
			given: tcGiven{
				pubKey: "public_key",
				mid:    "merch_id",
				pkey:   "public_key",
			},
			exp: tcExpected{
				result: &model.Issuer{
					MerchantID: "merch_id",
					PublicKey:  "public_key",
				},
			},
		},
	}

	repo := repository.NewIssuer()

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			tx, err := dbi.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadUncommitted})
			must.Equal(t, nil, err)

			t.Cleanup(func() { _ = tx.Rollback() })

			if tc.given.pkey != "" {
				err := seedIssuerForTest(ctx, tx, tc.given.mid, tc.given.pkey)
				must.Equal(t, nil, err)
			}

			actual, err := repo.GetByPubKey(ctx, tx, tc.given.pubKey)
			must.Equal(t, true, errors.Is(err, tc.exp.err))

			if tc.exp.err != nil {
				return
			}

			should.Equal(t, tc.exp.result.MerchantID, actual.MerchantID)
			should.Equal(t, tc.exp.result.PublicKey, actual.PublicKey)
		})
	}
}

func TestIssuer_Create(t *testing.T) {
	dbi, err := setupDBI()
	must.Equal(t, nil, err)

	t.Cleanup(func() {
		_, _ = dbi.Exec("TRUNCATE_TABLE order_cred_issuers;")
	})

	type tcGiven struct {
		req model.IssuerNew
	}

	type tcExpected struct {
		result *model.Issuer
		err    error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "result_1",
			given: tcGiven{
				req: model.IssuerNew{
					MerchantID: "merch_id_1",
					PublicKey:  "public_key_1",
				},
			},
			exp: tcExpected{
				result: &model.Issuer{
					MerchantID: "merch_id_1",
					PublicKey:  "public_key_1",
				},
			},
		},

		{
			name: "result_2",
			given: tcGiven{
				req: model.IssuerNew{
					MerchantID: "merch_id_2",
					PublicKey:  "public_key_2",
				},
			},
			exp: tcExpected{
				result: &model.Issuer{
					MerchantID: "merch_id_2",
					PublicKey:  "public_key_2",
				},
			},
		},
	}

	repo := repository.NewIssuer()

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			tx, err := dbi.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadUncommitted})
			must.Equal(t, nil, err)

			t.Cleanup(func() { _ = tx.Rollback() })

			actual1, err := repo.Create(ctx, tx, tc.given.req)
			must.Equal(t, true, errors.Is(err, tc.exp.err))

			should.Equal(t, tc.exp.result.MerchantID, actual1.MerchantID)
			should.Equal(t, tc.exp.result.PublicKey, actual1.PublicKey)

			actual2, err := repo.GetByMerchID(ctx, tx, actual1.MerchantID)
			must.Equal(t, true, errors.Is(err, tc.exp.err))

			should.Equal(t, actual1, actual2)

			actual3, err := repo.GetByPubKey(ctx, tx, actual2.PublicKey)
			must.Equal(t, true, errors.Is(err, tc.exp.err))

			should.Equal(t, actual2, actual3)
			should.Equal(t, actual1, actual3)
		})
	}
}

func seedIssuerForTest(ctx context.Context, dbi sqlx.ExecerContext, mid, pkey string) error {
	const q = `INSERT INTO order_cred_issuers (merchant_id, public_key)
	VALUES ($1, $2)`

	if _, err := dbi.ExecContext(ctx, q, mid, pkey); err != nil {
		return err
	}

	return nil
}
