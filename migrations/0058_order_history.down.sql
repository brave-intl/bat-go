DROP TRIGGER IF EXISTS handle_order_insert ON orders;
DROP TRIGGER IF EXISTS handle_order_update ON orders;
DROP TRIGGER IF EXISTS handle_order_delete ON orders;

DROP FUNCTION IF EXISTS fn_order_insert;
DROP FUNCTION IF EXISTS fn_order_update;
DROP FUNCTION IF EXISTS fn_order_delete;

DROP TABLE IF EXISTS order_history;
