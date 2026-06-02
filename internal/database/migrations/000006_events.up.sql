CREATE TABLE events (
    id          UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    owner_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type        TEXT        NOT NULL DEFAULT 'meeting',
    title       TEXT        NOT NULL DEFAULT '',
    description TEXT        NOT NULL DEFAULT '',
    starts_at   TIMESTAMPTZ NOT NULL,
    visibility  TEXT        NOT NULL DEFAULT 'private',   -- public | private
    details     JSONB       NOT NULL DEFAULT '{}',        -- per-type extra fields
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_events_owner ON events(owner_id);
CREATE INDEX idx_events_starts_at ON events(starts_at);
CREATE INDEX idx_events_visibility ON events(visibility);

CREATE TABLE event_participants (
    event_id   UUID        NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT        NOT NULL DEFAULT 'participant', -- owner | participant
    status     TEXT        NOT NULL DEFAULT 'pending',     -- pending | accepted | declined
    invited_by UUID        REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (event_id, user_id)
);

CREATE INDEX idx_event_participants_user ON event_participants(user_id);
