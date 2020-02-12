create table clobbered_claims(
  id uuid not null,
  created_at timestamp with time zone not null default current_timestamp
);
