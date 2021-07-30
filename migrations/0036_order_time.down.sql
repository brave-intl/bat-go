alter table orders drop column valid_for;
alter table orders drop column expires_at;
alter table order_items drop column valid_for;

drop table order_payment_history;
