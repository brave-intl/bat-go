CREATE INDEX CONCURRENTLY vote_drain_processed_erred_idx ON vote_drain (processed, erred) WHERE processed=false AND erred=false;
