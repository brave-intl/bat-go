--- drop wallet_custodian indexes
drop index wallet_custodian_linking_id_idx;
drop index wallet_custodian_wallet_id_idx;
--- drop the wallets column for wallet_custodian_id
alter table wallets drop column wallet_custodian_id;
--- drop wallet_custodian table
drop table wallet_custodian;
