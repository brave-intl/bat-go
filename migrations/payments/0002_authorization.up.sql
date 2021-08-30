--- authorizations - records of batch payout authorizations.
create table authorizations (
    --- batch_id is the id of the batch of transactions being authorized
    batch_id uuid not null,
    --- metadata dates around the authorization
    created_at timestamp with time zone not null default current_timestamp,
    updated_at timestamp with time zone not null default current_timestamp,
    --- the public key used for the authorization
    public_key text not null
    --- the signature of the authorization from the caller
    signature text not null
)
