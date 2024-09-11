package repository

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"
)

type OrderPayHistory struct{}

func NewOrderPayHistory() *OrderPayHistory { return &OrderPayHistory{} }

func (r *OrderPayHistory) Insert(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
	const q = `INSERT INTO order_payment_history (order_id, last_paid) VALUES ($1, $2) ON CONFLICT DO NOTHING`

	if _, err := dbi.ExecContext(ctx, q, id, when); err != nil {
		return err
	}

	return nil
}
