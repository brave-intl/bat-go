--- transactions is how transaction details are stored
create table transactions (
    --- qldb batch identifier for this payments run
    batch_id uuid not null,
    --- tx_id is custodian tx reference id.
    tx_id uuid default null,
    --- destination/origin are directional identifiers
    --- for custodian.  origin -> destination
    destination text not null,
    origin text not null,
    --- currency for the transaction
    currency text default 'BAT',
    --- amount is the total value
    approximate_value numeric(28, 18) not null check (approximate_value > 0.0),
    --- metadata dates around the transaction
    created_at timestamp with time zone not null default current_timestamp,
    updated_at timestamp with time zone not null default current_timestamp,
    --- status is the status of the transaction with custodian
    status text default null,
    --- signed tx ciphertext
    signed_tx_ciphertext bytea default null
);
