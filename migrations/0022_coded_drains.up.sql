ALTER TABLE claim_drain ADD COLUMN errcode text default null;
ALTER TABLE suggestion_drain ADD COLUMN errcode text default null;
ALTER TABLE vote_drain ADD COLUMN errcode text default null;
