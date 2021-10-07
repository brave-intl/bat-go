alter table orders add column allowed_payment_methods text[];
alter table order_items add column metadata jsonb default null;
alter table orders add column metadata jsonb default null;
