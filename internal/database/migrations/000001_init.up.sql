CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE users (
    id         UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    email      TEXT        UNIQUE NOT NULL,
    name       TEXT        NOT NULL DEFAULT '',
    avatar_url TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE email_codes (
    id         UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    email      TEXT        NOT NULL,
    code       TEXT        NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used       BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_email_codes_email ON email_codes(email);

CREATE TABLE user_locations (
    user_id    UUID             PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    lat        DOUBLE PRECISION NOT NULL,
    lon        DOUBLE PRECISION NOT NULL,
    updated_at TIMESTAMPTZ      NOT NULL DEFAULT NOW()
);

CREATE TABLE conversations (
    id         UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    user1_id   UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user2_id   UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user1_id, user2_id)
);

CREATE INDEX idx_conversations_user1 ON conversations(user1_id);
CREATE INDEX idx_conversations_user2 ON conversations(user2_id);

CREATE TABLE messages (
    id              UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    conversation_id UUID        NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    sender_id       UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content         TEXT        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_messages_conversation ON messages(conversation_id, created_at DESC);
