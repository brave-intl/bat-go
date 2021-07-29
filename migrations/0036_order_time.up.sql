alter table orders add column valid_for text default null;
alter table orders add column expires_at timestamp with time zone default null;

create table order_renewal (
    order_id uuid not null,
    last_paid timestamp with time zone not null,
    primary key (order_id, last_paid)
);
