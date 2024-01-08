create table allow_list (
    payment_id uuid PRIMARY KEY,
    created_at timestamp WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);
