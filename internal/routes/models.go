package routes

import (
	"time"

	"github.com/google/uuid"
)

// A route holds between MinPoints and MaxPoints ordered waypoints.
const (
	MinPoints = 1
	MaxPoints = 10
)

// RoutePoint is a single ordered waypoint. Order is implicit (the array index in
// Route.Points). PointID is set when the waypoint was added from the points DB;
// it is a soft reference (no FK), so deleting that POI does not break the route.
type RoutePoint struct {
	Lat     float64    `json:"lat"`
	Lon     float64    `json:"lon"`
	Name    string     `json:"name"`
	Address string     `json:"address"`
	PointID *uuid.UUID `json:"point_id,omitempty"`
}

// Route is a saved ride: an ordered list of waypoints. DistanceM/DurationS are
// denormalized values computed client-side via ORS and echoed back on read so the
// list/detail screens need not recompute. IsSuggested marks curated routes and is
// a server-only flag (clients always create non-suggested routes).
type Route struct {
	ID          uuid.UUID    `json:"id"`
	OwnerID     uuid.UUID    `json:"owner_id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Points      []RoutePoint `json:"points"`
	DistanceM   float64      `json:"distance_m"`
	DurationS   float64      `json:"duration_s"`
	IsSuggested bool         `json:"is_suggested"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}
