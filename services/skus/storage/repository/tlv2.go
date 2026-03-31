package repository

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	uuid "github.com/satori/go.uuid"

	"github.com/brave-intl/bat-go/services/skus/model"
)

type TLV2 struct{}

func NewTLV2() *TLV2 { return &TLV2{} }

func (r *TLV2) GetCredSubmissionReport(ctx context.Context, dbi sqlx.QueryerContext, orderID, itemID, reqID uuid.UUID, firstBCred string) (model.TLV2CredSubmissionReport, error) {
	const q = `SELECT EXISTS(
		SELECT 1 FROM time_limited_v2_order_creds WHERE order_id=$1 AND item_id=$2 AND blinded_creds->>0 = $4
	) AS submitted, EXISTS(
		SELECT 1 FROM time_limited_v2_order_creds WHERE order_id=$1 AND item_id=$2 AND request_id = $3 AND blinded_creds->>0 != $4
	) AS req_id_mismatch`

	result := model.TLV2CredSubmissionReport{}
	if err := sqlx.GetContext(ctx, dbi, &result, q, orderID, itemID, reqID, firstBCred); err != nil {
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

// ActiveBatchesByOrder returns the currently active credential batches across all items in an
// order, ordered oldest-first by valid_from. Each batch corresponds to one linked device.
func (r *TLV2) ActiveBatchesByOrder(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID, now time.Time) ([]model.TLV2ActiveBatch, error) {
	const q = `SELECT request_id, MIN(valid_from) AS oldest_valid_from
		FROM time_limited_v2_order_creds
		WHERE order_id=$1 AND valid_to > $2
		GROUP BY request_id
		ORDER BY MIN(valid_from) ASC`

	result := []model.TLV2ActiveBatch{}
	if err := sqlx.SelectContext(ctx, dbi, &result, q, orderID, now); err != nil {
		return nil, err
	}

	return result, nil
}

// ActiveBatchesByOrderItem returns the currently active credential batches for a specific
// order item, ordered oldest-first by valid_from. Each batch corresponds to one linked device.
func (r *TLV2) ActiveBatchesByOrderItem(ctx context.Context, dbi sqlx.QueryerContext, orderID, itemID uuid.UUID, now time.Time) ([]model.TLV2ActiveBatch, error) {
	const q = `SELECT request_id, MIN(valid_from) AS oldest_valid_from
		FROM time_limited_v2_order_creds
		WHERE order_id=$1 AND item_id=$2 AND valid_to > $3
		GROUP BY request_id
		ORDER BY MIN(valid_from) ASC`

	result := []model.TLV2ActiveBatch{}
	if err := sqlx.SelectContext(ctx, dbi, &result, q, orderID, itemID, now); err != nil {
		return nil, err
	}

	return result, nil
}

// DeleteCredsByRequestIDs removes time-limited-v2 credentials for the given request IDs
// within an order. Callers should pass a transaction for atomicity with DeleteOutboxByRequestIDs.
func (r *TLV2) DeleteCredsByRequestIDs(ctx context.Context, dbi sqlx.ExecerContext, orderID uuid.UUID, requestIDs []string) error {
	_, err := dbi.ExecContext(ctx,
		`DELETE FROM time_limited_v2_order_creds WHERE order_id=$1 AND request_id = ANY($2)`,
		orderID, pq.Array(requestIDs),
	)

	return err
}

// DeleteOutboxByRequestIDs removes pending signing requests from the outbox for the given
// request IDs within an order. Callers should pass a transaction for atomicity with DeleteCredsByRequestIDs.
func (r *TLV2) DeleteOutboxByRequestIDs(ctx context.Context, dbi sqlx.ExecerContext, orderID uuid.UUID, requestIDs []string) error {
	_, err := dbi.ExecContext(ctx,
		`DELETE FROM signing_order_request_outbox WHERE order_id=$1 AND request_id::text = ANY($2)`,
		orderID, pq.Array(requestIDs),
	)

	return err
}
