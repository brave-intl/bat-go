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

// GetByExternalID retrieves the order by metadata.externalID.
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

// GetOrderItem retrieves the order item by the given id.
func (r *Order) GetOrderItem(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.OrderItem, error) {
	const q = `SELECT
		id, order_id, sku, created_at, updated_at, currency,
		quantity, price, (quantity * price) as subtotal,
		location, description, credential_type,metadata, valid_for_iso, issuance_interval
	FROM order_items WHERE id = $1`

	result := &model.OrderItem{}
	if err := sqlx.GetContext(ctx, dbi, result, q, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrOrderItemNotFound
		}

		return nil, err
	}

	return result, nil
}

func (r *Order) Create(
	ctx context.Context,
	dbi sqlx.ExtContext,
	totalPrice decimal.Decimal,
	merchantID, status, currency, location string,
	validFor *time.Duration,
	items []model.OrderItem,
	paymentMethods *model.Methods,
) (*model.Order, error) {
	const q = `INSERT INTO orders
		(total_price, merchant_id, status, currency, location, allowed_payment_methods, valid_for)
	VALUES ($1, $2, $3, $4, $5, $6, $7)
	RETURNING id, created_at, currency, updated_at, total_price, merchant_id, location, status, allowed_payment_methods, valid_for`

	result := &model.Order{}
	if err := dbi.QueryRowxContext(ctx, q, totalPrice, merchantID, status, currency, location, pq.Array(*paymentMethods), validFor).StructScan(result); err != nil {
		return nil, err
	}

	// TODO: Handle payment history.

	items, err := r.createOrderItems(ctx, dbi, result.ID, items)
	if err != nil {
		return nil, err
	}

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

func (r *Order) createOrderItems(
	ctx context.Context,
	dbi sqlx.ExtContext,
	orderID uuid.UUID,
	items []model.OrderItem,
) ([]model.OrderItem, error) {
	const q = `INSERT INTO order_items (
		order_id, sku, quantity, price, currency, subtotal, location, description, credential_type, metadata, valid_for, valid_for_iso, issuance_interval
	) VALUES (
		:order_id, :sku, :quantity, :price, :currency, :subtotal, :location, :description, :credential_type, :metadata, :valid_for, :valid_for_iso, :issuance_interval
	) RETURNING id, order_id, sku, created_at, updated_at, currency, quantity, price, location, description, credential_type, (quantity * price) as subtotal, metadata, valid_for`

	model.OrderItemList(items).SetOrderID(orderID)

	rows, err := sqlx.NamedQueryContext(ctx, dbi, q, items)
	if err != nil {
		return nil, err
	}

	result := make([]model.OrderItem, 0, len(items))
	if err := sqlx.StructScan(rows, &result); err != nil {
		return nil, err
	}

	return result, nil
}
