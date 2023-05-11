DROP TRIGGER IF EXISTS handle_order_item_insert ON order_items;
DROP TRIGGER IF EXISTS handle_order_item_update ON order_items;
DROP TRIGGER IF EXISTS handle_order_item_delete ON order_items;

DROP FUNCTION IF EXISTS fn_order_item_insert;
DROP FUNCTION IF EXISTS fn_order_item_update;
DROP FUNCTION IF EXISTS fn_order_item_delete;

DROP TABLE IF EXISTS order_item_history;
