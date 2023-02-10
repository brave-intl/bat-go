create table verified_wallet_outbox (
    id uuid primary key not null default uuid_generate_v4(),
    created_at timestamp with time zone not null default current_timestamp,
    payment_id uuid not null,
    verified_wallet bool not null
);
