--- swap promotions fields for promotions v2
alter table promotions drop column auto_claim;
alter table promotions drop column skip_captcha;
alter table promotions drop column available;
alter table promotions drop column num_suggestions;

alter table promotions drop constraint check_promotion_type;
alter table promotions add constraint check_promotion_type check (promotion_type in ('ugp', 'ads'));
