CREATE INDEX CONCURRENTLY claim_drain_pending_idx ON claim_drain (status, updated_at) WHERE status != 'complete' AND status IS NOT NULL;
