create table transactions (
	id uuid primary key not null default uuid_generate_v4(),
	order_id uuid references orders(id),
	created_at timestamp with time zone not null default current_timestamp,
	updated_at timestamp with time zone not null default current_timestamp,
  external_transaction_id text not null,
	status   text NOT NULL,
	currency   text NOT NULL,
	kind   text NOT NULL,
	amount   numeric(28, 18) NOT NULL,
);
