package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func directKey(u1, u2 uuid.UUID) string {
	a, b := u1.String(), u2.String()
	if a > b {
		a, b = b, a
	}
	return a + ":" + b
}

func (r *Repository) GetOrCreateDirectChat(ctx context.Context, u1, u2 uuid.UUID) (*Chat, error) {
	key := directKey(u1, u2)
	c := &Chat{}
	var createdBy uuid.NullUUID
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO chats (type, direct_key) VALUES ('direct', $1)
		ON CONFLICT (direct_key) WHERE direct_key IS NOT NULL
		DO UPDATE SET direct_key = EXCLUDED.direct_key
		RETURNING id, type, title, avatar_url, created_by, created_at
	`, key).Scan(&c.ID, &c.Type, &c.Title, &c.AvatarURL, &createdBy, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	c.CreatedBy = createdBy.UUID

	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO chat_members (chat_id, user_id, role) VALUES ($1, $2, 'member'), ($1, $3, 'member')
		ON CONFLICT DO NOTHING
	`, c.ID, u1, u2); err != nil {
		return nil, err
	}
	return c, nil
}

func (r *Repository) CreateGroupChat(ctx context.Context, creatorID uuid.UUID, title, avatarURL string, memberIDs []uuid.UUID) (*Chat, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	c := &Chat{}
	var createdBy uuid.NullUUID
	err = tx.QueryRowContext(ctx, `
		INSERT INTO chats (type, title, avatar_url, created_by) VALUES ('group', $1, $2, $3)
		RETURNING id, type, title, avatar_url, created_by, created_at
	`, title, avatarURL, creatorID).Scan(&c.ID, &c.Type, &c.Title, &c.AvatarURL, &createdBy, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	c.CreatedBy = createdBy.UUID

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO chat_members (chat_id, user_id, role) VALUES ($1, $2, 'owner')
		ON CONFLICT DO NOTHING
	`, c.ID, creatorID); err != nil {
		return nil, err
	}
	for _, m := range memberIDs {
		if m == creatorID {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO chat_members (chat_id, user_id, role) VALUES ($1, $2, 'member')
			ON CONFLICT DO NOTHING
		`, c.ID, m); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return c, nil
}

func (r *Repository) GetChat(ctx context.Context, id uuid.UUID) (*Chat, error) {
	c := &Chat{}
	var createdBy uuid.NullUUID
	err := r.db.QueryRowContext(ctx, `
		SELECT id, type, title, avatar_url, created_by, created_at FROM chats WHERE id = $1
	`, id).Scan(&c.ID, &c.Type, &c.Title, &c.AvatarURL, &createdBy, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.CreatedBy = createdBy.UUID
	return c, nil
}

func (r *Repository) IsMember(ctx context.Context, chatID, userID uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM chat_members WHERE chat_id = $1 AND user_id = $2)
	`, chatID, userID).Scan(&exists)
	return exists, err
}

func (r *Repository) ListMemberIDs(ctx context.Context, chatID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT user_id FROM chat_members WHERE chat_id = $1
	`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *Repository) ListMembers(ctx context.Context, chatID uuid.UUID) ([]ChatMember, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT cm.user_id, u.name, COALESCE(u.username, ''), u.avatar_url, cm.role, cm.last_read_at
		FROM chat_members cm
		JOIN users u ON u.id = cm.user_id
		WHERE cm.chat_id = $1
		ORDER BY cm.joined_at
	`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []ChatMember
	for rows.Next() {
		var m ChatMember
		var lastRead sql.NullTime
		if err := rows.Scan(&m.UserID, &m.Name, &m.Username, &m.AvatarURL, &m.Role, &lastRead); err != nil {
			return nil, err
		}
		if lastRead.Valid {
			m.LastReadAt = &lastRead.Time
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

func (r *Repository) AddMember(ctx context.Context, chatID, userID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO chat_members (chat_id, user_id, role) VALUES ($1, $2, 'member')
		ON CONFLICT DO NOTHING
	`, chatID, userID)
	return err
}

