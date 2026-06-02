ALTER TABLE users ADD COLUMN username TEXT;

CREATE UNIQUE INDEX idx_users_username ON users (lower(username)) WHERE username IS NOT NULL;

CREATE TABLE contacts (
    id              UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    owner_id        UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    contact_user_id UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    alias           TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (owner_id, contact_user_id)
);

CREATE INDEX idx_contacts_owner ON contacts(owner_id);
