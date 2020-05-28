create table claim_drain_overflow(
  id uuid primary key not null default uuid_generate_v4(),
  created_at timestamp with time zone not null default current_timestamp,
  credentials json not null,
  wallet_id uuid not null,
  total numeric(28, 18) not null check (total > 0.0),
  transaction_id text default null,
  erred boolean not null default false
);
