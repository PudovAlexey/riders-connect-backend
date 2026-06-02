package contacts

import (
	"time"

	"github.com/google/uuid"
)

// Contact is an owner's saved contact, enriched with the contact user's info.
type Contact struct {
	ID            uuid.UUID `json:"id"`
	ContactUserID uuid.UUID `json:"contact_user_id"`
	Username      string    `json:"username"`
	Name          string    `json:"name"`
	AvatarURL     string    `json:"avatar_url"`
	Alias         string    `json:"alias"`
	CreatedAt     time.Time `json:"created_at"`
}
