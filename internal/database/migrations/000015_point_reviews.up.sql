CREATE TABLE point_reviews (
    id         UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    point_id   UUID        NOT NULL REFERENCES points(id) ON DELETE CASCADE,
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rating     SMALLINT    NOT NULL CHECK (rating BETWEEN 1 AND 5),
    text       TEXT        NOT NULL DEFAULT '',
    photos     JSONB       NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (point_id, user_id)            -- one review per user per point
);

CREATE INDEX idx_point_reviews_point ON point_reviews(point_id);
