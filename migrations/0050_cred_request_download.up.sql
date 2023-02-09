alter table time_limited_v2_order_creds add column downloaded_at timestamp default null;
alter table time_limited_v2_order_creds add column request_id text default null;

alter table time_limited_v2_order_creds drop constraint time_limited_v2_order_creds_unique;
alter table time_limited_v2_order_creds add constraint time_limited_v2_order_creds_unique unique (order_id, item_id, valid_to, valid_from, request_id);
