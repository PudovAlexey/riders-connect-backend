package contacts

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

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	list, err := h.svc.List(r.Context(), userID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if list == nil {
		list = []*Contact{}
	}
	respond.JSON(w, http.StatusOK, list)
}

func (h *Handler) Add(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	var req struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	list, err := h.svc.AddByUsername(r.Context(), userID, req.Username)
	switch err {
	case nil:
		respond.JSON(w, http.StatusOK, list)
	case ErrUserNotFound:
		respond.Error(w, http.StatusNotFound, "user not found")
	case ErrSelfContact:
		respond.Error(w, http.StatusBadRequest, "cannot add yourself")
	default:
		respond.Error(w, http.StatusInternalServerError, "internal error")
	}
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.Remove(r.Context(), userID, id); err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
