package garage

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
	vehicles, err := h.svc.List(r.Context(), userID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	respond.JSON(w, http.StatusOK, vehicles)
}

func (h *Handler) Add(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	var req struct {
		Make   string   `json:"make"`
		Model  string   `json:"model"`
		Year   int      `json:"year"`
		Color  string   `json:"color"`
		Notes  string   `json:"notes"`
		Photos []string `json:"photos"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.Make == "" || req.Model == "" || req.Year == 0 {
		respond.Error(w, http.StatusBadRequest, "make, model and year are required")
		return
	}
	if req.Photos == nil {
		req.Photos = []string{}
	}

	vehicle, err := h.svc.Add(r.Context(), userID, req.Make, req.Model, req.Year, req.Color, req.Notes, req.Photos)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	respond.JSON(w, http.StatusCreated, vehicle)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	found, err := h.svc.Delete(r.Context(), id, userID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !found {
		respond.Error(w, http.StatusNotFound, "not found")
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
