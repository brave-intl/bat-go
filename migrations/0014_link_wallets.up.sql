ALTER TABLE wallets ADD COLUMN provider_linking_id uuid;
ALTER TABLE wallets RENAME COLUMN payout_address TO anonymous_address;
CREATE INDEX wallets_claim_provider_linking ON wallets(provider_linking_id);
CREATE INDEX wallets_claim_anonymous_address ON wallets(anonymous_address);
ALTER TABLE wallets DROP CONSTRAINT check_provider;
ALTER TABLE wallets ADD CONSTRAINT check_provider CHECK (provider IN ('uphold', 'brave'));