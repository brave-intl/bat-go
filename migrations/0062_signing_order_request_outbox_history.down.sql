DROP TRIGGER IF EXISTS handle_signing_order_request_change ON signing_order_request_outbox;

DROP FUNCTION IF EXISTS save_signing_order_request_history;

DROP TABLE IF EXISTS signing_order_request_history;
