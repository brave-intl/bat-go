//go:build integration

package repository_test

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

						`INSERT INTO order_items (id, order_id, sku, sku_variant, credential_type, currency, quantity, price, subtotal, created_at, updated_at)
							VALUES ('ad0be000-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'brave-vpn-premium', 'brave-vpn-premium', 'time-limited-v2', 'USD', 1, 9.99, 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

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

						`INSERT INTO order_items (id, order_id, sku, sku_variant, credential_type, currency, quantity, price, subtotal, created_at, updated_at)
							VALUES ('ad0be000-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'brave-vpn-premium', 'brave-vpn-premium', 'time-limited-v2', 'USD', 1, 9.99, 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

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

						`INSERT INTO order_items (id, order_id, sku, sku_variant, credential_type, currency, quantity, price, subtotal, created_at, updated_at)
							VALUES ('ad0be000-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'brave-vpn-premium', 'brave-vpn-premium', 'time-limited-v2', 'USD', 1, 9.99, 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

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

						`INSERT INTO order_items (id, order_id, sku, sku_variant, credential_type, currency, quantity, price, subtotal, created_at, updated_at)
							VALUES ('ad0be000-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'brave-vpn-premium', 'brave-vpn-premium', 'time-limited-v2', 'USD', 1, 9.99, 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

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

						`INSERT INTO order_items (id, order_id, sku, sku_variant, credential_type, currency, quantity, price, subtotal, created_at, updated_at)
							VALUES ('ad0be000-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'brave-vpn-premium', 'brave-vpn-premium', 'time-limited-v2', 'USD', 1, 9.99, 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

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

						`INSERT INTO order_items (id, order_id, sku, sku_variant, credential_type, currency, quantity, price, subtotal, created_at, updated_at)
							VALUES ('ad0be000-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'brave-vpn-premium', 'brave-vpn-premium', 'time-limited-v2', 'USD', 1, 9.99, 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

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

						`INSERT INTO order_items (id, order_id, sku, sku_variant, credential_type, currency, quantity, price, subtotal, created_at, updated_at)
							VALUES ('ad0be000-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'brave-vpn-premium', 'brave-vpn-premium', 'time-limited-v2', 'USD', 1, 9.99, 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

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

						`INSERT INTO order_items (id, order_id, sku, sku_variant, credential_type, currency, quantity, price, subtotal, created_at, updated_at)
							VALUES ('ad0be000-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'brave-vpn-premium', 'brave-vpn-premium', 'time-limited-v2', 'USD', 1, 9.99, 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

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

func TestTLV2_ActiveBatchesByOrder(t *testing.T) {
	dbi, err := setupDBI()
	must.Equal(t, nil, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE TABLE time_limited_v2_order_creds, order_cred_issuers, order_items, orders;")
	}()

	type tcGiven struct {
		orderID uuid.UUID
		itemID  *uuid.UUID
		now     time.Time

		fnBefore func(ctx context.Context, dbi sqlx.ExtContext) error
	}

	type tcExpected struct {
		val []model.TLV2ActiveBatch
		err error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	sharedFixture := func(ctx context.Context, dbi sqlx.ExtContext) error {
		qs := []string{
			`INSERT INTO order_cred_issuers (id, merchant_id, public_key, created_at)
				VALUES ('5ca1ab1e-0000-4000-a000-000000000000', 'brave.com', 'public_key_01', '2024-01-01 00:00:01');`,

			`INSERT INTO orders (id, merchant_id, status, currency, total_price, created_at, updated_at)
				VALUES ('c0c0a000-0000-4000-a000-000000000000', 'brave.com', 'paid', 'USD', 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

			`INSERT INTO order_items (id, order_id, sku, sku_variant, credential_type, currency, quantity, price, subtotal, created_at, updated_at)
				VALUES ('ad0be000-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'brave-vpn-premium', 'brave-vpn-premium', 'time-limited-v2', 'USD', 1, 9.99, 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

			`INSERT INTO order_items (id, order_id, sku, sku_variant, credential_type, currency, quantity, price, subtotal, created_at, updated_at)
				VALUES ('ad0be000-0000-4000-a000-000000000001', 'c0c0a000-0000-4000-a000-000000000000', 'brave-vpn-premium', 'brave-vpn-premium', 'time-limited-v2', 'USD', 1, 9.99, 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,
		}

		for i := range qs {
			if _, err := dbi.ExecContext(ctx, qs[i]); err != nil {
				return err
			}
		}

		return nil
	}

	tests := []testCase{
		{
			name: "zero",
			given: tcGiven{
				orderID:  uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				now:      time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC),
				fnBefore: sharedFixture,
			},
			exp: tcExpected{val: []model.TLV2ActiveBatch{}},
		},

		{
			name: "one_batch_two_rows",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				now:     time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC),

				fnBefore: func(ctx context.Context, dbi sqlx.ExtContext) error {
					if err := sharedFixture(ctx, dbi); err != nil {
						return err
					}

					qs := []string{
						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('decade00-0000-4000-a000-000000000000', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', 'f100ded0-0000-4000-a000-000000000000', '2024-01-01 00:00:01', '2024-01-03 00:00:00', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_01"]', '["scred_01"]');`,

						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('decade00-0000-4000-a000-000000000001', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', 'f100ded0-0000-4000-a000-000000000000', '2024-01-02 00:00:01', '2024-01-04 00:00:00', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_02"]', '["scred_02"]');`,
					}

					for i := range qs {
						if _, err := dbi.ExecContext(ctx, qs[i]); err != nil {
							return err
						}
					}

					return nil
				},
			},
			exp: tcExpected{
				val: []model.TLV2ActiveBatch{
					{RequestID: "f100ded0-0000-4000-a000-000000000000", OldestValidFrom: time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC)},
				},
			},
		},

		{
			name: "two_batches_ordered_oldest_first",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				now:     time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC),

				fnBefore: func(ctx context.Context, dbi sqlx.ExtContext) error {
					if err := sharedFixture(ctx, dbi); err != nil {
						return err
					}

					qs := []string{
						// newer batch inserted first to confirm ORDER BY
						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('facade00-0000-4000-a000-000000000000', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', 'f100ded0-0000-4000-a000-000000000001', '2024-01-02 00:00:01', '2024-01-04 00:00:00', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_02"]', '["scred_02"]');`,

						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('decade00-0000-4000-a000-000000000000', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', 'f100ded0-0000-4000-a000-000000000000', '2024-01-01 00:00:01', '2024-01-03 00:00:00', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_01"]', '["scred_01"]');`,
					}

					for i := range qs {
						if _, err := dbi.ExecContext(ctx, qs[i]); err != nil {
							return err
						}
					}

					return nil
				},
			},
			exp: tcExpected{
				val: []model.TLV2ActiveBatch{
					{RequestID: "f100ded0-0000-4000-a000-000000000000", OldestValidFrom: time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC)},
					{RequestID: "f100ded0-0000-4000-a000-000000000001", OldestValidFrom: time.Date(2024, time.January, 2, 0, 0, 1, 0, time.UTC)},
				},
			},
		},

		{
			name: "expired_excluded",
			given: tcGiven{
				// now is past valid_to so rows are expired
				orderID: uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				now:     time.Date(2024, time.January, 4, 0, 0, 0, 0, time.UTC),

				fnBefore: func(ctx context.Context, dbi sqlx.ExtContext) error {
					if err := sharedFixture(ctx, dbi); err != nil {
						return err
					}

					_, err := dbi.ExecContext(ctx,
						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('decade00-0000-4000-a000-000000000000', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', 'f100ded0-0000-4000-a000-000000000000', '2024-01-01 00:00:01', '2024-01-03 00:00:00', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_01"]', '["scred_01"]');`,
					)

					return err
				},
			},
			exp: tcExpected{val: []model.TLV2ActiveBatch{}},
		},

		{
			name: "item_id_filter",
			given: tcGiven{
				orderID: uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				itemID:  func() *uuid.UUID { id := uuid.Must(uuid.FromString("ad0be000-0000-4000-a000-000000000000")); return &id }(),
				now:     time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC),

				fnBefore: func(ctx context.Context, dbi sqlx.ExtContext) error {
					if err := sharedFixture(ctx, dbi); err != nil {
						return err
					}

					qs := []string{
						// item 0 batch
						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('decade00-0000-4000-a000-000000000000', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', 'f100ded0-0000-4000-a000-000000000000', '2024-01-01 00:00:01', '2024-01-03 00:00:00', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_01"]', '["scred_01"]');`,

						// item 1 batch — should be excluded by the filter
						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('facade00-0000-4000-a000-000000000000', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000001', 'f100ded0-0000-4000-a000-000000000001', '2024-01-01 00:00:02', '2024-01-03 00:00:00', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_02"]', '["scred_02"]');`,
					}

					for i := range qs {
						if _, err := dbi.ExecContext(ctx, qs[i]); err != nil {
							return err
						}
					}

					return nil
				},
			},
			exp: tcExpected{
				val: []model.TLV2ActiveBatch{
					{RequestID: "f100ded0-0000-4000-a000-000000000000", OldestValidFrom: time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC)},
				},
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
				must.Equal(t, nil, err)
			}

			var actual []model.TLV2ActiveBatch
			if tc.given.itemID != nil {
				actual, err = repo.ActiveBatchesByOrderItem(ctx, tx, tc.given.orderID, *tc.given.itemID, tc.given.now)
			} else {
				actual, err = repo.ActiveBatchesByOrder(ctx, tx, tc.given.orderID, tc.given.now)
			}
			must.Equal(t, tc.exp.err, err)

			if tc.exp.err != nil {
				return
			}

			should.Equal(t, tc.exp.val, actual)
		})
	}
}

