CREATE TABLE rooms (
    name TEXT NOT NULL PRIMARY KEY,
    tier TEXT NOT NULL DEFAULT 'free' CONSTRAINT tier_options CHECK (tier IN ('paid', 'free')),
    head_count INTEGER NOT NULL DEFAULT 1, 
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    terminated_at TIMESTAMP,
    CONSTRAINT free_head_count_limit CHECK ((tier = 'free' AND head_count < 3) OR tier = 'paid'),
    CONSTRAINT terminated_at_after_created_at CHECK (terminated_at > created_at)
);

CREATE UNIQUE INDEX rooms_unique_name on rooms (LOWER(name));
