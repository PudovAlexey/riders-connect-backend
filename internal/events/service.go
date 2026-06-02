package events

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"riders-connect/internal/models"
)

var (
	ErrNotFound            = errors.New("event not found")
	ErrForbidden           = errors.New("forbidden")
	ErrInvalidType         = errors.New("invalid event type")
	ErrInvalidVisibility   = errors.New("invalid visibility")
	ErrStartRequired       = errors.New("starts_at is required")
	ErrDestinationRequired = errors.New("trip requires a destination")
	ErrInvalidDestination  = errors.New("invalid destination coordinates")
	ErrInvalidDetails      = errors.New("invalid details")
	ErrInvalidStatus       = errors.New("invalid status")
	ErrUserNotFound        = errors.New("user not found")
	ErrNotInvited          = errors.New("no pending invitation")
)

// userResolver lets the events service turn @logins into user IDs (same shape
// the chat and contacts services use).
type userResolver interface {
	GetByUsername(ctx context.Context, username string) (*models.User, error)
}

type Service struct {
	repo  *Repository
	users userResolver
}

func NewService(repo *Repository, users userResolver) *Service {
	return &Service{repo: repo, users: users}
}

type CreateParams struct {
	Type        string
	Title       string
	Description string
	StartsAt    time.Time
	Visibility  string
	Details     json.RawMessage
	InviteeIDs  []uuid.UUID
}

type UpdateParams struct {
	Type        *string
	Title       *string
	Description *string
	StartsAt    *time.Time
	Visibility  *string
	Details     *json.RawMessage
}

func (s *Service) ResolveUsername(ctx context.Context, username string) (uuid.UUID, error) {
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	u, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		return uuid.Nil, err
	}
	if u == nil {
		return uuid.Nil, ErrUserNotFound
	}
	return u.ID, nil
}

func (s *Service) Create(ctx context.Context, ownerID uuid.UUID, p CreateParams) (Event, error) {
	if p.Type == "" {
		p.Type = TypeMeeting
	}
	if p.Visibility == "" {
		p.Visibility = VisibilityPrivate
	}
	if !validEventTypes[p.Type] {
		return Event{}, ErrInvalidType
	}
	if !validVisibilities[p.Visibility] {
		return Event{}, ErrInvalidVisibility
	}
	if p.StartsAt.IsZero() {
		return Event{}, ErrStartRequired
	}
	if err := validateDetails(p.Type, p.Details); err != nil {
		return Event{}, err
	}

	return s.repo.Create(ctx, Event{
		OwnerID:     ownerID,
		Type:        p.Type,
		Title:       strings.TrimSpace(p.Title),
		Description: p.Description,
		StartsAt:    p.StartsAt,
		Visibility:  p.Visibility,
		Details:     p.Details,
	}, p.InviteeIDs)
}

// GetItem returns an event enriched with participants, enforcing visibility:
// public events are readable by anyone, private events only by participants.
func (s *Service) GetItem(ctx context.Context, id, userID uuid.UUID) (*EventItem, error) {
	e, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrNotFound
	}
	_, status, isMember, err := s.repo.GetMembership(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	if e.Visibility != VisibilityPublic && !isMember {
		return nil, ErrForbidden
	}
	participants, err := s.repo.ListParticipants(ctx, id)
	if err != nil {
		return nil, err
	}
	return &EventItem{Event: *e, Participants: participants, MyStatus: status}, nil
}

// List returns events for the given scope: "feed" (default), "mine", or "invitations".
func (s *Service) List(ctx context.Context, userID uuid.UUID, scope string) ([]Event, error) {
	switch scope {
	case "mine":
		return s.repo.ListMine(ctx, userID)
	case "invitations":
		return s.repo.ListInvitations(ctx, userID)
	default:
		return s.repo.ListFeed(ctx, userID)
	}
}

