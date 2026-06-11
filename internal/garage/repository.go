package garage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type Vehicle struct {
	ID           uuid.UUID     `json:"id"`
	UserID       uuid.UUID     `json:"user_id"`
	Make         string        `json:"make"`
	Model        string        `json:"model"`
	Year         int           `json:"year"`
	Color        string        `json:"color"`
	Notes        string        `json:"notes"`
	Photos       []string      `json:"photos"`
	Mileage      int           `json:"mileage"`
	ServiceItems []ServiceItem `json:"service_items"`
	CreatedAt    time.Time     `json:"created_at"`
}

type ServiceItem struct {
	ID                 uuid.UUID  `json:"id"`
	VehicleID          uuid.UUID  `json:"vehicle_id"`
	Kind               string     `json:"kind"`
	Title              string     `json:"title"`
	IntervalKm         int        `json:"interval_km"`
	LastServiceMileage int        `json:"last_service_mileage"`
	LastServiceAt      *time.Time `json:"last_service_at"`
	TimesDone          int        `json:"times_done"`
	// Computed at read time, not stored:
	RemainingKm int    `json:"remaining_km"`
	Status      string `json:"status"`
}

// VehicleUpdate carries optional fields for a partial vehicle update.
// A nil pointer means "leave unchanged".
type VehicleUpdate struct {
	Make    *string
	Model   *string
	Year    *int
	Color   *string
	Notes   *string
	Photos  *[]string
	Mileage *int
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListByUser(ctx context.Context, userID uuid.UUID) ([]Vehicle, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, make, model, year, color, notes, photos, mileage, created_at
		FROM garage_vehicles
		WHERE user_id = $1
		ORDER BY created_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var vehicles []Vehicle
	var ids []string
	for rows.Next() {
		var v Vehicle
		var photosRaw []byte
		if err := rows.Scan(&v.ID, &v.UserID, &v.Make, &v.Model, &v.Year,
			&v.Color, &v.Notes, &photosRaw, &v.Mileage, &v.CreatedAt); err != nil {
			return nil, err
		}
		if len(photosRaw) > 0 {
			json.Unmarshal(photosRaw, &v.Photos) //nolint:errcheck
		}
		if v.Photos == nil {
			v.Photos = []string{}
		}
		v.ServiceItems = []ServiceItem{}
		vehicles = append(vehicles, v)
		ids = append(ids, v.ID.String())
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if vehicles == nil {
		return []Vehicle{}, nil
	}

	itemsByVehicle, err := r.serviceItemsForVehicles(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range vehicles {
		if items, ok := itemsByVehicle[vehicles[i].ID]; ok {
			vehicles[i].ServiceItems = items
		}
	}
	return vehicles, nil
}

func (r *Repository) serviceItemsForVehicles(ctx context.Context, vehicleIDs []string) (map[uuid.UUID][]ServiceItem, error) {
	out := map[uuid.UUID][]ServiceItem{}
	if len(vehicleIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, vehicle_id, kind, title, interval_km, last_service_mileage, last_service_at, times_done
		FROM garage_service_items
		WHERE vehicle_id = ANY($1::uuid[])
		ORDER BY created_at
	`, pq.Array(vehicleIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		item, err := scanServiceItem(rows)
		if err != nil {
			return nil, err
		}
		out[item.VehicleID] = append(out[item.VehicleID], item)
	}
	return out, rows.Err()
}

func (r *Repository) Create(ctx context.Context, v Vehicle) (Vehicle, error) {
	photosJSON, err := json.Marshal(v.Photos)
	if err != nil {
		return Vehicle{}, err
	}

	var result Vehicle
	var photosRaw []byte
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO garage_vehicles (user_id, make, model, year, color, notes, photos, mileage)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, user_id, make, model, year, color, notes, photos, mileage, created_at
	`, v.UserID, v.Make, v.Model, v.Year, v.Color, v.Notes, photosJSON, v.Mileage).
		Scan(&result.ID, &result.UserID, &result.Make, &result.Model, &result.Year,
			&result.Color, &result.Notes, &photosRaw, &result.Mileage, &result.CreatedAt)
	if err != nil {
		return Vehicle{}, err
	}
	if len(photosRaw) > 0 {
		json.Unmarshal(photosRaw, &result.Photos) //nolint:errcheck
	}
	if result.Photos == nil {
		result.Photos = []string{}
	}
	result.ServiceItems = []ServiceItem{}
	return result, nil
}

// UpdateVehicle applies a partial update. Returns (vehicle, found, error).
func (r *Repository) UpdateVehicle(ctx context.Context, id, userID uuid.UUID, u VehicleUpdate) (Vehicle, bool, error) {
	var set []string
	var args []any
	n := 1
	add := func(col string, val any) {
		set = append(set, fmt.Sprintf("%s = $%d", col, n))
		args = append(args, val)
		n++
	}
	if u.Make != nil {
		add("make", *u.Make)
	}
	if u.Model != nil {
		add("model", *u.Model)
	}
	if u.Year != nil {
		add("year", *u.Year)
	}
	if u.Color != nil {
		add("color", *u.Color)
	}
	if u.Notes != nil {
		add("notes", *u.Notes)
	}
	if u.Photos != nil {
		photosJSON, err := json.Marshal(*u.Photos)
		if err != nil {
			return Vehicle{}, false, err
		}
		add("photos", photosJSON)
	}
	if u.Mileage != nil {
		add("mileage", *u.Mileage)
	}
	if len(set) == 0 {
		return Vehicle{}, false, fmt.Errorf("no fields to update")
	}

	args = append(args, id, userID)
	query := fmt.Sprintf(`
		UPDATE garage_vehicles
		SET %s
		WHERE id = $%d AND user_id = $%d
		RETURNING id, user_id, make, model, year, color, notes, photos, mileage, created_at
	`, strings.Join(set, ", "), n, n+1)

	var result Vehicle
	var photosRaw []byte
	err := r.db.QueryRowContext(ctx, query, args...).
		Scan(&result.ID, &result.UserID, &result.Make, &result.Model, &result.Year,
			&result.Color, &result.Notes, &photosRaw, &result.Mileage, &result.CreatedAt)
	if err == sql.ErrNoRows {
		return Vehicle{}, false, nil
	}
	if err != nil {
		return Vehicle{}, false, err
	}
	if len(photosRaw) > 0 {
		json.Unmarshal(photosRaw, &result.Photos) //nolint:errcheck
	}
	if result.Photos == nil {
		result.Photos = []string{}
	}
	result.ServiceItems = []ServiceItem{}
	return result, true, nil
}

func (r *Repository) Delete(ctx context.Context, id, userID uuid.UUID) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM garage_vehicles WHERE id = $1 AND user_id = $2
	`, id, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// AddServiceItem inserts a tracked item, seeding last_service_mileage from the
// vehicle's current odometer. Returns (item, found, error); found is false when
// the vehicle does not exist or is not owned by the user.
func (r *Repository) AddServiceItem(ctx context.Context, vehicleID, userID uuid.UUID, kind, title string, intervalKm int) (ServiceItem, bool, error) {
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO garage_service_items (vehicle_id, kind, title, interval_km, last_service_mileage, last_service_at)
		SELECT id, $2, $3, $4, mileage, NOW()
		FROM garage_vehicles
		WHERE id = $1 AND user_id = $5
		RETURNING id, vehicle_id, kind, title, interval_km, last_service_mileage, last_service_at, times_done
	`, vehicleID, kind, title, intervalKm, userID)
	item, err := scanServiceItem(row)
	if err == sql.ErrNoRows {
		return ServiceItem{}, false, nil
	}
	if err != nil {
		return ServiceItem{}, false, err
	}
	return item, true, nil
}

func (r *Repository) UpdateServiceItem(ctx context.Context, itemID, userID uuid.UUID, title string, intervalKm int) (ServiceItem, bool, error) {
	row := r.db.QueryRowContext(ctx, `
		UPDATE garage_service_items s
		SET title = $3, interval_km = $4
		FROM garage_vehicles v
		WHERE s.id = $1 AND s.vehicle_id = v.id AND v.user_id = $2
		RETURNING s.id, s.vehicle_id, s.kind, s.title, s.interval_km, s.last_service_mileage, s.last_service_at, s.times_done
	`, itemID, userID, title, intervalKm)
	item, err := scanServiceItem(row)
	if err == sql.ErrNoRows {
		return ServiceItem{}, false, nil
	}
	if err != nil {
		return ServiceItem{}, false, err
	}
	return item, true, nil
}

// ResetServiceItem records a completed service: snapshots the vehicle's current
// mileage and bumps the counter.
func (r *Repository) ResetServiceItem(ctx context.Context, itemID, userID uuid.UUID) (ServiceItem, bool, error) {
	row := r.db.QueryRowContext(ctx, `
		UPDATE garage_service_items s
		SET last_service_mileage = v.mileage, last_service_at = NOW(), times_done = s.times_done + 1
		FROM garage_vehicles v
		WHERE s.id = $1 AND s.vehicle_id = v.id AND v.user_id = $2
		RETURNING s.id, s.vehicle_id, s.kind, s.title, s.interval_km, s.last_service_mileage, s.last_service_at, s.times_done
	`, itemID, userID)
	item, err := scanServiceItem(row)
	if err == sql.ErrNoRows {
		return ServiceItem{}, false, nil
	}
	if err != nil {
		return ServiceItem{}, false, err
	}
	return item, true, nil
}

func (r *Repository) DeleteServiceItem(ctx context.Context, itemID, userID uuid.UUID) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM garage_service_items s
		USING garage_vehicles v
		WHERE s.id = $1 AND s.vehicle_id = v.id AND v.user_id = $2
	`, itemID, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanServiceItem(s scanner) (ServiceItem, error) {
	var item ServiceItem
	var lastAt sql.NullTime
	if err := s.Scan(&item.ID, &item.VehicleID, &item.Kind, &item.Title,
		&item.IntervalKm, &item.LastServiceMileage, &lastAt, &item.TimesDone); err != nil {
		return ServiceItem{}, err
	}
	if lastAt.Valid {
		t := lastAt.Time
		item.LastServiceAt = &t
	}
	return item, nil
}
