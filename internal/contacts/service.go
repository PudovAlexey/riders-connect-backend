package contacts

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"riders-connect/internal/models"
)

var ErrUserNotFound = errors.New("user not found")
var ErrSelfContact = errors.New("cannot add yourself")

// userResolver resolves @logins to users.
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

func (s *Service) List(ctx context.Context, ownerID uuid.UUID) ([]*Contact, error) {
	return s.repo.List(ctx, ownerID)
}

func (s *Service) AddByUsername(ctx context.Context, ownerID uuid.UUID, username string) ([]*Contact, error) {
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	u, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, ErrUserNotFound
	}
	if u.ID == ownerID {
		return nil, ErrSelfContact
	}
	if err := s.repo.Add(ctx, ownerID, u.ID, ""); err != nil {
		return nil, err
	}
	return s.repo.List(ctx, ownerID)
}

func (s *Service) Remove(ctx context.Context, ownerID, id uuid.UUID) error {
	return s.repo.Delete(ctx, ownerID, id)
}
