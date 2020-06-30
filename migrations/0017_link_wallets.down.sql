DROP INDEX wallets_claim_provider_linking;
DROP INDEX wallets_claim_anonymous_address;
ALTER TABLE wallets DROP COLUMN provider_linking_id;
ALTER TABLE wallets RENAME COLUMN anonymous_address TO payout_address;
ALTER TABLE wallets DROP CONSTRAINT check_provider;
ALTER TABLE wallets ADD CONSTRAINT check_provider CHECK (provider IN ('uphold'));