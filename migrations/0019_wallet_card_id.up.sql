ALTER TABLE wallets ADD COLUMN user_deposit_destination text not null default '';
create index wallet_user_deposit_destination_idx on wallets(user_deposit_destination);

CREATE INDEX wallets_public_key ON wallets(public_key);
