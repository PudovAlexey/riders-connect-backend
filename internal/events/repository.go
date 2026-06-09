package events

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

// eventColumns is aliased for SELECTs that join event_participants as `e`.
const eventColumns = `e.id, e.owner_id, e.type, e.title, e.description, e.starts_at, e.visibility, e.details, e.photos, e.created_at, e.updated_at`

// eventColumnsBare is the same list without the alias, for INSERT ... RETURNING.
const eventColumnsBare = `id, owner_id, type, title, description, starts_at, visibility, details, photos, created_at, updated_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanEvent(s rowScanner) (Event, error) {
	var e Event
	var details []byte
	var photosRaw []byte
	if err := s.Scan(&e.ID, &e.OwnerID, &e.Type, &e.Title, &e.Description,
		&e.StartsAt, &e.Visibility, &details, &photosRaw, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return Event{}, err
	}
	if len(details) > 0 {
		e.Details = json.RawMessage(details)
	} else {
		e.Details = json.RawMessage("{}")
	}
	if len(photosRaw) > 0 {
		json.Unmarshal(photosRaw, &e.Photos) //nolint:errcheck
	}
	if e.Photos == nil {
		e.Photos = []string{}
	}
	return e, nil
}

func detailsArg(d json.RawMessage) []byte {
	if len(d) == 0 {
		return []byte("{}")
	}
	return []byte(d)
}

func photosArg(photos []string) []byte {
	if photos == nil {
		photos = []string{}
	}
	b, _ := json.Marshal(photos) //nolint:errcheck
	return b
}

// Create inserts the event, adds the owner as an accepted participant, and adds
// any initial invitees as pending participants — all in a single transaction.
func (r *Repository) Create(ctx context.Context, e Event, inviteeIDs []uuid.UUID) (Event, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Event{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	created, err := scanEvent(tx.QueryRowContext(ctx, `
		INSERT INTO events (owner_id, type, title, description, starts_at, visibility, details, photos)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+eventColumnsBare,
		e.OwnerID, e.Type, e.Title, e.Description, e.StartsAt, e.Visibility, detailsArg(e.Details), photosArg(e.Photos)))
	if err != nil {
		return Event{}, err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO event_participants (event_id, user_id, role, status)
		VALUES ($1, $2, 'owner', 'accepted')
	`, created.ID, e.OwnerID); err != nil {
		return Event{}, err
	}

	for _, uid := range inviteeIDs {
		if uid == e.OwnerID {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO event_participants (event_id, user_id, role, status, invited_by)
			VALUES ($1, $2, 'participant', 'pending', $3)
			ON CONFLICT (event_id, user_id) DO NOTHING
		`, created.ID, uid, e.OwnerID); err != nil {
			return Event{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return Event{}, err
	}
	return created, nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (*Event, error) {
	e, err := scanEvent(r.db.QueryRowContext(ctx, `
		SELECT `+eventColumns+` FROM events e WHERE e.id = $1
	`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *Repository) queryEvents(ctx context.Context, query string, args ...any) ([]Event, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []Event{}
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// ListFeed returns every public event plus any event the user participates in.
func (r *Repository) ListFeed(ctx context.Context, userID uuid.UUID) ([]Event, error) {
	return r.queryEvents(ctx, `
		SELECT `+eventColumns+`
		FROM events e
		LEFT JOIN event_participants p ON p.event_id = e.id AND p.user_id = $1
		WHERE e.visibility = 'public' OR p.user_id IS NOT NULL
		ORDER BY e.starts_at
	`, userID)
}

// ListMine returns events the user owns or has accepted (owner rows are accepted).
func (r *Repository) ListMine(ctx context.Context, userID uuid.UUID) ([]Event, error) {
	return r.queryEvents(ctx, `
		SELECT `+eventColumns+`
		FROM events e
		JOIN event_participants p ON p.event_id = e.id AND p.user_id = $1 AND p.status = 'accepted'
		ORDER BY e.starts_at
	`, userID)
}

// ListInvitations returns events the user has a pending invitation to.
func (r *Repository) ListInvitations(ctx context.Context, userID uuid.UUID) ([]Event, error) {
	return r.queryEvents(ctx, `
		SELECT `+eventColumns+`
		FROM events e
		JOIN event_participants p ON p.event_id = e.id AND p.user_id = $1 AND p.status = 'pending'
		ORDER BY e.starts_at
	`, userID)
}

// Update applies mutable fields, scoped to the owner. Returns false if no event
// matched (missing or not owned).
func (r *Repository) Update(ctx context.Context, id, ownerID uuid.UUID, e Event) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE events
		SET type = $3, title = $4, description = $5, starts_at = $6, visibility = $7, details = $8, photos = $9, updated_at = NOW()
		WHERE id = $1 AND owner_id = $2
	`, id, ownerID, e.Type, e.Title, e.Description, e.StartsAt, e.Visibility, detailsArg(e.Details), photosArg(e.Photos))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *Repository) Delete(ctx context.Context, id, ownerID uuid.UUID) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM events WHERE id = $1 AND owner_id = $2
	`, id, ownerID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// GetMembership returns the caller's role and status for an event, if any.
func (r *Repository) GetMembership(ctx context.Context, eventID, userID uuid.UUID) (role, status string, found bool, err error) {
	err = r.db.QueryRowContext(ctx, `
		SELECT role, status FROM event_participants WHERE event_id = $1 AND user_id = $2
	`, eventID, userID).Scan(&role, &status)
	if err == sql.ErrNoRows {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return role, status, true, nil
}

func (r *Repository) ListParticipants(ctx context.Context, eventID uuid.UUID) ([]Participant, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT p.user_id, u.name, COALESCE(u.username, ''), u.avatar_url, p.role, p.status
		FROM event_participants p
		JOIN users u ON u.id = p.user_id
		WHERE p.event_id = $1
		ORDER BY p.created_at
	`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Participant{}
	for rows.Next() {
		var p Participant
		if err := rows.Scan(&p.UserID, &p.Name, &p.Username, &p.AvatarURL, &p.Role, &p.Status); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

// AddInvite creates a pending invitation; a no-op if the user is already a participant.
func (r *Repository) AddInvite(ctx context.Context, eventID, targetID, invitedBy uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO event_participants (event_id, user_id, role, status, invited_by)
		VALUES ($1, $2, 'participant', 'pending', $3)
		ON CONFLICT (event_id, user_id) DO NOTHING
	`, eventID, targetID, invitedBy)
	return err
}

func (r *Repository) SetStatus(ctx context.Context, eventID, userID uuid.UUID, status string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE event_participants SET status = $3 WHERE event_id = $1 AND user_id = $2
	`, eventID, userID, status)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *Repository) RemoveParticipant(ctx context.Context, eventID, userID uuid.UUID) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM event_participants WHERE event_id = $1 AND user_id = $2
	`, eventID, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
