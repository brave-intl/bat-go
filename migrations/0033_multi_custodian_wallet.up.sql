--- wallet_custodian - provides the ability to support multiple custodians per anonymous wallet
--- this log table is immutable, and referenced from the wallets table for linking information.
create table wallet_custodian (
    id uuid primary key not null default uuid_generate_v4(),
    wallet_id uuid not null,
    custodian text not null,
    created_at timestamp with time zone not null default current_timestamp,
    linked_at timestamp with time zone not null default current_timestamp,
    disconnected_at timestamp with time zone not null default current_timestamp,
    deposit_destination text not null,
    linking_id uuid not null
);

--- wallet_custodian_id is the link to the currently active custodial linking from the immutable 
--- table of linkings
alter table wallets add column wallet_custodian_id uuid default null;

--- create an index on the linking_id (which is how we check linking limits)
create index wallet_custodian_linking_id_idx on wallet_custodian(linking_id);
--- create an index on the wallet_id
create index wallet_custodian_wallet_id_idx on wallet_custodian(wallet_id);
--- check that the custodian text is in our supported custodians
alter table wallet_custodian add constraint check_custodian check (
    custodian IN (
        'brave', 'uphold', 'bitflyer', 'gemini'));
