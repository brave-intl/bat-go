drop table mint_drain;
drop table mint_drain_promotion;

-- Compose migration from payments service
ALTER TABLE orders DROP COLUMN metadata;