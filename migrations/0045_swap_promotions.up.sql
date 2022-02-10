--- swap promotions fields for promotions v2
alter table promotions add column auto_claim boolean;
alter table promotions add column skip_captcha boolean;
alter table promotions add column available boolean;
alter table promotions add column num_suggestions integer;
