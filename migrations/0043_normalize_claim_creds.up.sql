ALTER TABLE public.claim_creds ADD COLUMN created_at timestamp without time zone;
ALTER TABLE public.claim_creds ADD COLUMN updated_at timestamp without time zone;

create view claim_creds_redshift_staging as (
  select
    claim_id,
    blinded_creds,
    signed_creds,
    json_array_length(signed_creds),
    batch_proof,
    public_key,
    issuer_id,
    COALESCE(claim_creds.created_at, '2021-12-01'::date::timestamp without time zone) AS created_at,
    COALESCE(claim_creds.updated_at, '2021-12-01'::date::timestamp without time zone) AS updated_at
  from claim_creds
);
