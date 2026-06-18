package points

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// pointColumns is the bare column list used by INSERT/UPDATE … RETURNING, where
// no table alias applies and the rating aggregate isn't available yet.
const pointColumns = `id, created_by, category, name, description, lat, lon, address, phone, website, hours, photos, created_at, updated_at`

// pointReadSelect fetches a point joined with its rating aggregate. Points with
// no reviews report avg_rating/review_count as 0/0 (LEFT JOIN + COALESCE). Callers
// append their own WHERE/ORDER BY using the "p" alias and scan with scanPointWithRating.
const pointReadSelect = `
	SELECT p.id, p.created_by, p.category, p.name, p.description, p.lat, p.lon,
	       p.address, p.phone, p.website, p.hours, p.photos, p.created_at, p.updated_at,
	       COALESCE(r.avg_rating, 0), COALESCE(r.review_count, 0)
	FROM points p
	LEFT JOIN (
	  SELECT point_id, AVG(rating)::float8 AS avg_rating, COUNT(*) AS review_count
	  FROM point_reviews GROUP BY point_id
	) r ON r.point_id = p.id`

type rowScanner interface {
	Scan(dest ...any) error
}

func normalizePhotos(raw []byte, p *Point) {
	if len(raw) > 0 {
		json.Unmarshal(raw, &p.Photos) //nolint:errcheck
	}
	if p.Photos == nil {
		p.Photos = []string{}
	}
}

// scanPoint reads the 14 base columns (no rating aggregate); used for INSERT RETURNING.
func scanPoint(s rowScanner) (Point, error) {
	var p Point
	var createdBy uuid.NullUUID
	var photosRaw []byte
	if err := s.Scan(&p.ID, &createdBy, &p.Category, &p.Name, &p.Description,
		&p.Lat, &p.Lon, &p.Address, &p.Phone, &p.Website, &p.Hours,
		&photosRaw, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return Point{}, err
	}
	if createdBy.Valid {
		p.CreatedBy = createdBy.UUID
	}
	normalizePhotos(photosRaw, &p)
	return p, nil
}

// scanPointWithRating reads the base columns plus the avg_rating/review_count
// aggregate appended by pointReadSelect (16 columns).
func scanPointWithRating(s rowScanner) (Point, error) {
	var p Point
	var createdBy uuid.NullUUID
	var photosRaw []byte
	if err := s.Scan(&p.ID, &createdBy, &p.Category, &p.Name, &p.Description,
		&p.Lat, &p.Lon, &p.Address, &p.Phone, &p.Website, &p.Hours,
		&photosRaw, &p.CreatedAt, &p.UpdatedAt, &p.AvgRating, &p.ReviewCount); err != nil {
		return Point{}, err
	}
	if createdBy.Valid {
		p.CreatedBy = createdBy.UUID
	}
	normalizePhotos(photosRaw, &p)
	return p, nil
}

func (r *Repository) Create(ctx context.Context, p Point) (Point, error) {
	photosJSON, err := json.Marshal(p.Photos)
	if err != nil {
		return Point{}, err
	}
	return scanPoint(r.db.QueryRowContext(ctx, `
		INSERT INTO points (created_by, category, name, description, lat, lon, address, phone, website, hours, photos)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING `+pointColumns,
		p.CreatedBy, p.Category, p.Name, p.Description, p.Lat, p.Lon,
		p.Address, p.Phone, p.Website, p.Hours, photosJSON))
}

// GetInBounds returns points inside the lat/lon bounding box. An empty categories
// slice disables the category filter (returns every category in the box).
func (r *Repository) GetInBounds(ctx context.Context, minLat, maxLat, minLon, maxLon float64, categories []string) ([]*Point, error) {
	rows, err := r.db.QueryContext(ctx, pointReadSelect+`
		WHERE p.lat BETWEEN $1 AND $2 AND p.lon BETWEEN $3 AND $4
		  AND (cardinality($5::text[]) = 0 OR p.category = ANY($5))
		ORDER BY p.created_at DESC
	`, minLat, maxLat, minLon, maxLon, pq.Array(categories))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	points := []*Point{}
	for rows.Next() {
		p, err := scanPointWithRating(rows)
		if err != nil {
			return nil, err
		}
		points = append(points, &p)
	}
	return points, rows.Err()
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*Point, error) {
	p, err := scanPointWithRating(r.db.QueryRowContext(ctx,
		pointReadSelect+` WHERE p.id = $1`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Update applies mutable fields, scoped to the creator. Returns false if no point
// matched (missing or not owned).
func (r *Repository) Update(ctx context.Context, id, ownerID uuid.UUID, p Point) (bool, error) {
	photosJSON, err := json.Marshal(p.Photos)
	if err != nil {
		return false, err
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE points
		SET category = $3, name = $4, description = $5, lat = $6, lon = $7,
		    address = $8, phone = $9, website = $10, hours = $11, photos = $12, updated_at = NOW()
		WHERE id = $1 AND created_by = $2
	`, id, ownerID, p.Category, p.Name, p.Description, p.Lat, p.Lon,
		p.Address, p.Phone, p.Website, p.Hours, photosJSON)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *Repository) Delete(ctx context.Context, id, ownerID uuid.UUID) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM points WHERE id = $1 AND created_by = $2
	`, id, ownerID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