func (r *Repository) RemoveMember(ctx context.Context, chatID, userID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM chat_members WHERE chat_id = $1 AND user_id = $2
	`, chatID, userID)
	return err
}

// UpdateChat patches a chat's title and/or avatar. nil fields are left as-is
// (COALESCE), so callers update only what they pass.
func (r *Repository) UpdateChat(ctx context.Context, chatID uuid.UUID, title, avatarURL *string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE chats SET
			title = COALESCE($2, title),
			avatar_url = COALESCE($3, avatar_url)
		WHERE id = $1
	`, chatID, title, avatarURL)
	return err
}

// ListChats returns the chats a user belongs to, ordered by most recent
// activity, enriched with members and the last message.
func (r *Repository) ListChats(ctx context.Context, userID uuid.UUID) ([]*ChatListItem, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.id, c.type, c.title, c.avatar_url, c.created_by, c.created_at,
		       lm.id, lm.sender_id, lm.content, lm.message_type, lm.attachment_url, lm.attachment_meta, lm.created_at,
		       (
		           SELECT COUNT(*) FROM messages um
		           WHERE um.chat_id = c.id
		             AND um.deleted_at IS NULL
		             AND um.sender_id <> $1
		             AND (me.last_read_at IS NULL OR um.created_at > me.last_read_at)
		             AND NOT EXISTS (
		                 SELECT 1 FROM message_hidden h
		                 WHERE h.message_id = um.id AND h.user_id = $1
		             )
		       ) AS unread_count
		FROM chats c
		JOIN chat_members me ON me.chat_id = c.id AND me.user_id = $1
		LEFT JOIN LATERAL (
			SELECT id, sender_id, content, message_type, attachment_url, attachment_meta, created_at
			FROM messages WHERE chat_id = c.id AND deleted_at IS NULL
			ORDER BY created_at DESC LIMIT 1
		) lm ON TRUE
		ORDER BY COALESCE(lm.created_at, c.created_at) DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*ChatListItem
	for rows.Next() {
		item := &ChatListItem{}
		var createdBy uuid.NullUUID
		var (
			mID      uuid.NullUUID
			mSender  uuid.NullUUID
			mContent sql.NullString
			mType    sql.NullString
			mAttURL  sql.NullString
			mMeta    []byte
			mCreated sql.NullTime
			unread   int
		)
		if err := rows.Scan(
			&item.ID, &item.Type, &item.Title, &item.AvatarURL, &createdBy, &item.CreatedAt,
			&mID, &mSender, &mContent, &mType, &mAttURL, &mMeta, &mCreated,
			&unread,
		); err != nil {
			return nil, err
		}
		item.CreatedBy = createdBy.UUID
		item.UnreadCount = unread
		if mID.Valid {
			msg := &Message{
				ID:            mID.UUID,
				ChatID:        item.ID,
				SenderID:      mSender.UUID,
				Content:       mContent.String,
				MessageType:   mType.String,
				AttachmentURL: mAttURL.String,
				CreatedAt:     mCreated.Time,
			}
			if len(mMeta) > 0 {
				msg.AttachmentMeta = json.RawMessage(mMeta)
			}
			item.LastMessage = msg
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Attach members for each chat.
	for _, item := range items {
		members, err := r.ListMembers(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		item.Members = members
	}
	return items, nil
}

func (r *Repository) CreateMessage(ctx context.Context, chatID, senderID uuid.UUID, p SendMessageParams) (*Message, error) {
	var metaArg interface{}
	if len(p.AttachmentMeta) > 0 {
		metaArg = []byte(p.AttachmentMeta)
	}

	m := &Message{}
	var rawMeta []byte
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO messages (chat_id, sender_id, content, message_type, attachment_url, attachment_meta)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, chat_id, sender_id, content, message_type, attachment_url, attachment_meta, created_at
	`, chatID, senderID, p.Content, p.MessageType, p.AttachmentURL, metaArg).
		Scan(&m.ID, &m.ChatID, &m.SenderID, &m.Content, &m.MessageType, &m.AttachmentURL, &rawMeta, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	if len(rawMeta) > 0 {
		m.AttachmentMeta = json.RawMessage(rawMeta)
	}
	return m, nil
}

func (r *Repository) ListMessages(ctx context.Context, chatID, userID uuid.UUID, limit, offset int) ([]*Message, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, chat_id, sender_id, content, message_type, attachment_url, attachment_meta, created_at, edited_at
		FROM messages
		WHERE chat_id = $1
		  AND deleted_at IS NULL
		  AND NOT EXISTS (
		      SELECT 1 FROM message_hidden h
		      WHERE h.message_id = messages.id AND h.user_id = $4
		  )
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, chatID, limit, offset, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []*Message
	for rows.Next() {
		m := &Message{}
		var rawMeta []byte
		var editedAt sql.NullTime
		if err := rows.Scan(&m.ID, &m.ChatID, &m.SenderID, &m.Content, &m.MessageType, &m.AttachmentURL, &rawMeta, &m.CreatedAt, &editedAt); err != nil {
			return nil, err
		}
		if len(rawMeta) > 0 {
			m.AttachmentMeta = json.RawMessage(rawMeta)
		}
		if editedAt.Valid {
			m.EditedAt = &editedAt.Time
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// GetMessage fetches a single message by ID, returning nil if it does not exist.
func (r *Repository) GetMessage(ctx context.Context, messageID uuid.UUID) (*Message, error) {
	m := &Message{}
	var rawMeta []byte
	var editedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `
		SELECT id, chat_id, sender_id, content, message_type, attachment_url, attachment_meta, created_at, edited_at
		FROM messages WHERE id = $1
	`, messageID).Scan(&m.ID, &m.ChatID, &m.SenderID, &m.Content, &m.MessageType, &m.AttachmentURL, &rawMeta, &m.CreatedAt, &editedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(rawMeta) > 0 {
		m.AttachmentMeta = json.RawMessage(rawMeta)
	}
	if editedAt.Valid {
		m.EditedAt = &editedAt.Time
	}
	return m, nil
}

// UpdateMessageContent rewrites a message's content and stamps edited_at.
// It refuses messages already deleted for everyone (deleted_at set).
func (r *Repository) UpdateMessageContent(ctx context.Context, messageID uuid.UUID, content string) (*Message, error) {
	m := &Message{}
	var rawMeta []byte
	var editedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `
		UPDATE messages SET content = $2, edited_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, chat_id, sender_id, content, message_type, attachment_url, attachment_meta, created_at, edited_at
	`, messageID, content).Scan(&m.ID, &m.ChatID, &m.SenderID, &m.Content, &m.MessageType, &m.AttachmentURL, &rawMeta, &m.CreatedAt, &editedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(rawMeta) > 0 {
		m.AttachmentMeta = json.RawMessage(rawMeta)
	}
	if editedAt.Valid {
		m.EditedAt = &editedAt.Time
	}
	return m, nil
}

// MarkMessageDeleted hides a message for everyone (delete "for all").
func (r *Repository) MarkMessageDeleted(ctx context.Context, messageID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE messages SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL
	`, messageID)
	return err
}

// HideMessageForUser hides a message only for the given user (delete "for me").
func (r *Repository) HideMessageForUser(ctx context.Context, messageID, userID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO message_hidden (message_id, user_id) VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, messageID, userID)
	return err
}

// MarkRead advances the member's read cursor to the chat's newest message. The
// cursor only ever moves forward (GREATEST). Returns the new cursor, or nil when
// the chat has no messages (nothing to mark) or the user is not a member.
func (r *Repository) MarkRead(ctx context.Context, chatID, userID uuid.UUID) (*time.Time, error) {
	var t sql.NullTime
	err := r.db.QueryRowContext(ctx, `
		UPDATE chat_members cm
		SET last_read_at = GREATEST(COALESCE(cm.last_read_at, 'epoch'::timestamptz), sub.t)
		FROM (SELECT MAX(created_at) AS t FROM messages
		      WHERE chat_id = $1 AND deleted_at IS NULL) sub
		WHERE cm.chat_id = $1 AND cm.user_id = $2 AND sub.t IS NOT NULL
		RETURNING cm.last_read_at
	`, chatID, userID).Scan(&t)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !t.Valid {
		return nil, nil
	}
	return &t.Time, nil
}