func TestTLV2_DeleteByRequestIDs(t *testing.T) {
	dbi, err := setupDBI()
	must.Equal(t, nil, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE TABLE signing_order_request_outbox, time_limited_v2_order_creds, order_cred_issuers, order_items, orders;")
	}()

	type tcGiven struct {
		orderID    uuid.UUID
		requestIDs []string

		fnBefore func(ctx context.Context, dbi sqlx.ExtContext) error
	}

	type tcExpected struct {
		remainingReqIDs []string // request_ids that should still exist in time_limited_v2_order_creds
		err             error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	baseFixture := func(ctx context.Context, dbi sqlx.ExtContext) error {
		qs := []string{
			`INSERT INTO order_cred_issuers (id, merchant_id, public_key, created_at)
				VALUES ('5ca1ab1e-0000-4000-a000-000000000000', 'brave.com', 'public_key_01', '2024-01-01 00:00:01');`,

			`INSERT INTO orders (id, merchant_id, status, currency, total_price, created_at, updated_at)
				VALUES ('c0c0a000-0000-4000-a000-000000000000', 'brave.com', 'paid', 'USD', 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,

			`INSERT INTO order_items (id, order_id, sku, sku_variant, credential_type, currency, quantity, price, subtotal, created_at, updated_at)
				VALUES ('ad0be000-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'brave-vpn-premium', 'brave-vpn-premium', 'time-limited-v2', 'USD', 1, 9.99, 9.99, '2024-01-01 00:00:01', '2024-01-01 00:00:01');`,
		}

		for i := range qs {
			if _, err := dbi.ExecContext(ctx, qs[i]); err != nil {
				return err
			}
		}

		return nil
	}

	tests := []testCase{
		{
			name: "deletes_target_leaves_others",
			given: tcGiven{
				orderID:    uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				requestIDs: []string{"f100ded0-0000-4000-a000-000000000000"},

				fnBefore: func(ctx context.Context, dbi sqlx.ExtContext) error {
					if err := baseFixture(ctx, dbi); err != nil {
						return err
					}

					qs := []string{
						// batch to be deleted
						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('decade00-0000-4000-a000-000000000000', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', 'f100ded0-0000-4000-a000-000000000000', '2024-01-01 00:00:01', '2024-01-02 00:00:00', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_01"]', '["scred_01"]');`,

						// batch that must survive
						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('facade00-0000-4000-a000-000000000000', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', 'f100ded0-0000-4000-a000-000000000001', '2024-01-02 00:00:01', '2024-01-03 00:00:00', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_02"]', '["scred_02"]');`,

						// outbox entry for the batch to be deleted
						`INSERT INTO signing_order_request_outbox (request_id, order_id, item_id, message_data)
							VALUES ('f100ded0-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', '{}');`,
					}

					for i := range qs {
						if _, err := dbi.ExecContext(ctx, qs[i]); err != nil {
							return err
						}
					}

					return nil
				},
			},
			exp: tcExpected{
				remainingReqIDs: []string{"f100ded0-0000-4000-a000-000000000001"},
			},
		},

		{
			name: "empty_request_ids_noop",
			given: tcGiven{
				orderID:    uuid.Must(uuid.FromString("c0c0a000-0000-4000-a000-000000000000")),
				requestIDs: []string{},

				fnBefore: func(ctx context.Context, dbi sqlx.ExtContext) error {
					if err := baseFixture(ctx, dbi); err != nil {
						return err
					}

					_, err := dbi.ExecContext(ctx,
						`INSERT INTO time_limited_v2_order_creds (id, issuer_id, order_id, item_id, request_id, valid_from, valid_to, created_at, batch_proof, public_key, blinded_creds, signed_creds)
							VALUES ('decade00-0000-4000-a000-000000000000', '5ca1ab1e-0000-4000-a000-000000000000', 'c0c0a000-0000-4000-a000-000000000000', 'ad0be000-0000-4000-a000-000000000000', 'f100ded0-0000-4000-a000-000000000000', '2024-01-01 00:00:01', '2024-01-02 00:00:00', '2024-01-01 00:00:01', 'proof_01', 'public_key_01', '["cred_01"]', '["scred_01"]');`,
					)

					return err
				},
			},
			exp: tcExpected{
				remainingReqIDs: []string{"f100ded0-0000-4000-a000-000000000000"},
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
				must.Equal(t, nil, err)
			}

			err = repo.DeleteCredsByRequestIDs(ctx, tx, tc.given.orderID, tc.given.requestIDs)
			if err == nil {
				err = repo.DeleteOutboxByRequestIDs(ctx, tx, tc.given.orderID, tc.given.requestIDs)
			}
			must.Equal(t, tc.exp.err, err)

			if tc.exp.err != nil {
				return
			}

			// Verify the surviving request_ids in time_limited_v2_order_creds.
			var remaining []string
			const q = `SELECT DISTINCT request_id FROM time_limited_v2_order_creds WHERE order_id=$1 ORDER BY request_id;`
			err = sqlx.SelectContext(ctx, tx, &remaining, q, tc.given.orderID)
			must.Equal(t, nil, err)

			should.Equal(t, tc.exp.remainingReqIDs, remaining)

			// Verify the outbox entry for the deleted request_id is gone.
			if len(tc.given.requestIDs) > 0 {
				var outboxCount int
				const qo = `SELECT COUNT(*) FROM signing_order_request_outbox WHERE request_id::text = ANY($1);`
				err = sqlx.GetContext(ctx, tx, &outboxCount, qo, pq.Array(tc.given.requestIDs))
				must.Equal(t, nil, err)
				should.Equal(t, 0, outboxCount)
			}
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
