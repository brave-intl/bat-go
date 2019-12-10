drop index wallet_claim_provider_linking_idx;
drop index wallet_claim_anonymous_address_idx;
alter table wallets drop column provider_linking_id;
alter table wallets drop column anonymous_address;
