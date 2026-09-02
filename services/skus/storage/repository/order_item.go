package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
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

// GetForUpdate retrieves the order item by the given id and locks the row for update.
func (r *OrderItem) GetForUpdate(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.OrderItem, error) {
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

func (r *OrderItem) ApplyExtensionCAS(ctx context.Context, dbi sqlx.ExtContext, id uuid.UUID, expected *time.Time, newLimit int) error {
	const q = `
	UPDATE order_items
	SET max_active_batches_tlv2_creds = $2,
	    num_self_extensions           = num_self_extensions + 1,
	    last_self_extension_at        = NOW()
	WHERE id = $1
	  AND last_self_extension_at IS NOT DISTINCT FROM $3`

	result, err := dbi.ExecContext(ctx, q, id, newLimit, expected)
	if err != nil {
		if isErrExtensionInvalidLimit(err) {
			return model.ErrExtensionInvalidLimit
		}

		return err
	}

	n, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if n == 0 {
		return model.ErrExtensionConflict
	}

	return nil
}

func (r *OrderItem) UpdateMaxActiveBatchesTLV2Creds(ctx context.Context, dbi sqlx.ExtContext, id uuid.UUID, maxActiveBatches int, numSelfExt int, now time.Time) error {
	const q = `
	UPDATE order_items
	SET max_active_batches_tlv2_creds = $2,
	    num_self_extensions           = $3,
	    last_self_extension_at        = $4
	WHERE id = $1`

	if _, err := dbi.ExecContext(ctx, q, id, maxActiveBatches, numSelfExt, now); err != nil {
		if isErrExtensionInvalidLimit(err) {
			return model.ErrExtensionInvalidLimit
		}

		return err
	}

	return nil
}

func isErrExtensionInvalidLimit(err error) bool {
	var perr *pq.Error
	if !errors.As(err, &perr) {
		return false
	}

	if perr.Table != "order_items" {
		return false
	}

	if perr.Severity != "ERROR" {
		return false
	}

	if perr.Code != pq.ErrorCode("23514") {
		return false
	}

	return perr.Constraint == "order_items_max_active_batches_tlv2_creds_sanity"
}
