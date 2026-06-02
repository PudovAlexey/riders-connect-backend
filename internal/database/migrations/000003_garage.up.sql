CREATE TABLE garage_vehicles (
    id         UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    make       TEXT        NOT NULL,
    model      TEXT        NOT NULL,
    year       INTEGER     NOT NULL,
    color      TEXT        NOT NULL DEFAULT '',
    notes      TEXT        NOT NULL DEFAULT '',
    photos     JSONB       NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX ON garage_vehicles(user_id);
