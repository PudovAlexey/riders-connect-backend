package geo

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type UserLocation struct {
	UserID    uuid.UUID `json:"user_id"`
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	UpdatedAt time.Time `json:"updated_at"`
	Name      string    `json:"name"`
	AvatarURL string    `json:"avatar_url"`
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Upsert(ctx context.Context, userID uuid.UUID, lat, lon float64) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO user_locations (user_id, lat, lon)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET lat = $2, lon = $3, updated_at = NOW()
	`, userID, lat, lon)
	return err
}

func (r *Repository) GetAll(ctx context.Context) ([]*UserLocation, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT ul.user_id, ul.lat, ul.lon, ul.updated_at,
		       COALESCE(u.name, '') AS name, COALESCE(u.avatar_url, '') AS avatar_url
		FROM user_locations ul
		LEFT JOIN users u ON ul.user_id = u.id
		WHERE ul.updated_at > NOW() - INTERVAL '1 hour'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var locs []*UserLocation
	for rows.Next() {
		l := &UserLocation{}
		if err := rows.Scan(&l.UserID, &l.Lat, &l.Lon, &l.UpdatedAt, &l.Name, &l.AvatarURL); err != nil {
			return nil, err
		}
		locs = append(locs, l)
	}
	return locs, rows.Err()
}
