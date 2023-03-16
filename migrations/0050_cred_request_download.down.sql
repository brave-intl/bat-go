alter table time_limited_v2_order_creds drop constraint time_limited_v2_order_creds_unique;
alter table time_limited_v2_order_creds add constraint time_limited_v2_order_creds_unique unique (order_id, item_id, valid_to, valid_from);

alter table time_limited_v2_order_creds drop column downloaded_at;
alter table time_limited_v2_order_creds drop column request_id;

