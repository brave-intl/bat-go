package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/brave-intl/bat-go/services/wallet/model"
	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"
)

type Challenge struct{}

func NewChallenge() *Challenge { return &Challenge{} }

// Get retrieves a model.Challenge from the database by the given paymentID.
func (c *Challenge) Get(ctx context.Context, dbi sqlx.QueryerContext, paymentID uuid.UUID) (model.Challenge, error) {
	const q = `select * from challenge where payment_id = $1`

	var result model.Challenge
	if err := sqlx.GetContext(ctx, dbi, &result, q, paymentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return result, model.ErrChallengeNotFound
		}
		return result, err
	}

	return result, nil
}

// Upsert persists a model.Challenge to the database.
func (c *Challenge) Upsert(ctx context.Context, dbi sqlx.ExecerContext, chl model.Challenge) error {
	const q = `insert into challenge (payment_id, created_at, nonce) values($1, $2, $3) on conflict (payment_id) do update set created_at = $2, nonce = $3`

	result, err := dbi.ExecContext(ctx, q, chl.PaymentID, chl.CreatedAt, chl.Nonce)
	if err != nil {
		return err
	}

	row, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if row != 1 {
		return model.ErrNotInserted
	}

	return nil
}

// Delete removes a model.Challenge from the database identified by the paymentID.
func (c *Challenge) Delete(ctx context.Context, dbi sqlx.ExecerContext, paymentID uuid.UUID) error {
	const q = `delete from challenge where payment_id = $1`

	result, err := dbi.ExecContext(ctx, q, paymentID)
	if err != nil {
		return err
	}

	row, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if row == 0 {
		return model.ErrNoRowsDeleted
	}

	return nil
}

// DeleteAfter removes model.Challenge's from the database where their created at plus the specified interval it
// less than the current time. The interval should be specified in minutes.
func (c *Challenge) DeleteAfter(ctx context.Context, dbi sqlx.ExecerContext, interval time.Duration) error {
	const q = `delete from challenge where created_at + interval '1 min' * $1 < now()`

	result, err := dbi.ExecContext(ctx, q, interval)
	if err != nil {
		return err
	}

	row, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if row == 0 {
		return model.ErrNoRowsDeleted
	}

	return nil
}

type AllowList struct{}

func NewAllowList() *AllowList { return &AllowList{} }

// GetAllowListEntry retrieves a model.AllowListEntry from the database for the given paymentID.
func (a *AllowList) GetAllowListEntry(ctx context.Context, dbi sqlx.QueryerContext, paymentID uuid.UUID) (model.AllowListEntry, error) {
	const q = `select * from allow_list where payment_id = $1`

	var result model.AllowListEntry
	if err := sqlx.GetContext(ctx, dbi, &result, q, paymentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return result, model.ErrNotFound
		}
		return result, err
	}

	return result, nil
}
