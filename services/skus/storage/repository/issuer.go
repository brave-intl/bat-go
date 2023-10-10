package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"

	"github.com/brave-intl/bat-go/services/skus/model"
)

type Issuer struct{}

func NewIssuer() *Issuer { return &Issuer{} }

func (r *Issuer) GetByMerchID(ctx context.Context, dbi sqlx.QueryerContext, merchID string) (*model.Issuer, error) {
	const q = `SELECT id, created_at, merchant_id, public_key
	FROM order_cred_issuers WHERE merchant_id = $1`

	result := &model.Issuer{}
	if err := sqlx.GetContext(ctx, dbi, result, q, merchID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrIssuerNotFound
		}

		return nil, err
	}

	return result, nil
}

func (r *Issuer) GetByPubKey(ctx context.Context, dbi sqlx.QueryerContext, pubKey string) (*model.Issuer, error) {
	const q = `SELECT id, created_at, merchant_id, public_key
	FROM order_cred_issuers WHERE public_key = $1`

	result := &model.Issuer{}
	if err := sqlx.GetContext(ctx, dbi, result, q, pubKey); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrIssuerNotFound
		}

		return nil, err
	}

	return result, nil
}

func (r *Issuer) Create(ctx context.Context, dbi sqlx.QueryerContext, req model.IssuerNew) (*model.Issuer, error) {
	const q = `INSERT INTO order_cred_issuers (merchant_id, public_key)
	VALUES ($1, $2)
	RETURNING id, created_at, merchant_id, public_key`

	result := &model.Issuer{}
	if err := dbi.QueryRowxContext(ctx, q, req.MerchantID, req.PublicKey).StructScan(result); err != nil {
		return nil, err
	}

	return result, nil
}
