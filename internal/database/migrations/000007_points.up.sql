CREATE TABLE points (
    id          UUID             PRIMARY KEY DEFAULT uuid_generate_v4(),
    created_by  UUID             REFERENCES users(id) ON DELETE SET NULL,
    category    TEXT             NOT NULL,
    name        TEXT             NOT NULL,
    description TEXT             NOT NULL DEFAULT '',
    lat         DOUBLE PRECISION NOT NULL,
    lon         DOUBLE PRECISION NOT NULL,
    address     TEXT             NOT NULL DEFAULT '',
    phone       TEXT             NOT NULL DEFAULT '',
    website     TEXT             NOT NULL DEFAULT '',
    hours       TEXT             NOT NULL DEFAULT '',   -- working hours, free text
    photos      JSONB            NOT NULL DEFAULT '[]',
    created_at  TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ      NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_points_category ON points(category);
CREATE INDEX idx_points_lat_lon ON points(lat, lon);
