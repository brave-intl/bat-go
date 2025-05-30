// Package repository provides access to data available in SQL-based data store.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"

	"github.com/brave-intl/bat-go/libs/datastore"

	"github.com/brave-intl/bat-go/services/skus/model"
)

type Order struct{}

func NewOrder() *Order { return &Order{} }

// Get retrieves the order for the given id.
func (r *Order) Get(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
	const q = `SELECT
		id, created_at, currency, updated_at, total_price,
		merchant_id, location, status, allowed_payment_methods,
		metadata, valid_for, last_paid_at, expires_at, trial_days
	FROM orders WHERE id = $1`

	result := &model.Order{}
	if err := sqlx.GetContext(ctx, dbi, result, q, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrOrderNotFound
		}

		return nil, err
	}

	return result, nil
}

// GetByExternalID retrieves the order by extID in metadata.externalID.
func (r *Order) GetByExternalID(ctx context.Context, dbi sqlx.QueryerContext, extID string) (*model.Order, error) {
	const q = `SELECT
		id, created_at, currency, updated_at, total_price,
		merchant_id, location, status, allowed_payment_methods,
		metadata, valid_for, last_paid_at, expires_at, trial_days
	FROM orders WHERE metadata->>'externalID' = $1`

	result := &model.Order{}
	if err := sqlx.GetContext(ctx, dbi, result, q, extID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrOrderNotFound
		}

		return nil, err
	}

	return result, nil
}

func (r *Order) GetByRadomSubscriptionID(ctx context.Context, dbi sqlx.QueryerContext, rsid string) (*model.Order, error) {
	const q = `SELECT
		id, created_at, currency, updated_at, total_price,
		merchant_id, location, status, allowed_payment_methods,
		metadata, valid_for, last_paid_at, expires_at, trial_days
	FROM orders WHERE metadata->>'radomSubscriptionId' = $1`

	result := &model.Order{}
	if err := sqlx.GetContext(ctx, dbi, result, q, rsid); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrOrderNotFound
		}

		return nil, err
	}

	return result, nil
}

// Create creates an order with the data in req.
func (r *Order) Create(ctx context.Context, dbi sqlx.QueryerContext, oreq *model.OrderNew) (*model.Order, error) {
	const q = `INSERT INTO orders
		(total_price, merchant_id, status, currency, location, allowed_payment_methods, valid_for)
	VALUES ($1, $2, $3, $4, $5, $6, $7)
	RETURNING id, created_at, currency, updated_at, total_price, merchant_id, location, status, allowed_payment_methods, valid_for`

	result := &model.Order{}
	if err := dbi.QueryRowxContext(
		ctx,
		q,
		oreq.TotalPrice,
		oreq.MerchantID,
		oreq.Status,
		oreq.Currency,
		oreq.Location,
		oreq.AllowedPaymentMethods,
		oreq.ValidFor,
	).StructScan(result); err != nil {
		return nil, err
	}

	return result, nil
}

// SetLastPaidAt sets last_paid_at to when.
func (r *Order) SetLastPaidAt(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
	const q = `UPDATE orders SET last_paid_at = $2 WHERE id = $1`

	return r.execUpdate(ctx, dbi, q, id, when)
}

// SetTrialDays sets trial_days to ndays.
func (r *Order) SetTrialDays(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, ndays int64) error {
	const q = `UPDATE orders SET trial_days = $2, updated_at = now() WHERE id = $1`

	return r.execUpdate(ctx, dbi, q, id, ndays)
}

// SetStatus sets status to status.
func (r *Order) SetStatus(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, status string) error {
	const q = `UPDATE orders SET status = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $1`

	return r.execUpdate(ctx, dbi, q, id, status)
}

// GetTimeBounds returns valid_for and last_paid_at for the order.
func (r *Order) GetTimeBounds(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (model.OrderTimeBounds, error) {
	const q = `SELECT valid_for, last_paid_at FROM orders WHERE id = $1`

	var result model.OrderTimeBounds
	if err := sqlx.GetContext(ctx, dbi, &result, q, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.EmptyOrderTimeBounds(), model.ErrOrderNotFound
		}

		return model.EmptyOrderTimeBounds(), err
	}

	return result, nil
}

