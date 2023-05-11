CREATE TABLE IF NOT EXISTS order_item_history (
    id serial PRIMARY KEY,
    operation text NOT NULL,
    executed_by text NOT NULL DEFAULT current_user,
    recorded_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
    order_item_id uuid NOT NULL,
    order_id uuid NOT NULL,
    value_before jsonb,
    value_after jsonb
);

CREATE OR REPLACE FUNCTION fn_order_item_insert() RETURNS TRIGGER AS $$
    BEGIN
        IF TG_OP = 'INSERT' THEN
            INSERT INTO order_item_history(operation, order_item_id, order_id, value_after)
            VALUES (TG_OP, NEW.id, NEW.order_id, row_to_json(NEW)::jsonb);

            RETURN NEW;
        END IF;
    END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION fn_order_item_update() RETURNS TRIGGER AS $$
    BEGIN
        IF TG_OP = 'UPDATE' THEN
            INSERT INTO order_item_history(operation, order_item_id, order_id, value_before, value_after)
            VALUES (TG_OP, NEW.id, NEW.order_id, row_to_json(OLD)::jsonb, row_to_json(NEW)::jsonb);

            RETURN NEW;
        END IF;
    END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION fn_order_item_delete() RETURNS TRIGGER AS $$
    BEGIN
        IF TG_OP = 'DELETE' THEN
            INSERT INTO order_item_history(operation, order_item_id, order_id, value_before)
            VALUES (TG_OP, OLD.id, OLD.order_id, row_to_json(OLD)::jsonb);

            RETURN OLD;
        END IF;
    END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER handle_order_item_insert AFTER INSERT ON order_items FOR EACH ROW EXECUTE PROCEDURE fn_order_item_insert();

CREATE OR REPLACE TRIGGER handle_order_item_update AFTER UPDATE ON order_items FOR EACH ROW EXECUTE PROCEDURE fn_order_item_update();

CREATE OR REPLACE TRIGGER handle_order_item_delete AFTER DELETE ON order_items FOR EACH ROW EXECUTE PROCEDURE fn_order_item_delete();
