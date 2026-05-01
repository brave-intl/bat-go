ALTER TABLE order_items
  DROP CONSTRAINT IF EXISTS order_items_max_active_batches_tlv2_creds_sanity,
  DROP COLUMN num_self_extensions,
  DROP COLUMN last_self_extension_at;
