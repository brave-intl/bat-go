CREATE INDEX CONCURRENTLY order_creds_batch_proof_null_idx ON order_creds (batch_proof) WHERE batch_proof IS NULL;
