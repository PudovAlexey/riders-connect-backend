package garage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Vehicle struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Make      string    `json:"make"`
	Model     string    `json:"model"`
	Year      int       `json:"year"`
	Color     string    `json:"color"`
	Notes     string    `json:"notes"`
	Photos    []string  `json:"photos"`
	CreatedAt time.Time `json:"created_at"`
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListByUser(ctx context.Context, userID uuid.UUID) ([]Vehicle, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, make, model, year, color, notes, photos, created_at
		FROM garage_vehicles
		WHERE user_id = $1
		ORDER BY created_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var vehicles []Vehicle
	for rows.Next() {
		var v Vehicle
		var photosRaw []byte
		if err := rows.Scan(&v.ID, &v.UserID, &v.Make, &v.Model, &v.Year,
			&v.Color, &v.Notes, &photosRaw, &v.CreatedAt); err != nil {
			return nil, err
		}
		if len(photosRaw) > 0 {
			json.Unmarshal(photosRaw, &v.Photos) //nolint:errcheck
		}
		if v.Photos == nil {
			v.Photos = []string{}
		}
		vehicles = append(vehicles, v)
	}
	if vehicles == nil {
		vehicles = []Vehicle{}
	}
	return vehicles, rows.Err()
}

func (r *Repository) Create(ctx context.Context, v Vehicle) (Vehicle, error) {
	photosJSON, err := json.Marshal(v.Photos)
	if err != nil {
		return Vehicle{}, err
	}

	var result Vehicle
	var photosRaw []byte
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO garage_vehicles (user_id, make, model, year, color, notes, photos)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, user_id, make, model, year, color, notes, photos, created_at
	`, v.UserID, v.Make, v.Model, v.Year, v.Color, v.Notes, photosJSON).
		Scan(&result.ID, &result.UserID, &result.Make, &result.Model, &result.Year,
			&result.Color, &result.Notes, &photosRaw, &result.CreatedAt)
	if err != nil {
		return Vehicle{}, err
	}
	if len(photosRaw) > 0 {
		json.Unmarshal(photosRaw, &result.Photos) //nolint:errcheck
	}
	if result.Photos == nil {
		result.Photos = []string{}
	}
	return result, nil
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
