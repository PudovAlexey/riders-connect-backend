package routes

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

const routeColumns = `id, owner_id, name, description, points, distance_m, duration_s, is_suggested, created_at, updated_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRoute(s rowScanner) (Route, error) {
	var r Route
	var pointsRaw []byte
	if err := s.Scan(&r.ID, &r.OwnerID, &r.Name, &r.Description, &pointsRaw,
		&r.DistanceM, &r.DurationS, &r.IsSuggested, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return Route{}, err
	}
	if len(pointsRaw) > 0 {
		json.Unmarshal(pointsRaw, &r.Points) //nolint:errcheck
	}
	if r.Points == nil {
		r.Points = []RoutePoint{}
	}
	return r, nil
}

func pointsArg(points []RoutePoint) []byte {
	if points == nil {
		points = []RoutePoint{}
	}
	b, _ := json.Marshal(points) //nolint:errcheck
	return b
}

func (r *Repository) Create(ctx context.Context, rt Route) (Route, error) {
	return scanRoute(r.db.QueryRowContext(ctx, `
		INSERT INTO routes (owner_id, name, description, points, distance_m, duration_s, is_suggested)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+routeColumns,
		rt.OwnerID, rt.Name, rt.Description, pointsArg(rt.Points), rt.DistanceM, rt.DurationS, rt.IsSuggested))
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (*Route, error) {
	rt, err := scanRoute(r.db.QueryRowContext(ctx, `
		SELECT `+routeColumns+` FROM routes WHERE id = $1
	`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rt, nil
}

func (r *Repository) queryRoutes(ctx context.Context, query string, args ...any) ([]Route, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Route{}
	for rows.Next() {
		rt, err := scanRoute(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, rt)
	}
	return list, rows.Err()
}

// ListMine returns the user's own (non-suggested) routes, newest first.
func (r *Repository) ListMine(ctx context.Context, ownerID uuid.UUID) ([]Route, error) {
	return r.queryRoutes(ctx, `
		SELECT `+routeColumns+`
		FROM routes
		WHERE owner_id = $1 AND is_suggested = FALSE
		ORDER BY created_at DESC
	`, ownerID)
}

// ListSuggested returns curated recommendation routes, newest first. Empty until
// suggested routes are seeded.
func (r *Repository) ListSuggested(ctx context.Context) ([]Route, error) {
	return r.queryRoutes(ctx, `
		SELECT `+routeColumns+`
		FROM routes
		WHERE is_suggested = TRUE
		ORDER BY created_at DESC
	`)
}

// Update applies mutable fields, scoped to the owner. Returns false if no route
// matched (missing or not owned).
func (r *Repository) Update(ctx context.Context, id, ownerID uuid.UUID, rt Route) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE routes
		SET name = $3, description = $4, points = $5, distance_m = $6, duration_s = $7, updated_at = NOW()
		WHERE id = $1 AND owner_id = $2
	`, id, ownerID, rt.Name, rt.Description, pointsArg(rt.Points), rt.DistanceM, rt.DurationS)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *Repository) Delete(ctx context.Context, id, ownerID uuid.UUID) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM routes WHERE id = $1 AND owner_id = $2
	`, id, ownerID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
