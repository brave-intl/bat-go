// Package repository provides access to data available in SQL-based data store.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"

	"github.com/brave-intl/bat-go/services/skus/model"
)

type Order struct{}

func NewOrder() *Order { return &Order{} }

// Get retrieves the order for the given id.
func (r *Order) Get(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
	const q = `
	SELECT
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
	const q = `
	SELECT
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

// Create creates an order with the given inputs.
func (r *Order) Create(
	ctx context.Context,
	dbi sqlx.ExtContext,
	totalPrice decimal.Decimal,
	merchantID, status, currency, location string,
	paymentMethods *model.Methods,
	validFor *time.Duration,
) (*model.Order, error) {
	const q = `
	INSERT INTO orders
		(total_price, merchant_id, status, currency, location, allowed_payment_methods, valid_for)
	VALUES ($1, $2, $3, $4, $5, $6, $7)
	RETURNING id, created_at, currency, updated_at, total_price, merchant_id, location, status, allowed_payment_methods, valid_for`

	result := &model.Order{}
	if err := dbi.QueryRowxContext(
		ctx,
		q,
		totalPrice,
		merchantID,
		status,
		currency,
		location,
		pq.Array(*paymentMethods),
		validFor,
	).StructScan(result); err != nil {
		return nil, err
	}

	// TODO: Handle payment history.

	// itemsNew, err := r.createOrderItems(ctx, dbi, result.ID, items)
	// if err != nil {
	// 	return nil, err
	// }

	// result.Items = itemsNew

	return result, nil
}

func (r *Order) SetLastPaidAt(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
	const q = `UPDATE orders SET last_paid_at = $2 WHERE id = $1`

	result, err := dbi.ExecContext(ctx, q, id, when)
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
