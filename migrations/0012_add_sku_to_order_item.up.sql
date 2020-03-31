alter table order_items
add sku text not null default '';

alter table order_items
alter column sku drop default;

