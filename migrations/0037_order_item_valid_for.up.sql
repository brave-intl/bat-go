alter table order_items add column valid_for_iso text default null;

update order_items set valid_for_iso = 'P1M' where valid_for = '2629743001200000';
