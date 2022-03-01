--- swap promotions grant reward fields for kafka worker
alter table claims add column transaction_key uuid;
create unique index unique_claim_transaction_key on claims(transaction_key) where transaction_key is not NULL;
insert into wallets (id, provider, provider_id, public_key) values ('00000000-0000-0000-0000-000000000002', 'uphold', '-', '-');
