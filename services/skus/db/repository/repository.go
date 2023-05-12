// Package repository provides access to data available in SQL-based data store.
package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"

	"github.com/brave-intl/bat-go/services/skus/model"
)

type Order struct{}

func NewOrder() *Order {
	return &Order{}
}

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

	items, err := r.fetchOrderItems(ctx, dbi, id)
	if err != nil {
		return nil, err
	}

	result.Items = items

	return result, nil
}

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

	items, err := r.fetchOrderItems(ctx, dbi, result.ID)
	if err != nil {
		return nil, err
	}

	result.Items = items

	return result, nil
}

func (r *Order) fetchOrderItems(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) ([]model.OrderItem, error) {
	const q = `SELECT
		id, order_id, sku, created_at, updated_at, currency,
		quantity, price, (quantity * price) as subtotal,
		location, description, credential_type, metadata, valid_for_iso, issuance_interval
	FROM order_items WHERE order_id = $1`

	result := make([]model.OrderItem, 0)
	if err := sqlx.SelectContext(ctx, dbi, &result, q, orderID); err != nil {
		return nil, err
	}

	return result, nil
}
