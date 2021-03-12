create table bf_req_ids (
    id text primary key,
    created_at timestamp with time zone not null default current_timestamp
);
