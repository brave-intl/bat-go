create table mint_drain (
  id uuid primary key not null default uuid_generate_v4(),
  wallet_id uuid not null,
  erred boolean not null default false,
  status varchar(10) not null default 'pending'
);

create table mint_drain_promotion (
  promotion_id uuid not null,
  mint_drain_id uuid not null,
  total numeric(28, 18) not null default 0.0,
  done boolean default false,
  primary key(promotion_id, mint_drain_id),
  constraint no_dups unique (promotion_id, mint_drain_id)
);
