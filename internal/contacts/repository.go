package contacts

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

func (r *Repository) Add(ctx context.Context, ownerID, contactUserID uuid.UUID, alias string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO contacts (owner_id, contact_user_id, alias) VALUES ($1, $2, $3)
		ON CONFLICT (owner_id, contact_user_id) DO NOTHING
	`, ownerID, contactUserID, alias)
	return err
}

func (r *Repository) List(ctx context.Context, ownerID uuid.UUID) ([]*Contact, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.id, c.contact_user_id, COALESCE(u.username, ''), u.name, u.avatar_url, c.alias, c.created_at
		FROM contacts c
		JOIN users u ON u.id = c.contact_user_id
		WHERE c.owner_id = $1
		ORDER BY u.name, u.username
	`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*Contact
	for rows.Next() {
		c := &Contact{}
		if err := rows.Scan(&c.ID, &c.ContactUserID, &c.Username, &c.Name, &c.AvatarURL, &c.Alias, &c.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (r *Repository) Delete(ctx context.Context, ownerID, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM contacts WHERE id = $1 AND owner_id = $2
	`, id, ownerID)
	return err
}
