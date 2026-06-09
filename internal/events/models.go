package events

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	TypeMeeting      = "meeting"
	TypeTrip         = "trip"
	TypePhotosession = "photosession"
)

var validEventTypes = map[string]bool{
	TypeMeeting:      true,
	TypeTrip:         true,
	TypePhotosession: true,
}

const (
	VisibilityPublic  = "public"
	VisibilityPrivate = "private"
)

var validVisibilities = map[string]bool{
	VisibilityPublic:  true,
	VisibilityPrivate: true,
}

const (
	RoleOwner       = "owner"
	RoleParticipant = "participant"
)

const (
	StatusPending  = "pending"
	StatusAccepted = "accepted"
	StatusDeclined = "declined"
)

// Event is the stored event. Details holds per-type extra fields as raw JSON
// (validated per type in the service), mirroring chat.Message.AttachmentMeta.
type Event struct {
	ID          uuid.UUID       `json:"id"`
	OwnerID     uuid.UUID       `json:"owner_id"`
	Type        string          `json:"type"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	StartsAt    time.Time       `json:"starts_at"`
	Visibility  string          `json:"visibility"`
	Details     json.RawMessage `json:"details"`
	Photos      []string        `json:"photos"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// Destination is the required extra field for trip events.
type Destination struct {
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	Address string  `json:"address"`
}

// tripDetails is the typed shape of Event.Details when Type == TypeTrip.
type tripDetails struct {
	Destination *Destination `json:"destination"`
}

// Participant is an event member enriched with user profile fields.
type Participant struct {
	UserID    uuid.UUID `json:"user_id"`
	Name      string    `json:"name"`
	Username  string    `json:"username"`
	AvatarURL string    `json:"avatar_url"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
}

// EventItem is the enriched event shape returned to clients: the event plus its
// participants and the current user's own status.
type EventItem struct {
	Event
	Participants []Participant `json:"participants"`
	MyStatus     string        `json:"my_status,omitempty"`
}