func (s *Service) Update(ctx context.Context, id, userID uuid.UUID, p UpdateParams) (*Event, error) {
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrNotFound
	}
	if existing.OwnerID != userID {
		return nil, ErrForbidden
	}

	merged := *existing
	if p.Type != nil {
		merged.Type = *p.Type
	}
	if p.Title != nil {
		merged.Title = strings.TrimSpace(*p.Title)
	}
	if p.Description != nil {
		merged.Description = *p.Description
	}
	if p.StartsAt != nil {
		merged.StartsAt = *p.StartsAt
	}
	if p.Visibility != nil {
		merged.Visibility = *p.Visibility
	}
	if p.Details != nil {
		merged.Details = *p.Details
	}

	if !validEventTypes[merged.Type] {
		return nil, ErrInvalidType
	}
	if !validVisibilities[merged.Visibility] {
		return nil, ErrInvalidVisibility
	}
	if merged.StartsAt.IsZero() {
		return nil, ErrStartRequired
	}
	if err := validateDetails(merged.Type, merged.Details); err != nil {
		return nil, err
	}

	found, err := s.repo.Update(ctx, id, userID, merged)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNotFound
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id, userID uuid.UUID) error {
	e, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if e == nil {
		return ErrNotFound
	}
	if e.OwnerID != userID {
		return ErrForbidden
	}
	found, err := s.repo.Delete(ctx, id, userID)
	if err != nil {
		return err
	}
	if !found {
		return ErrNotFound
	}
	return nil
}

// Invite adds a pending invitation. The caller must be an accepted participant.
func (s *Service) Invite(ctx context.Context, eventID, callerID, targetID uuid.UUID) error {
	e, err := s.repo.Get(ctx, eventID)
	if err != nil {
		return err
	}
	if e == nil {
		return ErrNotFound
	}
	_, status, found, err := s.repo.GetMembership(ctx, eventID, callerID)
	if err != nil {
		return err
	}
	if !found || status != StatusAccepted {
		return ErrForbidden
	}
	return s.repo.AddInvite(ctx, eventID, targetID, callerID)
}

// Respond lets an invited user accept or decline their pending invitation.
func (s *Service) Respond(ctx context.Context, eventID, userID uuid.UUID, status string) error {
	if status != StatusAccepted && status != StatusDeclined {
		return ErrInvalidStatus
	}
	_, cur, found, err := s.repo.GetMembership(ctx, eventID, userID)
	if err != nil {
		return err
	}
	if !found || cur != StatusPending {
		return ErrNotInvited
	}
	_, err = s.repo.SetStatus(ctx, eventID, userID, status)
	return err
}

// RemoveParticipant removes a participant. The owner may remove anyone; a
// participant may remove only themselves (leave). The owner cannot be removed.
func (s *Service) RemoveParticipant(ctx context.Context, eventID, callerID, targetID uuid.UUID) error {
	e, err := s.repo.Get(ctx, eventID)
	if err != nil {
		return err
	}
	if e == nil {
		return ErrNotFound
	}
	if targetID == e.OwnerID {
		return ErrForbidden
	}
	if callerID != e.OwnerID && callerID != targetID {
		return ErrForbidden
	}
	found, err := s.repo.RemoveParticipant(ctx, eventID, targetID)
	if err != nil {
		return err
	}
	if !found {
		return ErrNotFound
	}
	return nil
}

func validateDetails(eventType string, raw json.RawMessage) error {
	if eventType == TypeTrip {
		var d tripDetails
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &d); err != nil {
				return ErrInvalidDetails
			}
		}
		if d.Destination == nil {
			return ErrDestinationRequired
		}
		if d.Destination.Lat < -90 || d.Destination.Lat > 90 ||
			d.Destination.Lon < -180 || d.Destination.Lon > 180 {
			return ErrInvalidDestination
		}
		return nil
	}
	if len(raw) > 0 && !json.Valid(raw) {
		return ErrInvalidDetails
	}
	return nil
}
