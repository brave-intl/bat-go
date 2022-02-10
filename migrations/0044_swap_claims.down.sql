--- address_id pk
alter table claims drop column address_id;

--- claim_address_id index on address_id
DROP INDEX claim_address_id ON claims;
