package reviews

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
	case ErrNotFound, ErrPointNotFound:
		respond.Error(w, http.StatusNotFound, err.Error())
	case ErrInvalidRating, ErrTooManyPhotos:
		respond.Error(w, http.StatusBadRequest, err.Error())
	default:
		respond.Error(w, http.StatusInternalServerError, "internal error")
	}
}

// List returns a point's reviews, newest first. Public.
//
//	GET /points/{id}/reviews
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	pointID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	list, err := h.svc.ListByPoint(r.Context(), pointID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if list == nil {
		list = []Review{}
	}
	respond.JSON(w, http.StatusOK, list)
}

// Upsert creates or overwrites the caller's review of a point. Auth required.
//
//	POST /points/{id}/reviews
func (h *Handler) Upsert(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	pointID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	var req struct {
		Rating int      `json:"rating"`
		Text   string   `json:"text"`
		Photos []string `json:"photos"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	rv, err := h.svc.Upsert(r.Context(), pointID, userID, UpsertParams{
		Rating: req.Rating,
		Text:   req.Text,
		Photos: req.Photos,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	respond.JSON(w, http.StatusOK, rv)
}

// Delete removes the caller's review of a point. Auth required.
//
//	DELETE /points/{id}/reviews
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	pointID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.Delete(r.Context(), pointID, userID); err != nil {
		writeErr(w, err)
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