// GetExpiresAtAfterISOPeriod returns a new value for expires_at that is last_paid_at plus ISO period.
//
// It falls back to now() when last_paid_at is NULL.
// It uses the maximum of the order items' valid_for_iso as inverval, and falls back to 1 month.
func (r *Order) GetExpiresAtAfterISOPeriod(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (time.Time, error) {
	const q = `SELECT COALESCE(last_paid_at, now()) +
	(SELECT COALESCE(MAX(valid_for_iso::interval), interval '1 month') FROM order_items WHERE order_id = $2)
	AS expires_at
	FROM orders WHERE id = $1`

	var result time.Time
	if err := sqlx.GetContext(ctx, dbi, &result, q, id, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, model.ErrOrderNotFound
		}

		return time.Time{}, err
	}

	return result, nil
}

// SetExpiresAt sets expires_at.
func (r *Order) SetExpiresAt(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
	const q = `UPDATE orders SET updated_at = CURRENT_TIMESTAMP, expires_at = $2 WHERE id = $1`

	return r.execUpdate(ctx, dbi, q, id, when)
}

// UpdateMetadata _sets_ metadata to data.
func (r *Order) UpdateMetadata(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, data datastore.Metadata) error {
	const q = `UPDATE orders SET metadata = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $1`

	return r.execUpdate(ctx, dbi, q, id, data)
}

// AppendMetadata sets value by key to order's metadata, and might create metadata if it was missing.
func (r *Order) AppendMetadata(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error {
	const q = `UPDATE orders
	SET metadata = COALESCE(metadata||jsonb_build_object($2::text, $3::text), metadata, jsonb_build_object($2::text, $3::text)),
	updated_at = CURRENT_TIMESTAMP WHERE id = $1`

	return r.execUpdate(ctx, dbi, q, id, key, val)
}

// AppendMetadataInt sets int value by key to order's metadata, and might create metadata if it was missing.
func (r *Order) AppendMetadataInt(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key string, val int) error {
	const q = `UPDATE orders
	SET metadata = COALESCE(metadata||jsonb_build_object($2::text, $3::integer), metadata, jsonb_build_object($2::text, $3::integer)),
	updated_at = CURRENT_TIMESTAMP where id = $1`

	return r.execUpdate(ctx, dbi, q, id, key, val)
}

// AppendMetadataInt64 sets int value by key to order's metadata, and might create metadata if it was missing.
func (r *Order) AppendMetadataInt64(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key string, val int64) error {
	const q = `UPDATE orders
	SET metadata = COALESCE(metadata||jsonb_build_object($2::text, $3::integer), metadata, jsonb_build_object($2::text, $3::integer)),
	updated_at = CURRENT_TIMESTAMP where id = $1`

	return r.execUpdate(ctx, dbi, q, id, key, val)
}

// GetExpiredStripeCheckoutSessionID returns stripeCheckoutSessionId if it's found and expired.
func (r *Order) GetExpiredStripeCheckoutSessionID(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) (string, error) {
	const q = `SELECT metadata->>'stripeCheckoutSessionId' AS checkout_session
	FROM orders
	WHERE id = $1 AND metadata IS NOT NULL AND status='pending' AND updated_at<now() - interval '1 hour'`

	var sessID *string
	if err := sqlx.GetContext(ctx, dbi, &sessID, q, orderID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", model.ErrExpiredStripeCheckoutSessionIDNotFound
		}

		return "", err
	}

	var result string
	if sessID != nil {
		result = *sessID
	}

	return result, nil
}

// HasExternalID indicates whether an order with the metadata.externalID exists.
func (r *Order) HasExternalID(ctx context.Context, dbi sqlx.QueryerContext, extID string) (bool, error) {
	const q = `SELECT true
	FROM orders
	WHERE metadata->>'externalID' = $1 AND metadata IS NOT NULL`

	var result bool
	if err := sqlx.GetContext(ctx, dbi, &result, q, extID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}

		return false, err
	}

	return result, nil
}

// GetMetadata returns metadata of the order.
func (r *Order) GetMetadata(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (datastore.Metadata, error) {
	const q = `SELECT metadata
	FROM orders
	WHERE id = $1 AND metadata IS NOT NULL`

	result := datastore.Metadata{}
	if err := sqlx.GetContext(ctx, dbi, &result, q, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrOrderNotFound
		}

		return nil, err
	}

	return result, nil
}

func (r *Order) IncrementNumPayFailed(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID) error {
	const q = `UPDATE orders
	SET metadata['numPaymentFailed'] = to_jsonb(COALESCE((metadata->>'numPaymentFailed')::integer, 0)+1)
	WHERE id = $1`

	return r.execUpdate(ctx, dbi, q, id)
}

func (r *Order) execUpdate(ctx context.Context, dbi sqlx.ExecerContext, q string, args ...interface{}) error {
	result, err := dbi.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}

	numAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if numAffected == 0 {
		return model.ErrNoRowsChangedOrder
	}

	return nil
}
