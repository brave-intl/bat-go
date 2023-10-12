package repository

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"

	"github.com/brave-intl/bat-go/services/skus/model"
)

type OrderPayHistory struct{}

func NewOrderPayHistory() *OrderPayHistory { return &OrderPayHistory{} }

func (r *OrderPayHistory) Insert(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
	const q = `INSERT INTO order_payment_history (order_id, last_paid) VALUES ($1, $2)`

	result, err := dbi.ExecContext(ctx, q, id, when)
	if err != nil {
		return err
	}

	numAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if numAffected == 0 {
		return model.ErrNoRowsChangedOrderPayHistory
	}

	return nil
}
