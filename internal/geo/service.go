package geo

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

func (s *Service) UpdateLocation(ctx context.Context, userID uuid.UUID, lat, lon float64) error {
	return s.repo.Upsert(ctx, userID, lat, lon)
}

func (s *Service) GetUsersOnMap(ctx context.Context) ([]*UserLocation, error) {
	return s.repo.GetAll(ctx)
}
