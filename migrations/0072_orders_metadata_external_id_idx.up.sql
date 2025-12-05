CREATE INDEX CONCURRENTLY IF NOT EXISTS orders_metadata_external_id_idx ON orders ((metadata->>'externalID')) WHERE metadata->>'externalID' IS NOT NULL;
