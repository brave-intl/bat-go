alter table order_creds add column id uuid not null default uuid_generate_v4();
alter table order_creds drop constraint order_creds_pkey;
alter table order_creds add primary key (id);

alter table order_creds add column valid_from timestamp with time zone default null;
alter table order_creds add column valid_to timestamp with time zone default null;
alter table order_creds add column credential_type text default null;
