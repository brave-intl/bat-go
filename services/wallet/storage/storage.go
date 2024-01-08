package storage

import (
	"context"
	"database/sql"
	"errors"

	"github.com/brave-intl/bat-go/services/wallet/model"
	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"
)

type Challenge struct{}

func NewChallenge() *Challenge { return &Challenge{} }

// Get retrieves a model.Challenge from the database by the given id.
func (c *Challenge) Get(ctx context.Context, dbi sqlx.QueryerContext, id string) (model.Challenge, error) {
	const q = `select * from challenge where id = $1`

	var result model.Challenge
	if err := sqlx.GetContext(ctx, dbi, &result, q, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return result, model.ErrNotFound
		}
		return result, err
	}

	return result, nil
}

// Insert persists a model.Challenge to the database.
func (c *Challenge) Insert(ctx context.Context, dbi sqlx.ExecerContext, chl model.Challenge) error {
	const q = `insert into challenge (id, created_at, nonce) values($1, $2, $3) on conflict (id) do update set created_at = $2, nonce = $3`

	result, err := dbi.ExecContext(ctx, q, chl.ID, chl.CreatedAt, chl.Nonce)
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

// Delete removes a model.Challenge from the database identified by the ID.
func (c *Challenge) Delete(ctx context.Context, dbi sqlx.ExecerContext, chl model.Challenge) error {
	const q = `delete from challenge where id = $1`

	result, err := dbi.ExecContext(ctx, q, chl.ID)
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
