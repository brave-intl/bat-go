create table vote_drain (
  id uuid primary key not null default uuid_generate_v4(),
  credentials json not null,
  vote_text text not null,
  vote_event bytea not null,
  erred boolean not null default false,
  processed boolean not null default false
);
