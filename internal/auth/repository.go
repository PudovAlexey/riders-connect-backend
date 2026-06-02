package auth

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"riders-connect/internal/models"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) UpsertUser(ctx context.Context, email string) (*models.User, error) {
	u := &models.User{}
	var username sql.NullString
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO users (email) VALUES ($1)
		ON CONFLICT (email) DO UPDATE SET updated_at = NOW()
		RETURNING id, email, username, name, avatar_url, created_at, updated_at
	`, email).Scan(&u.ID, &u.Email, &username, &u.Name, &u.AvatarURL, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}

	// Assign a default @login on first login.
	if !username.Valid || username.String == "" {
		generated, err := r.assignDefaultUsername(ctx, u.ID, email)
		if err != nil {
			return nil, err
		}
		username.String = generated
	}
	u.Username = username.String
	return u, nil
}

// assignDefaultUsername derives a username from the email local part and
// resolves collisions by appending an incrementing numeric suffix.
func (r *Repository) assignDefaultUsername(ctx context.Context, id uuid.UUID, email string) (string, error) {
	base := sanitizeUsername(email)
	if len(base) < 3 {
		base = base + "user"
	}
	if len(base) > 28 {
		base = base[:28]
	}

	for i := 0; i < 100; i++ {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s%d", base, i)
		}
		_, err := r.db.ExecContext(ctx, `
			UPDATE users SET username = $2 WHERE id = $1 AND username IS NULL
		`, id, candidate)
		if err == nil {
			return candidate, nil
		}
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			continue // username taken, try next suffix
		}
		return "", err
	}
	return "", fmt.Errorf("could not assign default username")
}

func sanitizeUsername(email string) string {
	local := email
	if i := strings.IndexByte(email, '@'); i >= 0 {
		local = email[:i]
	}
	local = strings.ToLower(local)
	var b strings.Builder
	for _, c := range local {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
			b.WriteRune(c)
		}
	}
	return b.String()
}

func (r *Repository) SaveCode(ctx context.Context, email, code string, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO email_codes (email, code, expires_at) VALUES ($1, $2, $3)
	`, email, code, expiresAt)
	return err
}

func (r *Repository) VerifyCode(ctx context.Context, email, code string) (bool, error) {
	var id uuid.UUID
	err := r.db.QueryRowContext(ctx, `
		UPDATE email_codes
		SET used = TRUE
		WHERE email = $1 AND code = $2 AND used = FALSE AND expires_at > NOW()
		RETURNING id
	`, email, code).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
