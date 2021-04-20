alter table suggestion_drain
add retry_from integer not null default 0;
