package chat

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"riders-connect/internal/mailer"
	"riders-connect/internal/models"
)

var ErrNotFound = errors.New("chat not found")
var ErrForbidden = errors.New("forbidden")
var ErrInvalidMessageType = errors.New("invalid message type")
var ErrUserNotFound = errors.New("user not found")
var ErrInvalidGroup = errors.New("group requires a title and at least one other member")
var ErrMessageNotFound = errors.New("message not found")
var ErrInvalidScope = errors.New("invalid delete scope")
var ErrEmptyContent = errors.New("content required")

// emailOnEveryMessage controls how aggressively new-message emails are sent.
// true  → email every other member on every message (the product's current ask).
// false → would email only offline recipients (flip here if Yandex rate-limits).
const emailOnEveryMessage = true

// userResolver lets the chat service turn @logins into user IDs and look users
// up by ID (for notification email addresses). profile.Service satisfies it.
type userResolver interface {
	GetByUsername(ctx context.Context, username string) (*models.User, error)
	GetUser(ctx context.Context, id uuid.UUID) (*models.User, error)
}

// pushSender delivers Web Push to a user and reports how many devices got it.
// push.Service satisfies it; nil-safe via the s.push != nil guards below.
type pushSender interface {
	SendToUser(ctx context.Context, userID uuid.UUID, title, body, url, tag string) int
}

type Service struct {
	repo    *Repository
	users   userResolver
	mail    *mailer.Mailer
	push    pushSender
	baseURL string
}

func NewService(repo *Repository, users userResolver, mail *mailer.Mailer, push pushSender, baseURL string) *Service {
	return &Service{repo: repo, users: users, mail: mail, push: push, baseURL: baseURL}
}

func (s *Service) ResolveUsername(ctx context.Context, username string) (uuid.UUID, error) {
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	u, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		return uuid.Nil, err
	}
	if u == nil {
		return uuid.Nil, ErrUserNotFound
	}
	return u.ID, nil
}

func (s *Service) CreateDirect(ctx context.Context, requesterID, targetID uuid.UUID) (*Chat, error) {
	return s.repo.GetOrCreateDirectChat(ctx, requesterID, targetID)
}

func (s *Service) CreateGroup(ctx context.Context, creatorID uuid.UUID, title string, memberIDs []uuid.UUID) (*Chat, error) {
	title = strings.TrimSpace(title)
	others := make([]uuid.UUID, 0, len(memberIDs))
	for _, id := range memberIDs {
		if id != creatorID {
			others = append(others, id)
		}
	}
	if title == "" || len(others) < 1 {
		return nil, ErrInvalidGroup
	}
	return s.repo.CreateGroupChat(ctx, creatorID, title, "", others)
}

func (s *Service) ListChats(ctx context.Context, userID uuid.UUID) ([]*ChatListItem, error) {
	return s.repo.ListChats(ctx, userID)
}

func (s *Service) GetChat(ctx context.Context, id uuid.UUID) (*Chat, error) {
	return s.repo.GetChat(ctx, id)
}

// GetChatItem returns a chat enriched with its members for a member.
func (s *Service) GetChatItem(ctx context.Context, chatID, userID uuid.UUID) (*ChatListItem, error) {
	chat, err := s.repo.GetChat(ctx, chatID)
	if err != nil {
		return nil, err
	}
	if chat == nil {
		return nil, ErrNotFound
	}
	ok, err := s.repo.IsMember(ctx, chatID, userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrForbidden
	}
	members, err := s.repo.ListMembers(ctx, chatID)
	if err != nil {
		return nil, err
	}
	return &ChatListItem{Chat: *chat, Members: members}, nil
}

func (s *Service) ListMemberIDs(ctx context.Context, chatID uuid.UUID) ([]uuid.UUID, error) {
	return s.repo.ListMemberIDs(ctx, chatID)
}

