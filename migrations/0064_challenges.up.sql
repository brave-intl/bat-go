create table challenge (
    id text PRIMARY KEY,
    created_at timestamp WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    nonce text NOT NULL,
    constraint challenge_nonce unique (nonce)
);

create index if not exists challenge_nonce_idx on challenge(nonce);
