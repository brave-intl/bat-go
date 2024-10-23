ALTER TABLE order_items ADD COLUMN sku_variant text NOT NULL DEFAULT '';

ALTER TABLE order_items ALTER COLUMN sku_variant DROP DEFAULT;


-- Will be executed manually.
-- UPDATE order_items SET sku_variant=sku;
