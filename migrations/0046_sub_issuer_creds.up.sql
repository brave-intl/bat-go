alter table order_creds add column valid_from timestamp with time zone default null;
alter table order_creds add column valid_to timestamp with time zone default null;
alter table order_creds add column credential_type text default null;