func (s *Service) AddMember(ctx context.Context, chatID, requesterID, targetID uuid.UUID) error {
	ok, err := s.repo.IsMember(ctx, chatID, requesterID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	if err := s.repo.AddMember(ctx, chatID, targetID); err != nil {
		return err
	}
	go s.notifyAddedToGroup(requesterID, chatID, targetID)
	return nil
}

func (s *Service) GetMessages(ctx context.Context, chatID, userID uuid.UUID, limit, offset int) ([]*Message, error) {
	ok, err := s.repo.IsMember(ctx, chatID, userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		// Distinguish missing chat from non-membership.
		chat, err := s.repo.GetChat(ctx, chatID)
		if err != nil {
			return nil, err
		}
		if chat == nil {
			return nil, ErrNotFound
		}
		return nil, ErrForbidden
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.repo.ListMessages(ctx, chatID, userID, limit, offset)
}

func (s *Service) SendMessage(ctx context.Context, chatID, senderID uuid.UUID, p SendMessageParams) (*Message, error) {
	if p.MessageType == "" {
		p.MessageType = MessageTypeText
	}
	if !validMessageTypes[p.MessageType] {
		return nil, ErrInvalidMessageType
	}
	ok, err := s.repo.IsMember(ctx, chatID, senderID)
	if err != nil {
		return nil, err
	}
	if !ok {
		chat, err := s.repo.GetChat(ctx, chatID)
		if err != nil {
			return nil, err
		}
		if chat == nil {
			return nil, ErrNotFound
		}
		return nil, ErrForbidden
	}
	msg, err := s.repo.CreateMessage(ctx, chatID, senderID, p)
	if err != nil {
		return nil, err
	}
	if emailOnEveryMessage {
		go s.notifyNewMessage(senderID, chatID, msg)
	}
	return msg, nil
}

// loadOwnMessage fetches a message and verifies it belongs to chatID and was
// sent by userID. Used by edit and delete-for-all, both author-only operations.
func (s *Service) loadOwnMessage(ctx context.Context, chatID, messageID, userID uuid.UUID) (*Message, error) {
	msg, err := s.repo.GetMessage(ctx, messageID)
	if err != nil {
		return nil, err
	}
	if msg == nil || msg.ChatID != chatID {
		return nil, ErrMessageNotFound
	}
	if msg.SenderID != userID {
		return nil, ErrForbidden
	}
	return msg, nil
}

// requireMember confirms the user belongs to the chat, distinguishing a missing
// chat (ErrNotFound) from a non-member (ErrForbidden) like the other methods.
func (s *Service) requireMember(ctx context.Context, chatID, userID uuid.UUID) error {
	ok, err := s.repo.IsMember(ctx, chatID, userID)
	if err != nil {
		return err
	}
	if !ok {
		chat, err := s.repo.GetChat(ctx, chatID)
		if err != nil {
			return err
		}
		if chat == nil {
			return ErrNotFound
		}
		return ErrForbidden
	}
	return nil
}

// EditMessage rewrites a message's content. Author-only, no time limit.
func (s *Service) EditMessage(ctx context.Context, chatID, messageID, userID uuid.UUID, content string) (*Message, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, ErrEmptyContent
	}
	if err := s.requireMember(ctx, chatID, userID); err != nil {
		return nil, err
	}
	if _, err := s.loadOwnMessage(ctx, chatID, messageID, userID); err != nil {
		return nil, err
	}
	msg, err := s.repo.UpdateMessageContent(ctx, messageID, content)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		// Message was deleted for everyone between the load and the update.
		return nil, ErrMessageNotFound
	}
	return msg, nil
}

// DeleteMessage hides a message. scope=me hides it only for the requesting
// member; scope=all hides it for everyone and is restricted to the author.
func (s *Service) DeleteMessage(ctx context.Context, chatID, messageID, userID uuid.UUID, scope string) error {
	if scope != DeleteScopeMe && scope != DeleteScopeAll {
		return ErrInvalidScope
	}
	if err := s.requireMember(ctx, chatID, userID); err != nil {
		return err
	}
	if scope == DeleteScopeAll {
		if _, err := s.loadOwnMessage(ctx, chatID, messageID, userID); err != nil {
			return err
		}
		return s.repo.MarkMessageDeleted(ctx, messageID)
	}
	// scope=me: any member may hide a message they can see.
	msg, err := s.repo.GetMessage(ctx, messageID)
	if err != nil {
		return err
	}
	if msg == nil || msg.ChatID != chatID {
		return ErrMessageNotFound
	}
	return s.repo.HideMessageForUser(ctx, messageID, userID)
}

