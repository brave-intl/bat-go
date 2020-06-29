CREATE TABLE IF NOT EXISTS bat_loss_events (
  id uuid PRIMARY KEY NOT NULL DEFAULT uuid_generate_v4(),
  wallet_id uuid NOT NULL,
  report_id INT NOT NULL,
  amount NUMERIC(28, 18) NOT NULL
);

CREATE INDEX wallet_idx ON bat_loss_events(wallet_id);
CREATE UNIQUE INDEX wallet_report_idx ON bat_loss_events(wallet_id, report_id);
