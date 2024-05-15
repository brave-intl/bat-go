package repository

import (
	"context"
	"time"

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

func (r *TLV2) UniqBatches(ctx context.Context, dbi sqlx.QueryerContext, orderID, itemID uuid.UUID, from, to time.Time) (int, error) {
	const q = `SELECT COUNT(DISTINCT request_id) FROM time_limited_v2_order_creds
	WHERE order_id=$1 AND item_id=$2 AND valid_to > $4 AND valid_from < $3;`

	var result int
	if err := sqlx.GetContext(ctx, dbi, &result, q, orderID, itemID, from, to); err != nil {
		return 0, err
	}

	return result, nil
}

// DeleteLegacy deletes creds where request_id matches the item_id.
//
// Most of the time, there will be only one such set of creds for a given period of time
// because there is only one item in an order.
//
// TODO(pavelb): Reconsider this when it's time for Bundles. By that time this method might be gone.
func (r *TLV2) DeleteLegacy(ctx context.Context, dbi sqlx.ExecerContext, orderID uuid.UUID) error {
	const q = `DELETE FROM time_limited_v2_order_creds WHERE order_id=$1 AND request_id=item_id::text;`

	_, err := dbi.ExecContext(ctx, q, orderID)

	return err
}
