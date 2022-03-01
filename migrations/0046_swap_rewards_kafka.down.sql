--- swap promotions grant reward fields for kafka worker
alter table claims drop column transaction_key;
delete from wallets where id = '00000000-0000-0000-0000-000000000002';
