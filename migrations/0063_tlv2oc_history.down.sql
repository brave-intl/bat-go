DROP TRIGGER IF EXISTS handle_tlv2_creds_change ON time_limited_v2_order_creds;

DROP FUNCTION IF EXISTS save_tlv2_creds_history;

DROP TABLE IF EXISTS tlv2_creds_history;
