--- swap_claims will hold all claims that are for swaps
create table swap_claims (
  id uuid primary key not null default uuid_generate_v4(),
  created_at timestamp with time zone not null default current_timestamp,
  updated_at timestamp with time zone not null default current_timestamp,
  drained_at timestamp with time zone,
  redeemed_at timestamp with time zone,
  promotion_id uuid not null references promotions(id),
  address_id text not null,
  approximate_value numeric(28, 18) not null check (approximate_value > 0.0),
  bonus numeric(28, 18) not null check (bonus >= 0.0) default 0,
  redeemed boolean not null default false,
  unique (promotion_id, address_id)
);

--- swap_claims index on address_id
create index on swap_claims(address_id) ;