// notifyNewMessage emails every other member of the chat about a new message.
// Best-effort and async: failures are logged, never surfaced to the sender.
func (s *Service) notifyNewMessage(senderID, chatID uuid.UUID, msg *Message) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ch, err := s.repo.GetChat(ctx, chatID)
	if err != nil || ch == nil {
		return
	}
	ids, err := s.repo.ListMemberIDs(ctx, chatID)
	if err != nil {
		return
	}
	sender, _ := s.users.GetUser(ctx, senderID)
	senderName := displayName(sender)
	preview := messagePreview(msg)

	var subject, intro string
	if ch.Type == ChatTypeGroup {
		subject = fmt.Sprintf("Новое сообщение в «%s»", ch.Title)
		intro = fmt.Sprintf("%s: %s", senderName, preview)
	} else {
		subject = fmt.Sprintf("Новое сообщение от %s", senderName)
		intro = preview
	}
	body := fmt.Sprintf("%s\n\nОткрыть: %s/chat/%s", intro, s.baseURL, chatID)
	url := fmt.Sprintf("/chat/%s", chatID)
	tag := "chat-" + chatID.String()

	for _, id := range ids {
		if id == senderID {
			continue
		}
		u, err := s.users.GetUser(ctx, id)
		if err != nil || u == nil {
			continue
		}
		// Push first; fall back to email only if no device was reached.
		delivered := 0
		if s.push != nil {
			delivered = s.push.SendToUser(ctx, id, subject, intro, url, tag)
		}
		if delivered == 0 && u.Email != "" {
			if err := s.mail.Send(u.Email, subject, body); err != nil {
				log.Printf("chat: notify new message to %s: %v", u.Email, err)
			}
		}
	}
}

// notifyAddedToGroup emails a user who was just added to a group chat.
func (s *Service) notifyAddedToGroup(adderID, chatID, targetID uuid.UUID) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ch, err := s.repo.GetChat(ctx, chatID)
	if err != nil || ch == nil || ch.Type != ChatTypeGroup {
		return
	}
	target, err := s.users.GetUser(ctx, targetID)
	if err != nil || target == nil {
		return
	}
	adder, _ := s.users.GetUser(ctx, adderID)
	subject := "Вас добавили в группу"
	intro := fmt.Sprintf("%s добавил вас в группу «%s».", displayName(adder), ch.Title)
	body := fmt.Sprintf("%s\n\nОткрыть: %s/chat/%s", intro, s.baseURL, chatID)

	delivered := 0
	if s.push != nil {
		delivered = s.push.SendToUser(ctx, targetID, subject, intro, fmt.Sprintf("/chat/%s", chatID), "chat-"+chatID.String())
	}
	if delivered == 0 && target.Email != "" {
		if err := s.mail.Send(target.Email, subject, body); err != nil {
			log.Printf("chat: notify added-to-group to %s: %v", target.Email, err)
		}
	}
}

// displayName picks a human label for notification text, with safe fallbacks.
func displayName(u *models.User) string {
	if u == nil {
		return "Кто-то"
	}
	if strings.TrimSpace(u.Name) != "" {
		return u.Name
	}
	if u.Username != "" {
		return "@" + u.Username
	}
	return "Кто-то"
}

// messagePreview renders a short, email-friendly preview of a message.
func messagePreview(m *Message) string {
	if m == nil {
		return "Новое сообщение"
	}
	if strings.TrimSpace(m.Content) != "" {
		r := []rune(m.Content)
		if len(r) > 140 {
			return string(r[:140]) + "…"
		}
		return m.Content
	}
	switch m.MessageType {
	case MessageTypeImage:
		return "📷 Фото"
	case MessageTypeVideo:
		return "🎥 Видео"
	case MessageTypeVideoNote:
		return "⭕ Видео-сообщение"
	case MessageTypeAudio:
		return "🎤 Голосовое сообщение"
	case MessageTypeFile:
		return "📎 Файл"
	default:
		return "Новое сообщение"
	}
}
