package routes

import (
	"encoding/json"
	"net/http"

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
	case ErrNotFound:
		respond.Error(w, http.StatusNotFound, err.Error())
	case ErrForbidden:
		respond.Error(w, http.StatusForbidden, err.Error())
	case ErrNameRequired, ErrNoPoints, ErrTooManyPoints, ErrInvalidPoint:
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

// ListSuggested returns the curated recommendation routes (empty until seeded).
func (h *Handler) ListSuggested(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	list, err := h.svc.List(r.Context(), userID, "suggested")
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	respond.JSON(w, http.StatusOK, list)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	var req struct {
		Name        string       `json:"name"`
		Description string       `json:"description"`
		Points      []RoutePoint `json:"points"`
		DistanceM   float64      `json:"distance_m"`
		DurationS   float64      `json:"duration_s"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	route, err := h.svc.Create(r.Context(), userID, CreateParams{
		Name:        req.Name,
		Description: req.Description,
		Points:      req.Points,
		DistanceM:   req.DistanceM,
		DurationS:   req.DurationS,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	respond.JSON(w, http.StatusCreated, route)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	route, err := h.svc.Get(r.Context(), id, userID)
	if err != nil {
		writeErr(w, err)
		return
	}
	respond.JSON(w, http.StatusOK, route)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	var req struct {
		Name        *string       `json:"name"`
		Description *string       `json:"description"`
		Points      *[]RoutePoint `json:"points"`
		DistanceM   *float64      `json:"distance_m"`
		DurationS   *float64      `json:"duration_s"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	route, err := h.svc.Update(r.Context(), id, userID, UpdateParams{
		Name:        req.Name,
		Description: req.Description,
		Points:      req.Points,
		DistanceM:   req.DistanceM,
		DurationS:   req.DurationS,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	respond.JSON(w, http.StatusOK, route)
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
