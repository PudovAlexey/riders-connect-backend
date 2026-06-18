package reviews

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrNotFound      = errors.New("review not found")
	ErrPointNotFound = errors.New("point not found")
	ErrInvalidRating = errors.New("rating must be between 1 and 5")
	ErrTooManyPhotos = errors.New("too many photos")
)

const (
	maxPhotos  = 5
	maxTextLen = 2000
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

type UpsertParams struct {
	Rating int
	Text   string
	Photos []string
}

// Upsert validates and stores the caller's review for a point, returning the
// enriched review (with author profile).
func (s *Service) Upsert(ctx context.Context, pointID, userID uuid.UUID, p UpsertParams) (*Review, error) {
	if p.Rating < 1 || p.Rating > 5 {
		return nil, ErrInvalidRating
	}
	if p.Photos == nil {
		p.Photos = []string{}
	}
	if len(p.Photos) > maxPhotos {
		return nil, ErrTooManyPhotos
	}
	p.Text = strings.TrimSpace(p.Text)
	if len(p.Text) > maxTextLen {
		p.Text = p.Text[:maxTextLen]
	}

	exists, err := s.repo.PointExists(ctx, pointID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrPointNotFound
	}

	id, err := s.repo.Upsert(ctx, pointID, userID, p.Rating, p.Text, p.Photos)
	if err != nil {
		return nil, err
	}
	rv, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if rv == nil {
		return nil, ErrNotFound
	}
	return rv, nil
}

func (s *Service) ListByPoint(ctx context.Context, pointID uuid.UUID) ([]Review, error) {
	return s.repo.ListByPoint(ctx, pointID)
}

// Delete removes the caller's review of a point.
func (s *Service) Delete(ctx context.Context, pointID, userID uuid.UUID) error {
	found, err := s.repo.DeleteByPointAndUser(ctx, pointID, userID)
	if err != nil {
		return err
	}
	if !found {
		return ErrNotFound
	}
	return nil
}
