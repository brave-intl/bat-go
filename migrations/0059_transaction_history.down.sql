DROP TRIGGER IF EXISTS handle_transaction_change ON transactions;

DROP FUNCTION IF EXISTS save_transaction_history;

DROP TABLE IF EXISTS transaction_history;
