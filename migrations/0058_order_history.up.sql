CREATE TABLE IF NOT EXISTS order_history (
    id serial PRIMARY KEY,
    operation text NOT NULL,
    executed_by text NOT NULL DEFAULT current_user,
    recorded_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
    order_id uuid NOT NULL,
    value_before jsonb,
    value_after jsonb
);

CREATE OR REPLACE FUNCTION fn_order_insert() RETURNS TRIGGER AS $$
    BEGIN
        IF TG_OP = 'INSERT' THEN
            INSERT INTO order_history(operation, order_id, value_after)
            VALUES (TG_OP, NEW.id, row_to_json(NEW)::jsonb);

            RETURN NEW;
        END IF;
    END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION fn_order_update() RETURNS TRIGGER AS $$
    BEGIN
        IF TG_OP = 'UPDATE' THEN
            INSERT INTO order_history(operation, order_id, value_before, value_after)
            VALUES (TG_OP, NEW.id, row_to_json(OLD)::jsonb, row_to_json(NEW)::jsonb);

            RETURN NEW;
        END IF;
    END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION fn_order_delete() RETURNS TRIGGER AS $$
    BEGIN
        IF TG_OP = 'DELETE' THEN
            INSERT INTO order_history(operation, order_id, value_before)
            VALUES (TG_OP, OLD.id, row_to_json(OLD)::jsonb);

            RETURN OLD;
        END IF;
    END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER handle_order_insert AFTER INSERT ON orders FOR EACH ROW EXECUTE PROCEDURE fn_order_insert();

CREATE OR REPLACE TRIGGER handle_order_update AFTER UPDATE ON orders FOR EACH ROW EXECUTE PROCEDURE fn_order_update();

CREATE OR REPLACE TRIGGER handle_order_delete AFTER DELETE ON orders FOR EACH ROW EXECUTE PROCEDURE fn_order_delete();
