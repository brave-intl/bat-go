drop index batch_id_idx;
--- completed indicates this claim_drain job is drained and complete
alter table claim_drain drop column completed;
--- completed_at indicates the time at which this claim_drain job was completed
alter table claim_drain drop column completed_at;
--- batch_id is the draining batch that this claim drain job belongs to
alter table claim_drain drop column batch_id;
