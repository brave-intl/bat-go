CREATE TABLE IF NOT EXISTS transaction_history (
    id serial PRIMARY KEY,
    operation text NOT NULL,
    executed_by text NOT NULL DEFAULT current_user,
    recorded_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
    transaction_id uuid NOT NULL,
    -- order_id is nullable in the upstream table.
    -- However, application code does not seem to allow that.
    order_id uuid,
    value_before jsonb,
    value_after jsonb
);

CREATE OR REPLACE FUNCTION fn_transaction_insert() RETURNS TRIGGER AS $$
    BEGIN
        IF TG_OP = 'INSERT' THEN
            INSERT INTO transaction_history(operation, transaction_id, order_id, value_after)
            VALUES (TG_OP, NEW.id, NEW.order_id, row_to_json(NEW)::jsonb);

            RETURN NEW;
        END IF;
    END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION fn_transaction_update() RETURNS TRIGGER AS $$
    BEGIN
        IF TG_OP = 'UPDATE' THEN
            INSERT INTO transaction_history(operation, transaction_id, order_id, value_before, value_after)
            VALUES (TG_OP, NEW.id, NEW.order_id, row_to_json(OLD)::jsonb, row_to_json(NEW)::jsonb);

            RETURN NEW;
        END IF;
    END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION fn_transaction_delete() RETURNS TRIGGER AS $$
    BEGIN
        IF TG_OP = 'DELETE' THEN
            INSERT INTO transaction_history(operation, transaction_id, order_id, value_before)
            VALUES (TG_OP, OLD.id, OLD.order_id, row_to_json(OLD)::jsonb);

            RETURN OLD;
        END IF;
    END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER handle_transaction_insert AFTER INSERT ON transactions FOR EACH ROW EXECUTE PROCEDURE fn_transaction_insert();

CREATE OR REPLACE TRIGGER handle_transaction_update AFTER UPDATE ON transactions FOR EACH ROW EXECUTE PROCEDURE fn_transaction_update();

CREATE OR REPLACE TRIGGER handle_transaction_delete AFTER DELETE ON transactions FOR EACH ROW EXECUTE PROCEDURE fn_transaction_delete();
