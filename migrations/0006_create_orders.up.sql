create table orders (
  id uuid primary key not null default uuid_generate_v4(),
  created_at timestamp with time zone not null default current_timestamp,
  updated_at timestamp with time zone not null default current_timestamp,
  total_price numeric(28, 18) not null,
  merchant_id text NOT NULL,
  status text NOT NULL CHECK (status IN ('pending', 'paid', 'fulfilled', 'canceled'))
);

create table order_items (
  id uuid primary key not null default uuid_generate_v4(),
  order_id uuid references orders(id),
  created_at timestamp with time zone not null default current_timestamp,
  updated_at timestamp with time zone not null default current_timestamp,
  currency text NOT NULL,
  quantity integer NOT NULL,
  price numeric(28, 18) NOT NULL,
  subtotal numeric(28, 18) NOT NULL
);

create index order_items_indx on order_items(order_id);
