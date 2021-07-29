alter table orders drop column valid_for;
alter table orders drop column expires_at;

drop table order_renewal;
