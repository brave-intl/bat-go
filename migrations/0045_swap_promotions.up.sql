--- swap promotions fields for promotions v2
alter table promotions add column auto_claim boolean;
alter table promotions add column skip_captcha boolean;
alter table promotions add column available boolean not null default false;
alter table promotions add column num_suggestions integer;

alter table promotions drop constraint check_promotion_type;

alter table promotions add constraint check_promotion_type check (promotion_type in ('ugp', 'ads', 'swap'));
