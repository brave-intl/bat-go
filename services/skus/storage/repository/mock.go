package repository

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"

	"github.com/brave-intl/bat-go/libs/datastore"
	"github.com/brave-intl/bat-go/services/skus/model"
)

type MockOrder struct {
	FnGet             func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error)
	FnGetByExternalID func(ctx context.Context, dbi sqlx.QueryerContext, extID string) (*model.Order, error)
	FnCreate          func(ctx context.Context, dbi sqlx.QueryerContext, oreq *model.OrderNew) (*model.Order, error)
	FnSetStatus       func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, status string) error
	FnSetExpiresAt    func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error
	FnSetLastPaidAt   func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error
}

func (r *MockOrder) Get(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
	if r.FnGet == nil {
		result := &model.Order{
			ID: uuid.NewV4(),
		}

		return result, nil
	}

	return r.FnGet(ctx, dbi, id)
}

func (r *MockOrder) GetByExternalID(ctx context.Context, dbi sqlx.QueryerContext, extID string) (*model.Order, error) {
	if r.FnGetByExternalID == nil {
		result := &model.Order{
			ID: uuid.NewV4(),
		}

		return result, nil
	}

	return r.FnGetByExternalID(ctx, dbi, extID)
}

func (r *MockOrder) Create(ctx context.Context, dbi sqlx.QueryerContext, oreq *model.OrderNew) (*model.Order, error) {
	if r.FnCreate == nil {
		result := &model.Order{
			ID:                    uuid.NewV4(),
			MerchantID:            oreq.MerchantID,
			Currency:              oreq.Currency,
			Status:                oreq.Status,
			Location:              datastore.NullString{NullString: oreq.Location},
			TotalPrice:            oreq.TotalPrice,
			AllowedPaymentMethods: oreq.AllowedPaymentMethods,
			ValidFor:              oreq.ValidFor,
		}

		return result, nil
	}

	return r.FnCreate(ctx, dbi, oreq)
}

func (r *MockOrder) SetStatus(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, status string) error {
	if r.FnSetStatus == nil {
		return nil
	}

	return r.FnSetStatus(ctx, dbi, id, status)
}

func (r *MockOrder) SetExpiresAt(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
	if r.FnSetExpiresAt == nil {
		return nil
	}

	return r.FnSetExpiresAt(ctx, dbi, id, when)
}

func (r *MockOrder) SetLastPaidAt(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
	if r.FnSetLastPaidAt == nil {
		return nil
	}

	return r.FnSetLastPaidAt(ctx, dbi, id, when)
}

type MockOrderItem struct {
	FnGet           func(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.OrderItem, error)
	FnFindByOrderID func(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) ([]model.OrderItem, error)
	FnInsertMany    func(ctx context.Context, dbi sqlx.ExtContext, items ...model.OrderItem) ([]model.OrderItem, error)
}

func (r *MockOrderItem) Get(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.OrderItem, error) {
	if r.FnGet == nil {
		return &model.OrderItem{ID: id}, nil
	}

	return r.FnGet(ctx, dbi, id)
}

func (r *MockOrderItem) FindByOrderID(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) ([]model.OrderItem, error) {
	if r.FnFindByOrderID == nil {
		return []model.OrderItem{{ID: uuid.Nil, OrderID: orderID}}, nil
	}

	return r.FnFindByOrderID(ctx, dbi, orderID)
}

func (r *MockOrderItem) InsertMany(ctx context.Context, dbi sqlx.ExtContext, items ...model.OrderItem) ([]model.OrderItem, error) {
	if r.FnInsertMany == nil {
		return items, nil
	}

	return r.FnInsertMany(ctx, dbi, items...)
}

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

type MockOrderPayHistory struct {
	FnInsert func(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error
}

func (r *MockOrderPayHistory) Insert(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error {
	if r.FnInsert == nil {
		return nil
	}

	return r.FnInsert(ctx, dbi, id, when)
}

type MockTLV2 struct {
	FnGetCredSubmissionReport func(ctx context.Context, dbi sqlx.QueryerContext, reqID uuid.UUID, creds ...string) (model.TLV2CredSubmissionReport, error)
	FnUniqBatches             func(ctx context.Context, dbi sqlx.QueryerContext, orderID, itemID uuid.UUID, from, to time.Time) (int, error)
	FnDeleteLegacy            func(ctx context.Context, dbi sqlx.ExecerContext, orderID uuid.UUID) error
}

func (r *MockTLV2) GetCredSubmissionReport(ctx context.Context, dbi sqlx.QueryerContext, reqID uuid.UUID, creds ...string) (model.TLV2CredSubmissionReport, error) {
	if r.FnGetCredSubmissionReport == nil {
		return model.TLV2CredSubmissionReport{}, nil
	}

	return r.FnGetCredSubmissionReport(ctx, dbi, reqID, creds...)
}

func (r *MockTLV2) UniqBatches(ctx context.Context, dbi sqlx.QueryerContext, orderID, itemID uuid.UUID, from, to time.Time) (int, error) {
	if r.FnUniqBatches == nil {
		return 0, nil
	}

	return r.FnUniqBatches(ctx, dbi, orderID, itemID, from, to)
}

func (r *MockTLV2) DeleteLegacy(ctx context.Context, dbi sqlx.ExecerContext, orderID uuid.UUID) error {
	if r.FnDeleteLegacy == nil {
		return nil
	}

	return r.FnDeleteLegacy(ctx, dbi, orderID)
}
