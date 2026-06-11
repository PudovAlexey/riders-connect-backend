-- Per-member read cursor: "this member has read every message in this chat up to
-- last_read_at (compared against messages.created_at)". NULL = nothing read yet.
ALTER TABLE chat_members ADD COLUMN last_read_at TIMESTAMPTZ;
