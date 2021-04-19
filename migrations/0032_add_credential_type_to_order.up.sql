alter table order_items
add credential_type text not null default 'single-use';
