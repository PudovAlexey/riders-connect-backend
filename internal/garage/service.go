package garage

import (
	"context"

	"github.com/google/uuid"
)

// soonThresholdKm is how many km before the interval an item flips to "soon".
const soonThresholdKm = 500

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]Vehicle, error) {
	vehicles, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range vehicles {
		for j := range vehicles[i].ServiceItems {
			computeStatus(&vehicles[i].ServiceItems[j], vehicles[i].Mileage)
		}
	}
	return vehicles, nil
}

func (s *Service) Add(ctx context.Context, userID uuid.UUID, make, model string, year int, color, notes string, photos []string, mileage int) (Vehicle, error) {
	return s.repo.Create(ctx, Vehicle{
		UserID:  userID,
		Make:    make,
		Model:   model,
		Year:    year,
		Color:   color,
		Notes:   notes,
		Photos:  photos,
		Mileage: mileage,
	})
}

func (s *Service) UpdateVehicle(ctx context.Context, id, userID uuid.UUID, u VehicleUpdate) (Vehicle, bool, error) {
	return s.repo.UpdateVehicle(ctx, id, userID, u)
}

func (s *Service) Delete(ctx context.Context, id, userID uuid.UUID) (bool, error) {
	return s.repo.Delete(ctx, id, userID)
}

func (s *Service) AddServiceItem(ctx context.Context, vehicleID, userID uuid.UUID, kind, title string, intervalKm int) (ServiceItem, bool, error) {
	return s.repo.AddServiceItem(ctx, vehicleID, userID, kind, title, intervalKm)
}

func (s *Service) UpdateServiceItem(ctx context.Context, itemID, userID uuid.UUID, title string, intervalKm int) (ServiceItem, bool, error) {
	return s.repo.UpdateServiceItem(ctx, itemID, userID, title, intervalKm)
}

func (s *Service) ResetServiceItem(ctx context.Context, itemID, userID uuid.UUID) (ServiceItem, bool, error) {
	return s.repo.ResetServiceItem(ctx, itemID, userID)
}

func (s *Service) DeleteServiceItem(ctx context.Context, itemID, userID uuid.UUID) (bool, error) {
	return s.repo.DeleteServiceItem(ctx, itemID, userID)
}

// computeStatus derives the remaining distance and status for a service item
// given the vehicle's current mileage. Not persisted — recomputed on every read.
func computeStatus(it *ServiceItem, mileage int) {
	it.RemainingKm = it.IntervalKm - (mileage - it.LastServiceMileage)
	switch {
	case it.IntervalKm <= 0:
		it.Status = "untracked"
	case it.RemainingKm <= 0:
		it.Status = "overdue"
	case it.RemainingKm <= soonThresholdKm:
		it.Status = "soon"
	default:
		it.Status = "ok"
	}
}
