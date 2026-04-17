ALTER TABLE order_items
  ADD COLUMN num_self_extensions    INT         NOT NULL DEFAULT 0,
  ADD COLUMN last_self_extension_at TIMESTAMPTZ;
