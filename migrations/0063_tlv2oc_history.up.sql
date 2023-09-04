CREATE TABLE IF NOT EXISTS tlv2_creds_history (
    id bigserial PRIMARY KEY,
    operation text NOT NULL,
    executed_by text NOT NULL DEFAULT current_user,
    recorded_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
    order_id uuid NOT NULL,
    value_before jsonb,
    value_after jsonb
);

CREATE OR REPLACE FUNCTION save_tlv2_creds_history() RETURNS TRIGGER AS $$
    BEGIN
        IF (TG_OP = 'INSERT') THEN
            INSERT INTO tlv2_creds_history(operation, order_id, value_after)
            VALUES (TG_OP, NEW.order_id, row_to_json(NEW)::jsonb);

            RETURN NEW;

        ELSIF (TG_OP = 'UPDATE') THEN
            INSERT INTO tlv2_creds_history(operation, order_id, value_before, value_after)
            VALUES (TG_OP, NEW.order_id, row_to_json(OLD)::jsonb, row_to_json(NEW)::jsonb);

            RETURN NEW;

        ELSIF (TG_OP = 'DELETE') THEN
            INSERT INTO tlv2_creds_history(operation, order_id, value_before)
            VALUES (TG_OP, OLD.order_id, row_to_json(OLD)::jsonb);

            RETURN OLD;
        END IF;
    END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER handle_tlv2_creds_change
AFTER INSERT OR UPDATE OR DELETE ON time_limited_v2_order_creds FOR EACH ROW EXECUTE FUNCTION save_tlv2_creds_history();
