//go:build integration

package repository_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/services/skus/model"
	"github.com/brave-intl/bat-go/services/skus/storage/repository"
)

func TestTLV2_GetCredSubmissionReport(t *testing.T) {
	dbi, err := setupDBI()
	must.Equal(t, nil, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE TABLE time_limited_v2_order_creds, order_cred_issuers, order_items, orders;")
	}()

	type tcGiven struct {
		orderID    uuid.UUID
		itemID     uuid.UUID
		reqID      uuid.UUID
		firstBCred string

		fnBefore func(ctx context.Context, dbi sqlx.ExtContext) error
	}

	type tcExpected struct {
		val model.TLV2CredSubmissionReport
		err error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "submitted",
			given: tcGiven{
				orderID:    uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				itemID:     uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
				reqID:      uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
				firstBCred: "cred_01",

				fnBefore: func(ctx context.Context, dbi sqlx.ExtContext) error {
					qs := []string{
						`INSERT INTO order_cred_issuers (id, merchant_id, public_key, created_at)
							VALUES ('5ca1ab1e-0000-4000-a000-000000000000', 'brave.com', 'public_key_01', '2024-01-01 00:00:01');`,

						`INSERT INTO orders (id, merchant_id, status, currency, total_price, created_at, updated_at)
							VALUES ('c0c0a000-0000-4000-a000-000000000000', 'brave.com', 'paid', 'USD', 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

						`INSERT INTO order_items (id, order_id, sku, credential_type, currency, quantity, price, subtotal, created_at, updated_at)
							VALUES ('ad0be000-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'brave-vpn-premium', 'time-limited-v2', 'USD', 1, 9.99, 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('decade00-0000-4000-a000-000000000000', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', 'f100ded0-0000-4000-a000-000000000000', '2024-01-01 00:00:02', '2024-01-02 00:00:02', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_01", "cred_02", "cred_03"]', '["scred_01", "scred_02", "scred_03"]');`,
					}

					for i := range qs {
						if _, err := dbi.ExecContext(ctx, qs[i]); err != nil {
							return nil
						}
					}

					return nil
				},
			},
			exp: tcExpected{
				val: model.TLV2CredSubmissionReport{Submitted: true},
			},
		},

		{
			name: "mismatch",
			given: tcGiven{
				orderID:    uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				itemID:     uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
				reqID:      uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
				firstBCred: "cred_01",

				fnBefore: func(ctx context.Context, dbi sqlx.ExtContext) error {
					qs := []string{
						`INSERT INTO order_cred_issuers (id, merchant_id, public_key, created_at)
							VALUES ('5ca1ab1e-0000-4000-a000-000000000000', 'brave.com', 'public_key_01', '2024-01-01 00:00:01');`,

						`INSERT INTO orders (id, merchant_id, status, currency, total_price, created_at, updated_at)
							VALUES ('c0c0a000-0000-4000-a000-000000000000', 'brave.com', 'paid', 'USD', 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

						`INSERT INTO order_items (id, order_id, sku, credential_type, currency, quantity, price, subtotal, created_at, updated_at)
							VALUES ('ad0be000-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'brave-vpn-premium', 'time-limited-v2', 'USD', 1, 9.99, 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('decade00-0000-4000-a000-000000000000', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', 'f100ded0-0000-4000-a000-000000000000', '2024-01-01 00:00:02', '2024-01-02 00:00:02', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_11", "cred_12", "cred_13"]', '["scred_11", "scred_12", "scred_13"]');`,
					}

					for i := range qs {
						if _, err := dbi.ExecContext(ctx, qs[i]); err != nil {
							return nil
						}
					}

					return nil
				},
			},
			exp: tcExpected{
				val: model.TLV2CredSubmissionReport{ReqIDMismatch: true},
			},
		},
	}

	repo := repository.NewTLV2()

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			tx, err := dbi.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadUncommitted})
			must.Equal(t, nil, err)

			t.Cleanup(func() { _ = tx.Rollback() })

			if tc.given.fnBefore != nil {
				err := tc.given.fnBefore(ctx, tx)
				t.Log(err)
				must.Equal(t, nil, err)
			}

			actual, err := repo.GetCredSubmissionReport(ctx, tx, tc.given.orderID, tc.given.itemID, tc.given.reqID, tc.given.firstBCred)
			must.Equal(t, tc.exp.err, err)

			if tc.exp.err != nil {
				return
			}

			should.Equal(t, tc.exp.val, actual)
		})
	}
}

