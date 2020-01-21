create table order_cred_issuers (
  id uuid primary key not null default uuid_generate_v4(),
  created_at timestamp with time zone not null default current_timestamp,
	merchant_id text NOT NULL,
  public_key text not null
);

create table order_creds (
  item_id uuid primary key not null references order_items(id),
  order_id uuid not null references orders(id),
  issuer_id uuid not null references order_cred_issuers(id),
  blinded_creds json not null,
  signed_creds json,
  batch_proof text,
  public_key text
);
