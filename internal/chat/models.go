package chat

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	MessageTypeText      = "text"
	MessageTypeImage     = "image"
	MessageTypeVideo     = "video"
	MessageTypeVideoNote = "video_note"
	MessageTypeAudio     = "audio"
	MessageTypeFile      = "file"
)

var validMessageTypes = map[string]bool{
	MessageTypeText:      true,
	MessageTypeImage:     true,
	MessageTypeVideo:     true,
	MessageTypeVideoNote: true,
	MessageTypeAudio:     true,
	MessageTypeFile:      true,
}

// DeleteScope controls how far a message deletion reaches.
const (
	DeleteScopeMe  = "me"  // hide only for the requesting user
	DeleteScopeAll = "all" // hide for everyone (sender only)
)

const (
	ChatTypeDirect = "direct"
	ChatTypeGroup  = "group"
)

const (
	RoleOwner  = "owner"
	RoleMember = "member"
)

type Chat struct {
	ID        uuid.UUID `json:"id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	AvatarURL string    `json:"avatar_url"`
	CreatedBy uuid.UUID `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

type ChatMember struct {
	UserID    uuid.UUID `json:"user_id"`
	Name      string    `json:"name"`
	Username  string    `json:"username"`
	AvatarURL string    `json:"avatar_url"`
	Role      string    `json:"role"`
}

// ChatListItem is the enriched chat shape returned to clients: the chat plus
// its members and the most recent message.
type ChatListItem struct {
	Chat
	Members     []ChatMember `json:"members"`
	LastMessage *Message     `json:"last_message,omitempty"`
}

type Message struct {
	ID             uuid.UUID       `json:"id"`
	ChatID         uuid.UUID       `json:"chat_id"`
	SenderID       uuid.UUID       `json:"sender_id"`
	Content        string          `json:"content"`
	MessageType    string          `json:"message_type"`
	AttachmentURL  string          `json:"attachment_url,omitempty"`
	AttachmentMeta json.RawMessage `json:"attachment_meta,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	EditedAt       *time.Time      `json:"edited_at,omitempty"`
}

type SendMessageParams struct {
	Content        string
	MessageType    string
	AttachmentURL  string
	AttachmentMeta json.RawMessage
}
