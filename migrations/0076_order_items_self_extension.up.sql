ALTER TABLE order_items
  ADD COLUMN num_self_extensions    INT         NOT NULL DEFAULT 0 CHECK (num_self_extensions >= 0),
  ADD COLUMN last_self_extension_at TIMESTAMPTZ,
  ADD CONSTRAINT order_items_max_active_batches_tlv2_creds_sanity
    CHECK (max_active_batches_tlv2_creds IS NULL OR max_active_batches_tlv2_creds <= 1000);
