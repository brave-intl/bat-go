alter table issuers add column id uuid not null default uuid_generate_v4();
alter table issuers drop constraint issuers_pkey;
alter table issuers add primary key (id);
alter table issuers add constraint promo_cohort_uniq unique (promotion_id, cohort);

delete from claim_creds;
alter table claim_creds add column issuer_id uuid not null references issuers(id);
create index on claim_creds(batch_proof);

create table suggestion_drain (
  id uuid primary key not null default uuid_generate_v4(),
  credentials json not null,
  suggestion_text text not null,
  suggestion_event bytea not null
);
