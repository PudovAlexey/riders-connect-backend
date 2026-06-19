CREATE TABLE routes (
    id           UUID             PRIMARY KEY DEFAULT uuid_generate_v4(),
    owner_id     UUID             NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT             NOT NULL DEFAULT '',
    description  TEXT             NOT NULL DEFAULT '',
    points       JSONB            NOT NULL DEFAULT '[]',          -- ordered waypoints, up to 10
    distance_m   DOUBLE PRECISION NOT NULL DEFAULT 0,            -- denormalized, computed client-side via ORS
    duration_s   DOUBLE PRECISION NOT NULL DEFAULT 0,
    is_suggested BOOLEAN          NOT NULL DEFAULT FALSE,         -- curated recommendations (server-only flag)
    created_at   TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ      NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_routes_owner ON routes(owner_id);
CREATE INDEX idx_routes_suggested ON routes(is_suggested) WHERE is_suggested = TRUE;
