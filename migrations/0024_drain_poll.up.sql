--- completed indicates this claim_drain job is drained and complete
alter table claim_drain add column completed boolean not null default false;
--- completed_at indicates the time at which this claim_drain job was completed
alter table claim_drain add column completed_at timestamp;
--- batch_id is the draining batch that this claim drain job belongs to
alter table claim_drain add column batch_id uuid default null;
--- create an index on the batch_id for easy lookup
create index batch_id_idx on claim_drain(batch_id);
