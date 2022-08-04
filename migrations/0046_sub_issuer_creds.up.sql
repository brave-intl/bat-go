-- TODO add constraints and index
create table time_limited_v2_order_creds (
    id uuid primary key not null default uuid_generate_v4(),
    created_at timestamp with time zone not null default current_timestamp,
    item_id uuid not null references order_items(id),
    order_id uuid not null references orders(id),
    issuer_id uuid not null references order_cred_issuers(id),
    valid_from timestamp with time zone not null ,
    valid_to timestamp with time zone not null,
    batch_proof text not null,
    public_key text not null,
    blinded_creds json not null,
    signed_creds json not null
);

create table signing_request_submitted (
    id uuid primary key not null default uuid_generate_v4(),
    created_at timestamp with time zone not null default current_timestamp,
    order_id uuid not null
);

create index if not exists signing_request_submitted_item_id_idx on signing_request_submitted(order_id);