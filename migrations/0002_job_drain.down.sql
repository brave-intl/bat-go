drop table if exists suggestion_drain;

drop index claim_creds_batch_proof_idx ;
alter table claim_creds drop column issuer_id;

alter table issuers drop constraint promo_cohort_uniq;
alter table issuers drop constraint issuers_pkey;
alter table issuers drop column id;
alter table issuers add primary key (promotion_id, cohort);
