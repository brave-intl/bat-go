--- Updated at for claims table
alter table claims add column updated_at timestamp with time zone;

--- Updated at for claim drain table
alter table claim_drain add column updated_at timestamp;

--- link claim_id to claim_drain item
alter table claim_drain add column claim_id uuid;

--- claim_type (dd, vg)
alter table claims add column claim_type text;

alter table claims add column drained_at timestamp;


create or replace function update_updated_at_claims()
  returns trigger
as
$body$
  begin
    new.updated_at = current_timestamp;
    return new;
  end;
$body$
language plpgsql;

create trigger claims_updated_at
    before update on claims
    for each row
    execute procedure update_updated_at_claims();

create trigger claim_drain_updated_at
    before update on claim_drain
    for each row
    execute procedure update_updated_at_claims();