func TestTLV2_UniqBatches(t *testing.T) {
	dbi, err := setupDBI()
	must.Equal(t, nil, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE TABLE time_limited_v2_order_creds, order_cred_issuers, order_items, orders;")
	}()

	type tcGiven struct {
		orderID uuid.UUID
		itemID  uuid.UUID
		from    time.Time
		to      time.Time

		fnBefore func(ctx context.Context, dbi sqlx.ExtContext) error
	}

	type tcExpected struct {
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
			name: "zero",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				itemID:  uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
				from:    time.UnixMilli(1704067201000),
				to:      time.UnixMilli(1704067201000),

				fnBefore: func(ctx context.Context, dbi sqlx.ExtContext) error { return nil },
			},
			exp: tcExpected{},
		},

		{
			name: "one",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				itemID:  uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
				from:    time.UnixMilli(1704110401000),
				to:      time.UnixMilli(1704110401000),

				fnBefore: func(ctx context.Context, dbi sqlx.ExtContext) error {
					qs := []string{
						`INSERT INTO order_cred_issuers (id, merchant_id, public_key, created_at)
							VALUES ('5ca1ab1e-0000-4000-a000-000000000000', 'brave.com', 'public_key_01', '2024-01-01 00:00:01');`,

						`INSERT INTO orders (id, merchant_id, status, currency, total_price, created_at, updated_at)
							VALUES ('c0c0a000-0000-4000-a000-000000000000', 'brave.com', 'paid', 'USD', 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

						`INSERT INTO order_items (id, order_id, sku, credential_type, currency, quantity, price, subtotal, created_at, updated_at)
							VALUES ('ad0be000-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'brave-vpn-premium', 'time-limited-v2', 'USD', 1, 9.99, 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('decade00-0000-4000-a000-000000000000', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', 'f100ded0-0000-4000-a000-000000000000', '2024-01-01 00:00:01', '2024-01-02 00:00:01', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_01", "cred_02", "cred_03"]', '["scred_01", "scred_02", "scred_03"]');`,
					}

					for i := range qs {
						if _, err := dbi.ExecContext(ctx, qs[i]); err != nil {
							return nil
						}
					}

					return nil
				},
			},
			exp: tcExpected{
				val: 1,
			},
		},

		{
			name: "two",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				itemID:  uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
				from:    time.UnixMilli(1704110401000),
				to:      time.UnixMilli(1704110401000),

				fnBefore: func(ctx context.Context, dbi sqlx.ExtContext) error {
					qs := []string{
						`INSERT INTO order_cred_issuers (id, merchant_id, public_key, created_at)
							VALUES ('5ca1ab1e-0000-4000-a000-000000000000', 'brave.com', 'public_key_01', '2024-01-01 00:00:01');`,

						`INSERT INTO orders (id, merchant_id, status, currency, total_price, created_at, updated_at)
							VALUES ('c0c0a000-0000-4000-a000-000000000000', 'brave.com', 'paid', 'USD', 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

						`INSERT INTO order_items (id, order_id, sku, credential_type, currency, quantity, price, subtotal, created_at, updated_at)
							VALUES ('ad0be000-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'brave-vpn-premium', 'time-limited-v2', 'USD', 1, 9.99, 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('decade00-0000-4000-a000-000000000000', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', 'f100ded0-0000-4000-a000-000000000000', '2024-01-01 00:00:01', '2024-01-02 00:00:01', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_01", "cred_02", "cred_03"]', '["scred_01", "scred_02", "scred_03"]');`,

						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('facade00-0000-4000-a000-000000000000', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', 'f100ded0-0000-4000-a000-000000000001', '2024-01-01 00:00:01', '2024-01-02 00:00:01', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_01", "cred_02", "cred_03"]', '["scred_01", "scred_02", "scred_03"]');`,
					}

					for i := range qs {
						if _, err := dbi.ExecContext(ctx, qs[i]); err != nil {
							return nil
						}
					}

					return nil
				},
			},
			exp: tcExpected{
				val: 2,
			},
		},

		{
			name: "multiple",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				itemID:  uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
				from:    time.UnixMilli(1704110401000),
				to:      time.UnixMilli(1704110401000),

				fnBefore: func(ctx context.Context, dbi sqlx.ExtContext) error {
					qs := []string{
						`INSERT INTO order_cred_issuers (id, merchant_id, public_key, created_at)
							VALUES ('5ca1ab1e-0000-4000-a000-000000000000', 'brave.com', 'public_key_01', '2024-01-01 00:00:01');`,

						`INSERT INTO orders (id, merchant_id, status, currency, total_price, created_at, updated_at)
							VALUES ('c0c0a000-0000-4000-a000-000000000000', 'brave.com', 'paid', 'USD', 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

						`INSERT INTO order_items (id, order_id, sku, credential_type, currency, quantity, price, subtotal, created_at, updated_at)
							VALUES ('ad0be000-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'brave-vpn-premium', 'time-limited-v2', 'USD', 1, 9.99, 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('decade00-0000-4000-a000-000000000000', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', 'f100ded0-0000-4000-a000-000000000000', '2024-01-01 00:00:01', '2024-01-02 00:00:01', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_01", "cred_02", "cred_03"]', '["scred_01", "scred_02", "scred_03"]');`,

						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('decade00-0000-4000-a000-000000000001', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', 'f100ded0-0000-4000-a000-000000000000', '2024-01-02 00:00:01', '2024-01-03 00:00:01', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_01", "cred_02", "cred_03"]', '["scred_01", "scred_02", "scred_03"]');`,

						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('facade00-0000-4000-a000-000000000000', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', 'f100ded0-0000-4000-a000-000000000001', '2024-01-01 00:00:01', '2024-01-02 00:00:01', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_01", "cred_02", "cred_03"]', '["scred_01", "scred_02", "scred_03"]');`,

						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('facade00-0000-4000-a000-000000000001', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', 'f100ded0-0000-4000-a000-000000000001', '2024-01-02 00:00:01', '2024-01-03 00:00:01', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_01", "cred_02", "cred_03"]', '["scred_01", "scred_02", "scred_03"]');`,
					}

					for i := range qs {
						if _, err := dbi.ExecContext(ctx, qs[i]); err != nil {
							return nil
						}
					}

					return nil
				},
			},
			exp: tcExpected{
				val: 2,
			},
		},
	}

	repo := repository.NewTLV2()

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			tx, err := dbi.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadUncommitted})
			must.Equal(t, nil, err)

			t.Cleanup(func() { _ = tx.Rollback() })

			if tc.given.fnBefore != nil {
				err := tc.given.fnBefore(ctx, tx)
				t.Log(err)
				must.Equal(t, nil, err)
			}

			actual, err := repo.UniqBatches(ctx, tx, tc.given.orderID, tc.given.itemID, tc.given.from, tc.given.to)
			must.Equal(t, tc.exp.err, err)

			if tc.exp.err != nil {
				return
			}

			should.Equal(t, tc.exp.val, actual)
		})
	}
}

func TestTLV2_DeleteLegacy(t *testing.T) {
	dbi, err := setupDBI()
	must.Equal(t, nil, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE TABLE time_limited_v2_order_creds, order_cred_issuers, order_items, orders;")
	}()

	type tcGiven struct {
		orderID uuid.UUID
		itemID  uuid.UUID
		reqID   uuid.UUID

		fnBefore func(ctx context.Context, dbi sqlx.ExtContext) error
	}

	type tcExpected struct {
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
			name: "empty",
			given: tcGiven{
				orderID:  uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				itemID:   uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
				reqID:    uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
				fnBefore: func(ctx context.Context, dbi sqlx.ExtContext) error { return nil },
			},
			exp: tcExpected{},
		},

		{
			name: "one",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				itemID:  uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
				reqID:   uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),

				fnBefore: func(ctx context.Context, dbi sqlx.ExtContext) error {
					qs := []string{
						`INSERT INTO order_cred_issuers (id, merchant_id, public_key, created_at)
							VALUES ('5ca1ab1e-0000-4000-a000-000000000000', 'brave.com', 'public_key_01', '2024-01-01 00:00:01');`,

						`INSERT INTO orders (id, merchant_id, status, currency, total_price, created_at, updated_at)
							VALUES ('c0c0a000-0000-4000-a000-000000000000', 'brave.com', 'paid', 'USD', 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

						`INSERT INTO order_items (id, order_id, sku, credential_type, currency, quantity, price, subtotal, created_at, updated_at)
							VALUES ('ad0be000-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'brave-vpn-premium', 'time-limited-v2', 'USD', 1, 9.99, 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('decade00-0000-4000-a000-000000000000', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', '2024-01-01 00:00:01', '2024-01-02 00:00:01', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_01", "cred_02", "cred_03"]', '["scred_01", "scred_02", "scred_03"]');`,
					}

					for i := range qs {
						if _, err := dbi.ExecContext(ctx, qs[i]); err != nil {
							return nil
						}
					}

					return nil
				},
			},
			exp: tcExpected{},
		},

		{
			name: "two",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				itemID:  uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
				reqID:   uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),

				fnBefore: func(ctx context.Context, dbi sqlx.ExtContext) error {
					qs := []string{
						`INSERT INTO order_cred_issuers (id, merchant_id, public_key, created_at)
							VALUES ('5ca1ab1e-0000-4000-a000-000000000000', 'brave.com', 'public_key_01', '2024-01-01 00:00:01');`,

						`INSERT INTO orders (id, merchant_id, status, currency, total_price, created_at, updated_at)
							VALUES ('c0c0a000-0000-4000-a000-000000000000', 'brave.com', 'paid', 'USD', 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

						`INSERT INTO order_items (id, order_id, sku, credential_type, currency, quantity, price, subtotal, created_at, updated_at)
							VALUES ('ad0be000-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'brave-vpn-premium', 'time-limited-v2', 'USD', 1, 9.99, 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('decade00-0000-4000-a000-000000000000', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', '2024-01-01 00:00:01', '2024-01-02 00:00:01', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_01", "cred_02", "cred_03"]', '["scred_01", "scred_02", "scred_03"]');`,

						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('facade00-0000-4000-a000-000000000000', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', 'f100ded0-0000-4000-a000-000000000000', '2024-01-01 00:00:01', '2024-01-02 00:00:01', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_01", "cred_02", "cred_03"]', '["scred_01", "scred_02", "scred_03"]');`,
					}

					for i := range qs {
						if _, err := dbi.ExecContext(ctx, qs[i]); err != nil {
							return nil
						}
					}

					return nil
				},
			},
			exp: tcExpected{},
		},

		{
			name: "multiple",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				itemID:  uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),
				reqID:   uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")),

				fnBefore: func(ctx context.Context, dbi sqlx.ExtContext) error {
					qs := []string{
						`INSERT INTO order_cred_issuers (id, merchant_id, public_key, created_at)
							VALUES ('5ca1ab1e-0000-4000-a000-000000000000', 'brave.com', 'public_key_01', '2024-01-01 00:00:01');`,

						`INSERT INTO orders (id, merchant_id, status, currency, total_price, created_at, updated_at)
							VALUES ('c0c0a000-0000-4000-a000-000000000000', 'brave.com', 'paid', 'USD', 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

						`INSERT INTO order_items (id, order_id, sku, credential_type, currency, quantity, price, subtotal, created_at, updated_at)
							VALUES ('ad0be000-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'brave-vpn-premium', 'time-limited-v2', 'USD', 1, 9.99, 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('decade00-0000-4000-a000-000000000000', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', '2024-01-01 00:00:01', '2024-01-02 00:00:01', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_01", "cred_02", "cred_03"]', '["scred_01", "scred_02", "scred_03"]');`,

						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('decade00-0000-4000-a000-000000000001', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', '2024-01-02 00:00:01', '2024-01-03 00:00:01', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_01", "cred_02", "cred_03"]', '["scred_01", "scred_02", "scred_03"]');`,

						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('facade00-0000-4000-a000-000000000000', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', 'f100ded0-0000-4000-a000-000000000001', '2024-01-01 00:00:01', '2024-01-02 00:00:01', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_01", "cred_02", "cred_03"]', '["scred_01", "scred_02", "scred_03"]');`,

						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('facade00-0000-4000-a000-000000000001', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', 'f100ded0-0000-4000-a000-000000000001', '2024-01-02 00:00:01', '2024-01-03 00:00:01', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_01", "cred_02", "cred_03"]', '["scred_01", "scred_02", "scred_03"]');`,
					}

					for i := range qs {
						if _, err := dbi.ExecContext(ctx, qs[i]); err != nil {
							return nil
						}
					}

					return nil
				},
			},
			exp: tcExpected{},
		},
	}

	repo := repository.NewTLV2()

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			tx, err := dbi.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadUncommitted})
			must.Equal(t, nil, err)

			t.Cleanup(func() { _ = tx.Rollback() })

			if tc.given.fnBefore != nil {
				err := tc.given.fnBefore(ctx, tx)
				t.Log(err)
				must.Equal(t, nil, err)
			}

			{
				err := repo.DeleteLegacy(ctx, tx, tc.given.orderID)
				must.Equal(t, tc.exp.err, err)
			}

			if tc.exp.err != nil {
				return
			}

			actual, err := countTLV2ByItemReqID(ctx, dbi, tc.given.itemID, tc.given.reqID)
			must.Equal(t, nil, err)

			should.Equal(t, 0, actual)
		})
	}
}

func countTLV2ByItemReqID(ctx context.Context, dbi sqlx.QueryerContext, itemID, reqID uuid.UUID) (int, error) {
	const q = `SELECT COUNT(*) FROM time_limited_v2_order_creds WHERE item_id=$1 AND request_id=$2;`

	var num int
	if err := sqlx.GetContext(ctx, dbi, &num, q, itemID, reqID); err != nil {
		return 0, err
	}

	return num, nil
}
