ALTER TABLE time_limited_v2_order_creds DROP CONSTRAINT IF EXISTS time_limited_v2_order_creds_unique;

ALTER TABLE time_limited_v2_order_creds ADD CONSTRAINT time_limited_v2_order_creds_unique UNIQUE (order_id, item_id, request_id, valid_to, valid_from);
