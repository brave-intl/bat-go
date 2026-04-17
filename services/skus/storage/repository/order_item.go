package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"

	"github.com/brave-intl/bat-go/services/skus/model"
)

type OrderItem struct{}

func NewOrderItem() *OrderItem { return &OrderItem{} }

// Get retrieves the order item by the given id.
func (r *OrderItem) Get(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.OrderItem, error) {
	const q = `
	SELECT
		id, order_id, sku, sku_variant, created_at, updated_at, currency,
		quantity, price, (quantity * price) as subtotal, location, description, credential_type, metadata,
		valid_for_iso, issuance_interval, max_active_batches_tlv2_creds,
		num_self_extensions, last_self_extension_at
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

// FindByOrderID returns order items for the given orderID.
func (r *OrderItem) FindByOrderID(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) ([]model.OrderItem, error) {
	const q = `
	SELECT
		id, order_id, sku, sku_variant, created_at, updated_at, currency, quantity, price,
		(quantity * price) as subtotal, location, description, credential_type, metadata, valid_for_iso,
		issuance_interval, max_active_batches_tlv2_creds,
		num_self_extensions, last_self_extension_at
	FROM order_items WHERE order_id = $1`

	result := make([]model.OrderItem, 0)
	if err := sqlx.SelectContext(ctx, dbi, &result, q, orderID); err != nil {
		return nil, err
	}

	return result, nil
}

// InsertMany inserts given items and returns the result.
func (r *OrderItem) InsertMany(ctx context.Context, dbi sqlx.ExtContext, items ...model.OrderItem) ([]model.OrderItem, error) {
	if len(items) == 0 {
		return []model.OrderItem{}, nil
	}

	const q = `
	INSERT INTO order_items (
		order_id, sku, sku_variant, quantity, price, currency, subtotal, location, description, credential_type, metadata, valid_for, valid_for_iso, issuance_interval, max_active_batches_tlv2_creds
	) VALUES (
		:order_id, :sku, :sku_variant, :quantity, :price, :currency, :subtotal, :location, :description, :credential_type, :metadata, :valid_for, :valid_for_iso, :issuance_interval, :max_active_batches_tlv2_creds
	) RETURNING id, order_id, sku, sku_variant, created_at, updated_at, currency, quantity, price, location, description, credential_type, (quantity * price) as subtotal, metadata, valid_for, max_active_batches_tlv2_creds, num_self_extensions, last_self_extension_at`

	rows, err := sqlx.NamedQueryContext(ctx, dbi, q, items)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make([]model.OrderItem, 0, len(items))
	if err := sqlx.StructScan(rows, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// LockForUpdate fetches the order item by id under a SELECT … FOR UPDATE row lock.
// Must be called within a transaction.
func (r *OrderItem) LockForUpdate(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.OrderItem, error) {
	const q = `
	SELECT
		id, order_id, sku, sku_variant, created_at, updated_at, currency,
		quantity, price, (quantity * price) as subtotal, location, description, credential_type, metadata,
		valid_for_iso, issuance_interval, max_active_batches_tlv2_creds,
		num_self_extensions, last_self_extension_at
	FROM order_items WHERE id = $1 FOR UPDATE`

	result := &model.OrderItem{}
	if err := sqlx.GetContext(ctx, dbi, result, q, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrOrderItemNotFound
		}

		return nil, err
	}

	return result, nil
}

// ApplyExtension sets the new device limit, increments the self-extension counter, and
// records the timestamp. Must be called within a transaction after LockForUpdate.
func (r *OrderItem) ApplyExtension(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, newLimit int) error {
	const q = `
	UPDATE order_items
	SET max_active_batches_tlv2_creds = $2,
	    num_self_extensions           = num_self_extensions + 1,
	    last_self_extension_at        = NOW()
	WHERE id = $1`

	result, err := dbi.ExecContext(ctx, q, id, newLimit)
	if err != nil {
		return err
	}

	n, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if n == 0 {
		return model.ErrOrderItemNotFound
	}

	return nil
}
