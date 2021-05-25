alter table order_items  drop column payment_methods;
alter table orders drop column metadata;
alter table order_items drop column payment_methods;
alter table order_items add column payment_methods text[];
