drop trigger update_updated_at_on_wallets on wallets;

drop function update_updated_at();

alter table wallets drop column updated_at;

alter table wallets drop column created_at;
