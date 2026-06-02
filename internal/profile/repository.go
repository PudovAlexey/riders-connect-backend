package profile

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"riders-connect/internal/models"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

const userCols = `id, email, COALESCE(username, ''), name, avatar_url, created_at, updated_at`

func scanUser(row interface {
	Scan(dest ...any) error
}) (*models.User, error) {
	u := &models.User{}
	err := row.Scan(&u.ID, &u.Email, &u.Username, &u.Name, &u.AvatarURL, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, `
		SELECT `+userCols+` FROM users WHERE id = $1
	`, id))
}

func (r *Repository) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, `
		SELECT `+userCols+` FROM users WHERE lower(username) = lower($1)
	`, username))
}

func (r *Repository) Update(ctx context.Context, id uuid.UUID, name, avatarURL string) (*models.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, `
		UPDATE users SET name = $2, avatar_url = $3, updated_at = NOW()
		WHERE id = $1
		RETURNING `+userCols+`
	`, id, name, avatarURL))
}

func (r *Repository) UpdateUsername(ctx context.Context, id uuid.UUID, username string) (*models.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, `
		UPDATE users SET username = $2, updated_at = NOW()
		WHERE id = $1
		RETURNING `+userCols+`
	`, id, username))
}

func (r *Repository) SearchUsers(ctx context.Context, query string, excludeID uuid.UUID, limit int) ([]*models.User, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+userCols+` FROM users
		WHERE id <> $2 AND username IS NOT NULL
		  AND (username ILIKE '%' || $1 || '%' OR name ILIKE '%' || $1 || '%')
		ORDER BY username
		LIMIT $3
	`, query, excludeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		u := &models.User{}
		if err := rows.Scan(&u.ID, &u.Email, &u.Username, &u.Name, &u.AvatarURL, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}
