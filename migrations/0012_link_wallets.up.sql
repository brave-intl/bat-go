ALTER TABLE wallets ADD COLUMN provider_linking_id uuid;
ALTER TABLE wallets ADD COLUMN anonymous_address uuid;
CREATE INDEX wallet_claim_provider_linking_idx on wallets(provider_linking_id);
CREATE INDEX wallet_claim_anonymous_address_idx on wallets(anonymous_address);
