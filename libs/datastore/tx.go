package datastore

import (
	"context"
	"fmt"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/jmoiron/sqlx"
)

// TxAble - something that is capable of beginning and rolling back a sqlx.Tx
type TxAble interface {
	RollbackTx(*sqlx.Tx)
	BeginTx() (*sqlx.Tx, error)
}

// GetTx will get or create a tx on the context, if created hands back rollback and commit functions
func GetTx(ctx context.Context, ta TxAble) (context.Context, *sqlx.Tx, func(), func() error, error) {
	logger := logging.Logger(ctx, "datastore.GetTx")
	// get tx
	tx, noContextTx := ctx.Value(appctx.DatabaseTransactionCTXKey).(*sqlx.Tx)
	if !noContextTx {
		tx, err := CreateTx(ctx, ta)
		if err != nil || tx == nil {
			logger.Error().Err(err).Msg("error creating tx")
			return ctx, nil, func() {}, func() error { return nil }, fmt.Errorf("failed to create tx: %w", err)
		}
		ctx = context.WithValue(ctx, appctx.DatabaseTransactionCTXKey, tx)
		return ctx, tx, rollbackFn(ctx, ta, tx), commitFn(ctx, tx), nil
	}
	return ctx, tx, func() {}, func() error { return nil }, nil
}

func rollbackFn(ctx context.Context, ta TxAble, tx *sqlx.Tx) func() {
	return func() {
		ta.RollbackTx(tx)
	}
}

func commitFn(ctx context.Context, tx *sqlx.Tx) func() error {
	logger := logging.Logger(ctx, "datastore.commitFn")
	return func() error {
		if err := tx.Commit(); err != nil {
			logger.Error().Err(err).Msg("failed to commit transaction")
			return err
		}
		return nil
	}
}

// CreateTx - helper to create a tx
func CreateTx(ctx context.Context, ta TxAble) (tx *sqlx.Tx, err error) {
	logger := logging.Logger(ctx, "datastore.CreateTx")
	tx, err = ta.BeginTx()
	if err != nil {
		logger.Error().Err(err).
			Msg("error creating transaction")
		return tx, fmt.Errorf("failed to create transaction: %w", err)
	}
	return tx, nil
}
