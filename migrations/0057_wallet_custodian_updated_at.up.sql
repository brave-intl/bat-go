alter table wallet_custodian add column updated_at timestamp with time zone not null default current_timestamp;
