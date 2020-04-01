create table api_keys (
  id uuid primary key not null default uuid_generate_v4(),
  merchant_id text NOT NULL,
  encrypted_secret_key text NOT NULL,
  nonce text NOT NULL,
  created_at timestamp with time zone not null default current_timestamp,
  expiry TIMESTAMP with time zone
);

create index merchant_index on api_keys(merchant_id);
