CREATE TABLE IF NOT EXISTS order_item_history (
    id bigserial PRIMARY KEY,
    operation text NOT NULL,
    executed_by text NOT NULL DEFAULT current_user,
    recorded_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
    order_item_id uuid NOT NULL,
    order_id uuid NOT NULL,
    value_before jsonb,
    value_after jsonb
);

CREATE OR REPLACE FUNCTION save_order_item_history() RETURNS TRIGGER AS $$
    BEGIN
        IF (TG_OP = 'INSERT') THEN
            INSERT INTO order_item_history(operation, order_item_id, order_id, value_after)
            VALUES (TG_OP, NEW.id, NEW.order_id, row_to_json(NEW)::jsonb);

            RETURN NEW;

        ELSIF (TG_OP = 'UPDATE') THEN
            INSERT INTO order_item_history(operation, order_item_id, order_id, value_before, value_after)
            VALUES (TG_OP, NEW.id, NEW.order_id, row_to_json(OLD)::jsonb, row_to_json(NEW)::jsonb);

            RETURN NEW;

        ELSIF (TG_OP = 'DELETE') THEN
            INSERT INTO order_item_history(operation, order_item_id, order_id, value_before)
            VALUES (TG_OP, OLD.id, OLD.order_id, row_to_json(OLD)::jsonb);

            RETURN OLD;
        END IF;
    END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER handle_order_item_change
AFTER INSERT OR UPDATE OR DELETE ON order_items FOR EACH ROW EXECUTE PROCEDURE save_order_item_history();
