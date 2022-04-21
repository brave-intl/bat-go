-- allow vBAT values up to 1 BAT
ALTER TABLE promotions ADD CONSTRAINT check_value_suggest_per_grant CHECK ((approximate_value / suggestions_per_grant) < 1.0);
ALTER TABLE promotions ALTER COLUMN approximate_value SET DEFAULT 20.0;
ALTER TABLE promotions ALTER COLUMN suggestions_per_grant SET DEFAULT 80.0;
