-- drop index to improve ETL speeds
drop index wallets_updated_at_idx;

--- drop wallet_custodian indexes
drop index wallet_custodian_linking_id_idx;
drop index wallet_custodian_wallet_id_idx;
--- drop wallet_custodian table
drop table wallet_custodian;
