alter table order_creds drop constraint order_creds_pkey;
alter table order_creds drop column id;
alter table order_creds add primary key (item_id);

alter table order_creds drop column valid_from;
alter table order_creds drop column valid_to;
alter table order_creds drop column credential_type;
