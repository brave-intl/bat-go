create extension if not exists "uuid-ossp";

create table promotions (
  id uuid primary key not null default uuid_generate_v4(),
  promotion_type text not null,
  created_at timestamp with time zone not null default current_timestamp,
  expires_at timestamp with time zone not null default current_timestamp + interval '4 months',
  version integer not null default 5,
  suggestions_per_grant integer not null,
  approximate_value numeric(28, 18) not null check (approximate_value > 0.0),
  remaining_grants integer not null check (remaining_grants >= 0),
  active boolean not null default false
);

alter table promotions add constraint check_promotion_type check (promotion_type in ('ugp', 'ads'));

create table issuers (
  promotion_id uuid not null references promotions(id),
  cohort text not null,
  public_key text not null,
  primary key (promotion_id, cohort)
);

create table wallets (
  id uuid primary key not null,
  -- created_at timestamp with time zone not null default current_timestamp,
  provider text not null default 'uphold',
  provider_id text not null,
  public_key text not null
);

alter table wallets add constraint check_provider check (provider in ('uphold'));

create table claims (
  id uuid primary key not null default uuid_generate_v4(),
  created_at timestamp with time zone not null default current_timestamp,
  promotion_id uuid not null references promotions(id),
  wallet_id uuid not null references wallets(id),
  approximate_value numeric(28, 18) not null check (approximate_value > 0.0),
  redeemed boolean not null default false,
  unique (promotion_id, wallet_id)
);

create table claim_creds (
  claim_id uuid primary key not null references claims(id),
  blinded_creds json not null,
  signed_creds json,
  batch_proof text,
  public_key text
);
