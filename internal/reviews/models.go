package reviews

import (
	"time"

	"github.com/google/uuid"
)

// Review is a single user's review of a map point: a 1–5 star rating with
// optional text and photos. The author profile fields (Name/Username/AvatarURL)
// are joined from users on read. One review per (point_id, user_id) — see the
// UNIQUE constraint in migration 000015; POST upserts.
type Review struct {
	ID        uuid.UUID `json:"id"`
	PointID   uuid.UUID `json:"point_id"`
	UserID    uuid.UUID `json:"user_id"`
	Rating    int       `json:"rating"`
	Text      string    `json:"text"`
	Photos    []string  `json:"photos"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Author profile fields, joined from users on read.
	Name      string `json:"name"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
}
