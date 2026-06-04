package points

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrNotFound        = errors.New("point not found")
	ErrForbidden       = errors.New("forbidden")
	ErrInvalidCategory = errors.New("invalid category")
	ErrNameRequired    = errors.New("name is required")
	ErrInvalidCoords   = errors.New("invalid coordinates")
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

type CreateParams struct {
	Category    string
	Name        string
	Description string
	Lat         float64
	Lon         float64
	Address     string
	Phone       string
	Website     string
	Hours       string
	Photos      []string
}

type UpdateParams struct {
	Category    *string
	Name        *string
	Description *string
	Lat         *float64
	Lon         *float64
	Address     *string
	Phone       *string
	Website     *string
	Hours       *string
	Photos      *[]string
}

// Bounds is the map viewport a List query is scoped to.
type Bounds struct {
	MinLat, MaxLat, MinLon, MaxLon float64
}

func validCoords(lat, lon float64) bool {
	return lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, p CreateParams) (Point, error) {
	if !validCategories[p.Category] {
		return Point{}, ErrInvalidCategory
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return Point{}, ErrNameRequired
	}
	if !validCoords(p.Lat, p.Lon) {
		return Point{}, ErrInvalidCoords
	}
	if p.Photos == nil {
		p.Photos = []string{}
	}
	return s.repo.Create(ctx, Point{
		CreatedBy:   userID,
		Category:    p.Category,
		Name:        p.Name,
		Description: p.Description,
		Lat:         p.Lat,
		Lon:         p.Lon,
		Address:     p.Address,
		Phone:       p.Phone,
		Website:     p.Website,
		Hours:       p.Hours,
		Photos:      p.Photos,
	})
}

// ListInBounds returns points inside the viewport, optionally narrowed to the
// given categories. Unknown categories are silently dropped.
func (s *Service) ListInBounds(ctx context.Context, b Bounds, categories []string) ([]*Point, error) {
	if b.MinLat > b.MaxLat {
		b.MinLat, b.MaxLat = b.MaxLat, b.MinLat
	}
	if b.MinLon > b.MaxLon {
		b.MinLon, b.MaxLon = b.MaxLon, b.MinLon
	}
	// Non-nil slice: pq.Array encodes a nil slice as SQL NULL (which would make
	// the cardinality()=0 filter fail and drop every row), but an empty non-nil
	// slice as '{}', correctly disabling the category filter.
	filtered := make([]string, 0, len(categories))
	for _, c := range categories {
		if validCategories[c] {
			filtered = append(filtered, c)
		}
	}
	return s.repo.GetInBounds(ctx, b.MinLat, b.MaxLat, b.MinLon, b.MaxLon, filtered)
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Point, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrNotFound
	}
	return p, nil
}

func (s *Service) Update(ctx context.Context, id, userID uuid.UUID, p UpdateParams) (*Point, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrNotFound
	}
	if existing.CreatedBy != userID {
		return nil, ErrForbidden
	}

	merged := *existing
	if p.Category != nil {
		merged.Category = *p.Category
	}
	if p.Name != nil {
		merged.Name = strings.TrimSpace(*p.Name)
	}
	if p.Description != nil {
		merged.Description = *p.Description
	}
	if p.Lat != nil {
		merged.Lat = *p.Lat
	}
	if p.Lon != nil {
		merged.Lon = *p.Lon
	}
	if p.Address != nil {
		merged.Address = *p.Address
	}
	if p.Phone != nil {
		merged.Phone = *p.Phone
	}
	if p.Website != nil {
		merged.Website = *p.Website
	}
	if p.Hours != nil {
		merged.Hours = *p.Hours
	}
	if p.Photos != nil {
		merged.Photos = *p.Photos
	}

	if !validCategories[merged.Category] {
		return nil, ErrInvalidCategory
	}
	if merged.Name == "" {
		return nil, ErrNameRequired
	}
	if !validCoords(merged.Lat, merged.Lon) {
		return nil, ErrInvalidCoords
	}

	found, err := s.repo.Update(ctx, id, userID, merged)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNotFound
	}
	return s.repo.GetByID(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id, userID uuid.UUID) error {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrNotFound
	}
	if existing.CreatedBy != userID {
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
