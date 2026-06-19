package routes

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrNotFound      = errors.New("route not found")
	ErrForbidden     = errors.New("forbidden")
	ErrNameRequired  = errors.New("name is required")
	ErrNoPoints      = errors.New("at least one point is required")
	ErrTooManyPoints = errors.New("a route can have at most 10 points")
	ErrInvalidPoint  = errors.New("invalid point coordinates")
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

type CreateParams struct {
	Name        string
	Description string
	Points      []RoutePoint
	DistanceM   float64
	DurationS   float64
}

type UpdateParams struct {
	Name        *string
	Description *string
	Points      *[]RoutePoint
	DistanceM   *float64
	DurationS   *float64
}

func validatePoints(points []RoutePoint) error {
	if len(points) < MinPoints {
		return ErrNoPoints
	}
	if len(points) > MaxPoints {
		return ErrTooManyPoints
	}
	for _, p := range points {
		if p.Lat < -90 || p.Lat > 90 || p.Lon < -180 || p.Lon > 180 {
			return ErrInvalidPoint
		}
	}
	return nil
}

func (s *Service) Create(ctx context.Context, ownerID uuid.UUID, p CreateParams) (Route, error) {
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return Route{}, ErrNameRequired
	}
	if err := validatePoints(p.Points); err != nil {
		return Route{}, err
	}
	return s.repo.Create(ctx, Route{
		OwnerID:     ownerID,
		Name:        name,
		Description: strings.TrimSpace(p.Description),
		Points:      p.Points,
		DistanceM:   p.DistanceM,
		DurationS:   p.DurationS,
		IsSuggested: false, // clients never create suggested routes
	})
}

// Get returns a route, enforcing visibility: a route is readable by its owner, or
// by anyone if it is a suggested (curated) route.
func (s *Service) Get(ctx context.Context, id, userID uuid.UUID) (*Route, error) {
	rt, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if rt == nil {
		return nil, ErrNotFound
	}
	if rt.OwnerID != userID && !rt.IsSuggested {
		return nil, ErrForbidden
	}
	return rt, nil
}

// List returns routes for the given scope: "suggested" (curated) or the caller's
// own routes (default).
func (s *Service) List(ctx context.Context, userID uuid.UUID, scope string) ([]Route, error) {
	if scope == "suggested" {
		return s.repo.ListSuggested(ctx)
	}
	return s.repo.ListMine(ctx, userID)
}

func (s *Service) Update(ctx context.Context, id, userID uuid.UUID, p UpdateParams) (*Route, error) {
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrNotFound
	}
	if existing.OwnerID != userID {
		return nil, ErrForbidden
	}

	merged := *existing
	if p.Name != nil {
		merged.Name = strings.TrimSpace(*p.Name)
	}
	if p.Description != nil {
		merged.Description = strings.TrimSpace(*p.Description)
	}
	if p.Points != nil {
		merged.Points = *p.Points
	}
	if p.DistanceM != nil {
		merged.DistanceM = *p.DistanceM
	}
	if p.DurationS != nil {
		merged.DurationS = *p.DurationS
	}

	if merged.Name == "" {
		return nil, ErrNameRequired
	}
	if err := validatePoints(merged.Points); err != nil {
		return nil, err
	}

	found, err := s.repo.Update(ctx, id, userID, merged)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNotFound
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id, userID uuid.UUID) error {
	rt, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if rt == nil {
		return ErrNotFound
	}
	if rt.OwnerID != userID {
		return ErrForbidden
	}
	found, err := s.repo.Delete(ctx, id, userID)
	if err != nil {
		return err
	}
	if !found {
		return ErrNotFound
	}
	return nil
}
