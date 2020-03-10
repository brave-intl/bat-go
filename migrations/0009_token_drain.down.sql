alter table claims drop column drained;
alter table wallets drop column payout_address;
drop table if exists claim_drain;
