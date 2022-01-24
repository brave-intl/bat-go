DROP VIEW normalized_claim_creds;

ALTER TABLE public.claim_creds DROP COLUMN created_at;
ALTER TABLE public.claim_creds DROP COLUMN updated_at;
