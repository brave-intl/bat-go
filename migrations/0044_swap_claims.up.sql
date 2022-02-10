--- address_id pk
alter table claims add column address_id text;

--- claim_address_id index on address_id
create index claim_address_id on claims(address_id);
