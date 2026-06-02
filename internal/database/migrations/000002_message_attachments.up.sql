ALTER TABLE messages
    ADD COLUMN message_type    TEXT NOT NULL DEFAULT 'text',
    ADD COLUMN attachment_url  TEXT NOT NULL DEFAULT '',
    ADD COLUMN attachment_meta JSONB;
