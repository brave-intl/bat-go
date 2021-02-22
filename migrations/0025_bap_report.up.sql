create table bap_report (
  id uuid primary key not null default uuid_generate_v4(),
  wallet_id uuid not null,
  amount numeric(28,18) not null default 0.0,
  created_at timestamp with time zone not null default current_timestamp,
  constraint no_bap_report_dups unique(wallet_id)
);
