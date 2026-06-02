-- Best-effort reverse: recreate the 1-on-1 conversations model (group chats are dropped).
CREATE TABLE conversations (
    id         UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    user1_id   UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user2_id   UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user1_id, user2_id)
);

CREATE INDEX idx_conversations_user1 ON conversations(user1_id);
CREATE INDEX idx_conversations_user2 ON conversations(user2_id);

INSERT INTO conversations (id, user1_id, user2_id, created_at)
SELECT c.id,
       MIN(cm.user_id) AS user1_id,
       MAX(cm.user_id) AS user2_id,
       c.created_at
FROM chats c
JOIN chat_members cm ON cm.chat_id = c.id
WHERE c.type = 'direct'
GROUP BY c.id, c.created_at
HAVING COUNT(*) = 2;

ALTER TABLE messages ADD COLUMN conversation_id UUID REFERENCES conversations(id) ON DELETE CASCADE;
UPDATE messages SET conversation_id = chat_id WHERE chat_id IN (SELECT id FROM conversations);
DELETE FROM messages WHERE conversation_id IS NULL;
ALTER TABLE messages ALTER COLUMN conversation_id SET NOT NULL;

DROP INDEX IF EXISTS idx_messages_chat;
ALTER TABLE messages DROP COLUMN chat_id;
CREATE INDEX idx_messages_conversation ON messages(conversation_id, created_at DESC);

DROP TABLE chat_members;
DROP TABLE chats;
