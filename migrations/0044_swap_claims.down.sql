--- address_id pk - implicitly drops claim_address_id index too
alter table claims drop column address_id;
