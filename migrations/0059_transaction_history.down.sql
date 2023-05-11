DROP TRIGGER IF EXISTS handle_transaction_insert ON transactions;
DROP TRIGGER IF EXISTS handle_transaction_update ON transactions;
DROP TRIGGER IF EXISTS handle_transaction_delete ON transactions;

DROP FUNCTION IF EXISTS fn_transaction_insert;
DROP FUNCTION IF EXISTS fn_transaction_update;
DROP FUNCTION IF EXISTS fn_transaction_delete;

DROP TABLE IF EXISTS transaction_history;
