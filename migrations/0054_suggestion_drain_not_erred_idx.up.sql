CREATE INDEX CONCURRENTLY suggestion_drain_not_erred_idx ON suggestion_drain (erred) WHERE NOT erred;
