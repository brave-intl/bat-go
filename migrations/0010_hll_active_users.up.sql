CREATE EXTENSION IF NOT EXISTS hll;

-- Create the destination table
CREATE TABLE IF NOT EXISTS daily_unique_metrics (
  date          DATE NOT NULL,
  activity_type TEXT NOT NULL,
  wallets       HLL,
  PRIMARY KEY   (date, activity_type)
);

ALTER TABLE daily_unique_metrics
ADD CONSTRAINT check_activity_type
CHECK(activity_type in ('active'));
