-- Unified chat model: direct (2 members) and group (N members).
CREATE TABLE chats (
    id         UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    type       TEXT        NOT NULL DEFAULT 'direct',
    title      TEXT        NOT NULL DEFAULT '',
    avatar_url TEXT        NOT NULL DEFAULT '',
    direct_key TEXT,
    created_by UUID        REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- One direct chat per unordered user pair; NULL for groups.
CREATE UNIQUE INDEX idx_chats_direct_key ON chats(direct_key) WHERE direct_key IS NOT NULL;

CREATE TABLE chat_members (
    chat_id   UUID        NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    user_id   UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role      TEXT        NOT NULL DEFAULT 'member',
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (chat_id, user_id)
);

CREATE INDEX idx_chat_members_user ON chat_members(user_id);

-- Backfill existing 1-on-1 conversations into the unified model, reusing the
-- conversation id as the chat id so messages.chat_id can be copied directly.
INSERT INTO chats (id, type, direct_key, created_at)
SELECT id, 'direct',
       CASE WHEN user1_id < user2_id
            THEN user1_id || ':' || user2_id
            ELSE user2_id || ':' || user1_id END,
       created_at
FROM conversations;

INSERT INTO chat_members (chat_id, user_id, role)
SELECT id, user1_id, 'member' FROM conversations
UNION ALL
SELECT id, user2_id, 'member' FROM conversations;

-- Repoint messages from conversation_id to chat_id.
ALTER TABLE messages ADD COLUMN chat_id UUID REFERENCES chats(id) ON DELETE CASCADE;
UPDATE messages SET chat_id = conversation_id;
ALTER TABLE messages ALTER COLUMN chat_id SET NOT NULL;

DROP INDEX IF EXISTS idx_messages_conversation;
ALTER TABLE messages DROP COLUMN conversation_id;
DROP TABLE conversations;

CREATE INDEX idx_messages_chat ON messages(chat_id, created_at DESC);
