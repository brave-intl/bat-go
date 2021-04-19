--- Updated at for claims table
alter table claims drop column updated_at;

--- Updated at for claim drain table
alter table claim_drain drop column updated_at;

--- link claim_id to claim_drain item
alter table claim_drain drop column claim_id;

--- claim_type (dd, vg)
alter table claims drop column claim_type;

alter table claims drop column drained_at;

