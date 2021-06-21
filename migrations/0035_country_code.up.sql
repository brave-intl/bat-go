--- add country code for this linked wallet/custodian
alter table wallet_custodian add column country_code text default null;
