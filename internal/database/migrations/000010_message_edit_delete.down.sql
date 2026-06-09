DROP TABLE IF EXISTS message_hidden;
ALTER TABLE messages DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE messages DROP COLUMN IF EXISTS edited_at;
