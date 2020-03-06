alter table claims add column drained bool not null default false;

alter table wallets add column payout_address text default null;

create table claim_drain (
  id uuid primary key not null default uuid_generate_v4(),
  credentials json not null,
  wallet_id uuid not null,
  total numeric(28, 18) not null check (total > 0.0),
  transaction_id text default null,
  erred boolean not null default false
);
