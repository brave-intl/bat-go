CREATE INDEX CONCURRENTLY IF NOT EXISTS orders_metadata_checkout_session_id_idx ON orders ((metadata->>'stripeCheckoutSessionId')) WHERE metadata->>'stripeCheckoutSessionId' IS NOT NULL;
