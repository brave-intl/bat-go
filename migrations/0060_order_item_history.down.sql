DROP TRIGGER IF EXISTS handle_order_item_change ON order_items;

DROP FUNCTION IF EXISTS save_order_item_history;

DROP TABLE IF EXISTS order_item_history;
