package repository

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"

	"github.com/brave-intl/bat-go/services/skus/model"
)

type MockIssuer struct {
	FnGetByMerchID func(ctx context.Context, dbi sqlx.QueryerContext, merchID string) (*model.Issuer, error)
	FnGetByPubKey  func(ctx context.Context, dbi sqlx.QueryerContext, pubKey string) (*model.Issuer, error)
	FnCreate       func(ctx context.Context, dbi sqlx.QueryerContext, req model.IssuerNew) (*model.Issuer, error)
}

func (r *MockIssuer) GetByMerchID(ctx context.Context, dbi sqlx.QueryerContext, merchID string) (*model.Issuer, error) {
	if r.FnGetByMerchID == nil {
		result := &model.Issuer{
			ID:         uuid.NewV4(),
			MerchantID: merchID,
			PublicKey:  "public_key",
			CreatedAt:  time.Now().UTC(),
		}

		return result, nil
	}

	return r.FnGetByMerchID(ctx, dbi, merchID)
}

func (r *MockIssuer) GetByPubKey(ctx context.Context, dbi sqlx.QueryerContext, pubKey string) (*model.Issuer, error) {
	if r.FnGetByPubKey == nil {
		result := &model.Issuer{
			ID:         uuid.NewV4(),
			MerchantID: "merchant_id",
			PublicKey:  pubKey,
			CreatedAt:  time.Now().UTC(),
		}

		return result, nil
	}

	return r.FnGetByPubKey(ctx, dbi, pubKey)
}

func (r *MockIssuer) Create(ctx context.Context, dbi sqlx.QueryerContext, req model.IssuerNew) (*model.Issuer, error) {
	if r.FnCreate == nil {
		result := &model.Issuer{
			ID:         uuid.NewV4(),
			MerchantID: req.MerchantID,
			PublicKey:  req.PublicKey,
			CreatedAt:  time.Now().UTC(),
		}

		return result, nil
	}

	return r.FnCreate(ctx, dbi, req)
}
