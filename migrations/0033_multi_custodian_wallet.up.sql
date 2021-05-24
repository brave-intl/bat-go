--- wallet_custodian - provides the ability to support multiple custodians per anonymous wallet
create table wallet_custodian (
    wallet_id uuid not null,
    custodian text not null,
    linking_id uuid not null,
    created_at timestamp with time zone not null default current_timestamp,
    linked_at timestamp with time zone not null default current_timestamp,
    disconnected_at timestamp with time zone,
    primary key (wallet_id, linking_id, custodian)
);

--- only one custodian can be connected at a time
create unique index wallet_custodian_unique_connected
    on wallet_custodian (
        custodian, wallet_id, linking_id, coalesce(disconnected_at, '1970-01-01'));

--- create an index on the linking_id (which is how we check linking limits)
create index wallet_custodian_linking_id_idx on wallet_custodian(linking_id);
--- create an index on the wallet_id
create index wallet_custodian_wallet_id_idx on wallet_custodian(wallet_id);
--- check that the custodian text is in our supported custodians
alter table wallet_custodian add constraint check_custodian check (
    custodian IN (
        'brave', 'uphold', 'bitflyer', 'gemini'));
