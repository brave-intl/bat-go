package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"

	"github.com/brave-intl/bat-go/services/skus/model"
)

type TLV2 struct{}

func NewTLV2() *TLV2 { return &TLV2{} }

func (r *TLV2) GetCredSubmissionReport(ctx context.Context, dbi sqlx.QueryerContext, reqID uuid.UUID, creds ...string) (model.TLV2CredSubmissionReport, error) {
	if len(creds) == 0 {
		return model.TLV2CredSubmissionReport{}, model.ErrTLV2InvalidCredNum
	}

	const q = `SELECT EXISTS(
		SELECT 1 FROM time_limited_v2_order_creds WHERE blinded_creds->>0 = $2
	) AS submitted, EXISTS(
		SELECT 1 FROM time_limited_v2_order_creds WHERE blinded_creds->>0 != $2 AND request_id = $1
	) AS req_id_mismatch`

	result := model.TLV2CredSubmissionReport{}
	if err := sqlx.GetContext(ctx, dbi, &result, q, reqID, creds[0]); err != nil {
		return model.TLV2CredSubmissionReport{}, err
	}

	return result, nil
}
