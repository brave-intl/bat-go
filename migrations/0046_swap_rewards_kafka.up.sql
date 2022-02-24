--- swap promotions grant reward fields for kafka worker
alter table claims add column transaction_key uuid;
create unique index unique_claim_hack_idx on claims(transaction_key) where transaction_key is not NULL;
