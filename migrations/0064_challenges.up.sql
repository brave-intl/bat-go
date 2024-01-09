CREATE TABLE challenge (
    payment_id uuid PRIMARY KEY,
    created_at timestamp WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    nonce text NOT NULL,
    CONSTRAINT challenge_nonce UNIQUE (nonce)
);
