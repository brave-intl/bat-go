alter table wallets add column created_at timestamp with time zone default current_timestamp;

create index concurrently wallets_created_at_idx on wallets(created_at);

alter table wallets add column updated_at timestamp with time zone default current_timestamp;

create index concurrently wallets_updated_at_idx on wallets(updated_at);

create function update_updated_at()
  returns trigger
as
$body$
  begin
    new.updated_at = current_timestamp;
    return new;
  end;
$body$
language plpgsql;

create trigger update_updated_at_on_wallets
  before update on wallets
  for each row
  execute procedure update_updated_at();

update wallets set created_at = current_timestamp;
