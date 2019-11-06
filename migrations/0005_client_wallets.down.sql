alter table wallets drop constraint check_provider;
alter table wallets add constraint check_provider check (provider in ('uphold'));
