package garage

import (
	"context"

	"github.com/google/uuid"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]Vehicle, error) {
	return s.repo.ListByUser(ctx, userID)
}

func (s *Service) Add(ctx context.Context, userID uuid.UUID, make, model string, year int, color, notes string, photos []string) (Vehicle, error) {
	return s.repo.Create(ctx, Vehicle{
		UserID: userID,
		Make:   make,
		Model:  model,
		Year:   year,
		Color:  color,
		Notes:  notes,
		Photos: photos,
	})
}

func (s *Service) Delete(ctx context.Context, id, userID uuid.UUID) (bool, error) {
	return s.repo.Delete(ctx, id, userID)
}
