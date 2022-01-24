DROP VIEW claim_creds_redshift_staging;

ALTER TABLE public.claim_creds DROP COLUMN created_at;
ALTER TABLE public.claim_creds DROP COLUMN updated_at;
