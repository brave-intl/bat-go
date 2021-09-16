alter table order_item add column valid_for_iso text default null;

update order_item set valid_for_iso = 'P1M' where valid_for = '2629743001200000';
