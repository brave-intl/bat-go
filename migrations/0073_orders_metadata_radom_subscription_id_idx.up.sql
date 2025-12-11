CREATE INDEX CONCURRENTLY IF NOT EXISTS orders_metadata_radom_subscription_id_idx ON orders ((metadata->>'radomSubscriptionId')) WHERE metadata->>'radomSubscriptionId' IS NOT NULL;
