--- swap promotions fields for promotions v2
alter table promotions drop column auto_claim;
alter table promotions drop column skip_captcha;
alter table promotions drop column available;
alter table promotions drop column num_suggestions;
