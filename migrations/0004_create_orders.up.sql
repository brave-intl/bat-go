create table orders (
  id uuid primary key not null default uuid_generate_v4(),
	created_at timestamp with time zone not null default current_timestamp,
	updated_at timestamp with time zone not null default current_timestamp,
	total_price text,
	merchant_id text,
	status text
);

create table order_items (
  id uuid primary key not null default uuid_generate_v4(),
  order_id uuid,
	created_at timestamp with time zone not null default current_timestamp,
	updated_at timestamp with time zone not null default current_timestamp,
	currency   text,
  quantity integer,
	price   text,
	subtotal text
);

create index order_items_indx on order_items(order_id);
