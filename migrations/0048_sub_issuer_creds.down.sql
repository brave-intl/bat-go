drop table if exists time_limited_v2_order_creds;
drop table if exists signing_order_request_outbox;

drop index if exists time_limited_v2_order_creds_order_id_idx;
drop index if exists time_limited_v2_order_creds_item_id_idx;
drop index if exists signing_order_request_outbox_order_id_request_id_idx;
drop index if exists signing_order_request_outbox_order_id_idx;
drop index if exists signing_order_request_outbox_order_id_item_id_idx;
