package chat

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"riders-connect/internal/models"
)

var ErrNotFound = errors.New("chat not found")
var ErrForbidden = errors.New("forbidden")
var ErrInvalidMessageType = errors.New("invalid message type")
var ErrUserNotFound = errors.New("user not found")
var ErrInvalidGroup = errors.New("group requires a title and at least one other member")

// userResolver lets the chat service turn @logins into user IDs.
type userResolver interface {
	GetByUsername(ctx context.Context, username string) (*models.User, error)
}

type Service struct {
	repo  *Repository
	users userResolver
}

func NewService(repo *Repository, users userResolver) *Service {
	return &Service{repo: repo, users: users}
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
	return s.repo.AddMember(ctx, chatID, targetID)
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
	return s.repo.ListMessages(ctx, chatID, limit, offset)
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
	return s.repo.CreateMessage(ctx, chatID, senderID, p)
}
