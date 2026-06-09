package push

import (
	"time"

	"github.com/google/uuid"
)

// Subscription is a stored Web Push subscription for one browser/device.
type Subscription struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Endpoint  string    `json:"endpoint"`
	P256dh    string    `json:"p256dh"`
	Auth      string    `json:"auth"`
	CreatedAt time.Time `json:"created_at"`
}
