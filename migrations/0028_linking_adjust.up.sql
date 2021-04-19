create table linking_limit_adjust (
    provider_linking_id uuid,
    created_at timestamp with time zone not null default current_timestamp
);
