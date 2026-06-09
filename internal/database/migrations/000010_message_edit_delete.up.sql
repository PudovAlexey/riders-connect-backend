ALTER TABLE messages ADD COLUMN edited_at  TIMESTAMPTZ;
ALTER TABLE messages ADD COLUMN deleted_at TIMESTAMPTZ;  -- set when deleted "for everyone"

-- Per-user "delete for me": hides a message only for the listed user.
CREATE TABLE message_hidden (
    message_id UUID        NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id    UUID        NOT NULL REFERENCES users(id)    ON DELETE CASCADE,
    hidden_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (message_id, user_id)
);
