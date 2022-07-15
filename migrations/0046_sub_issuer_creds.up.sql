-- adding metadata about the credentials (used first for validity period)
alter table order_creds add column metadata jsonb default null;
alter table order_creds add column credential_type text default null;
