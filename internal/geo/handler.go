package geo

import (
	"encoding/json"
	"net/http"

	"riders-connect/internal/middleware"
	"riders-connect/internal/respond"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) UpdateLocation(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	var req struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := h.svc.UpdateLocation(r.Context(), userID, req.Lat, req.Lon); err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) GetUsersOnMap(w http.ResponseWriter, r *http.Request) {
	locs, err := h.svc.GetUsersOnMap(r.Context())
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if locs == nil {
		locs = []*UserLocation{}
	}
	respond.JSON(w, http.StatusOK, locs)
}
