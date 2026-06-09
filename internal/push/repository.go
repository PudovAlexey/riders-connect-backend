package push

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// Upsert stores a subscription, keyed by its unique endpoint. Re-subscribing
// (or a device being claimed by another account) updates the row in place.
func (r *Repository) Upsert(ctx context.Context, userID uuid.UUID, endpoint, p256dh, auth string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (endpoint) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			p256dh  = EXCLUDED.p256dh,
			auth    = EXCLUDED.auth
	`, userID, endpoint, p256dh, auth)
	return err
}

func (r *Repository) ListByUserID(ctx context.Context, userID uuid.UUID) ([]Subscription, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, endpoint, p256dh, auth, created_at
		FROM push_subscriptions
		WHERE user_id = $1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []Subscription
	for rows.Next() {
		var s Subscription
		if err := rows.Scan(&s.ID, &s.UserID, &s.Endpoint, &s.P256dh, &s.Auth, &s.CreatedAt); err != nil {
			return nil, err
		}
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

func (r *Repository) DeleteByEndpoint(ctx context.Context, endpoint string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM push_subscriptions WHERE endpoint = $1
	`, endpoint)
	return err
}
