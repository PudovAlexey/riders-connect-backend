package reviews

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/google/uuid"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// reviewSelect joins the author profile. Callers append their own WHERE/ORDER BY
// using the "rv" (review) and "u" (user) aliases and scan with scanReview.
const reviewSelect = `
	SELECT rv.id, rv.point_id, rv.user_id, rv.rating, rv.text, rv.photos,
	       rv.created_at, rv.updated_at, u.name, COALESCE(u.username, ''), u.avatar_url
	FROM point_reviews rv
	JOIN users u ON u.id = rv.user_id`

func scanReview(s interface{ Scan(...any) error }) (Review, error) {
	var rv Review
	var photosRaw []byte
	if err := s.Scan(&rv.ID, &rv.PointID, &rv.UserID, &rv.Rating, &rv.Text,
		&photosRaw, &rv.CreatedAt, &rv.UpdatedAt, &rv.Name, &rv.Username, &rv.AvatarURL); err != nil {
		return Review{}, err
	}
	if len(photosRaw) > 0 {
		json.Unmarshal(photosRaw, &rv.Photos) //nolint:errcheck
	}
	if rv.Photos == nil {
		rv.Photos = []string{}
	}
	return rv, nil
}

// Upsert creates the caller's review for a point, or overwrites their existing
// one (one review per user per point). Returns the review id.
func (r *Repository) Upsert(ctx context.Context, pointID, userID uuid.UUID, rating int, text string, photos []string) (uuid.UUID, error) {
	photosJSON, err := json.Marshal(photos)
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO point_reviews (point_id, user_id, rating, text, photos)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (point_id, user_id)
		DO UPDATE SET rating = EXCLUDED.rating, text = EXCLUDED.text,
		              photos = EXCLUDED.photos, updated_at = NOW()
		RETURNING id
	`, pointID, userID, rating, text, photosJSON).Scan(&id)
	return id, err
}

// GetByID returns one enriched review, or nil if it doesn't exist.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*Review, error) {
	rv, err := scanReview(r.db.QueryRowContext(ctx, reviewSelect+` WHERE rv.id = $1`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rv, nil
}

// ListByPoint returns a point's reviews, newest first, enriched with author profile.
func (r *Repository) ListByPoint(ctx context.Context, pointID uuid.UUID) ([]Review, error) {
	rows, err := r.db.QueryContext(ctx, reviewSelect+`
		WHERE rv.point_id = $1
		ORDER BY rv.created_at DESC
	`, pointID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Review{}
	for rows.Next() {
		rv, err := scanReview(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, rv)
	}
	return list, rows.Err()
}

// DeleteByPointAndUser removes the caller's review of a point. Returns false if
// they had none.
func (r *Repository) DeleteByPointAndUser(ctx context.Context, pointID, userID uuid.UUID) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM point_reviews WHERE point_id = $1 AND user_id = $2
	`, pointID, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// PointExists reports whether a point row exists, so the service can return a
// clean 404 instead of a foreign-key error on upsert.
func (r *Repository) PointExists(ctx context.Context, pointID uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM points WHERE id = $1)`, pointID).Scan(&exists)
	return exists, err
}
