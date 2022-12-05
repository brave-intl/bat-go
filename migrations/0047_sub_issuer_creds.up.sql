create table time_limited_v2_order_creds (
    id uuid primary key not null default uuid_generate_v4(),
    created_at timestamp with time zone not null default current_timestamp,
    item_id uuid not null references order_items(id),
    order_id uuid not null references orders(id),
    issuer_id uuid not null references order_cred_issuers(id),
    valid_to timestamp with time zone not null,
    valid_from timestamp with time zone not null ,
    batch_proof text not null,
    public_key text not null,
    blinded_creds json not null,
    signed_creds json not null,
    constraint time_limited_v2_order_creds_unique unique (order_id, item_id, valid_to, valid_from)
);

create index if not exists time_limited_v2_order_creds_order_id_idx on time_limited_v2_order_creds(order_id);
create index if not exists time_limited_v2_order_creds_item_id_idx on time_limited_v2_order_creds(item_id);

create table signing_order_request_outbox (
    id uuid primary key not null default uuid_generate_v4(),
    created_at timestamp with time zone not null default current_timestamp,
    submitted_at timestamp with time zone default null,
    completed_at timestamp with time zone default null,
    request_id uuid not null,
    order_id uuid not null,
    item_id uuid not null,
    message_data json not null,
    constraint signing_order_request_outbox_request_id_unique unique (request_id)
);

create index if not exists signing_order_request_outbox_order_id_request_id_idx on signing_order_request_outbox(request_id);
create index if not exists signing_order_request_outbox_order_id_idx on signing_order_request_outbox(order_id);
create index if not exists signing_order_request_outbox_order_id_item_id_idx on signing_order_request_outbox(order_id, item_id);
