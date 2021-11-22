create view normalized_credentials as (
  select
    id,
    substring(json_array_elements(credentials)->>'issuer', 0, 37) as promotion_id,
    substring(json_array_elements(credentials)->>'issuer', 38)    as cohort,
    json_array_elements(credentials)->>'t'                        as t,
    json_array_elements(credentials)->>'signature'                as signature,
    wallet_id,
    total,
    transaction_id,
    erred,
    errcode,
    completed,
    completed_at,
    batch_id,
    updated_at,
    claim_id,
    status,
    deposit_destination
  from claim_drain
);
