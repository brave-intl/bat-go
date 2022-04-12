ALTER TABLE promotions DROP CONSTRAINT check_value_suggest_per_grant;
ALTER TABLE promotions ALTER COLUMN approximate_value DROP DEFAULT;
ALTER TABLE promotions ALTER COLUMN suggestions_per_grant DROP DEFAULT;
