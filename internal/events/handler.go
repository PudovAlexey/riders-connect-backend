package events

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"riders-connect/internal/middleware"
	"riders-connect/internal/respond"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// writeErr maps service errors to HTTP status codes.
func writeErr(w http.ResponseWriter, err error) {
	switch err {
	case ErrNotFound, ErrUserNotFound:
		respond.Error(w, http.StatusNotFound, err.Error())
	case ErrForbidden:
		respond.Error(w, http.StatusForbidden, err.Error())
	case ErrInvalidType, ErrInvalidVisibility, ErrStartRequired,
		ErrDestinationRequired, ErrInvalidDestination, ErrInvalidDetails,
		ErrInvalidStatus, ErrNotInvited:
		respond.Error(w, http.StatusBadRequest, err.Error())
	default:
		respond.Error(w, http.StatusInternalServerError, "internal error")
	}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	scope := r.URL.Query().Get("scope")
	list, err := h.svc.List(r.Context(), userID, scope)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	respond.JSON(w, http.StatusOK, list)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	var req struct {
		Type        string          `json:"type"`
		Title       string          `json:"title"`
		Description string          `json:"description"`
		StartsAt    time.Time       `json:"starts_at"`
		Visibility  string          `json:"visibility"`
		Details     json.RawMessage `json:"details"`
		InviteeIDs  []uuid.UUID     `json:"invitee_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	event, err := h.svc.Create(r.Context(), userID, CreateParams{
		Type:        req.Type,
		Title:       req.Title,
		Description: req.Description,
		StartsAt:    req.StartsAt,
		Visibility:  req.Visibility,
		Details:     req.Details,
		InviteeIDs:  req.InviteeIDs,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	respond.JSON(w, http.StatusCreated, event)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	item, err := h.svc.GetItem(r.Context(), id, userID)
	if err != nil {
		writeErr(w, err)
		return
	}
	respond.JSON(w, http.StatusOK, item)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	var req struct {
		Type        *string          `json:"type"`
		Title       *string          `json:"title"`
		Description *string          `json:"description"`
		StartsAt    *time.Time       `json:"starts_at"`
		Visibility  *string          `json:"visibility"`
		Details     *json.RawMessage `json:"details"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	event, err := h.svc.Update(r.Context(), id, userID, UpdateParams{
		Type:        req.Type,
		Title:       req.Title,
		Description: req.Description,
		StartsAt:    req.StartsAt,
		Visibility:  req.Visibility,
		Details:     req.Details,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	respond.JSON(w, http.StatusOK, event)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.Delete(r.Context(), id, userID); err != nil {
		writeErr(w, err)
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) Invite(w http.ResponseWriter, r *http.Request) {
	callerID, _ := middleware.GetUserID(r.Context())
	eventID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	var req struct {
		Username string     `json:"username"`
		UserID   *uuid.UUID `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	var targetID uuid.UUID
	switch {
	case req.UserID != nil:
		targetID = *req.UserID
	case strings.TrimSpace(req.Username) != "":
		targetID, err = h.svc.ResolveUsername(r.Context(), req.Username)
		if err != nil {
			writeErr(w, err)
			return
		}
	default:
		respond.Error(w, http.StatusBadRequest, "username or user_id required")
		return
	}

	if err := h.svc.Invite(r.Context(), eventID, callerID, targetID); err != nil {
		writeErr(w, err)
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) Respond(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	eventID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	if err := h.svc.Respond(r.Context(), eventID, userID, req.Status); err != nil {
		writeErr(w, err)
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) RemoveParticipant(w http.ResponseWriter, r *http.Request) {
	callerID, _ := middleware.GetUserID(r.Context())
	eventID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	targetID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid user id")
		return
	}

	if err := h.svc.RemoveParticipant(r.Context(), eventID, callerID, targetID); err != nil {
		writeErr(w, err)
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
