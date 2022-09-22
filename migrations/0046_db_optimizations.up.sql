CREATE INDEX claim_drain_pending ON claim_drain (status, updated_at) WHERE status LIKE '%pending%';
CREATE INDEX vote_drain_processed_erred ON vote_drain (processed, erred) WHERE processed=false AND erred=false;
CREATE INDEX suggestion_drain_not_erred ON suggestion_drain (erred) WHERE NOT erred;
CREATE INDEX order_creds_batch_proof_null ON order_creds (batch_proof) WHERE batch_proof IS NULL;
