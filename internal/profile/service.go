package profile

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"riders-connect/internal/models"
)

var ErrUsernameTaken = errors.New("username taken")
var ErrInvalidUsername = errors.New("invalid username")

var usernameRe = regexp.MustCompile(`^[a-z0-9_]{3,32}$`)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetUser(ctx context.Context, id uuid.UUID) (*models.User, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	return s.repo.GetByUsername(ctx, normalizeUsername(username))
}

func (s *Service) UpdateUser(ctx context.Context, id uuid.UUID, name, avatarURL string) (*models.User, error) {
	return s.repo.Update(ctx, id, name, avatarURL)
}

func (s *Service) UpdateUsername(ctx context.Context, id uuid.UUID, username string) (*models.User, error) {
	username = normalizeUsername(username)
	if !usernameRe.MatchString(username) {
		return nil, ErrInvalidUsername
	}
	u, err := s.repo.UpdateUsername(ctx, id, username)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return nil, ErrUsernameTaken
		}
		return nil, err
	}
	return u, nil
}

func (s *Service) SearchUsers(ctx context.Context, query string, excludeID uuid.UUID) ([]*models.User, error) {
	query = strings.TrimPrefix(strings.TrimSpace(query), "@")
	if query == "" {
		return []*models.User{}, nil
	}
	return s.repo.SearchUsers(ctx, query, excludeID, 20)
}

func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(username), "@"))
}
