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
		Make    string   `json:"make"`
		Model   string   `json:"model"`
		Year    int      `json:"year"`
		Color   string   `json:"color"`
		Notes   string   `json:"notes"`
		Photos  []string `json:"photos"`
		Mileage int      `json:"mileage"`
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
	if req.Mileage < 0 {
		req.Mileage = 0
	}

	vehicle, err := h.svc.Add(r.Context(), userID, req.Make, req.Model, req.Year, req.Color, req.Notes, req.Photos, req.Mileage)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	respond.JSON(w, http.StatusCreated, vehicle)
}

// UpdateVehicle applies a partial update (primarily the odometer/mileage).
func (h *Handler) UpdateVehicle(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	var req struct {
		Make    *string   `json:"make"`
		Model   *string   `json:"model"`
		Year    *int      `json:"year"`
		Color   *string   `json:"color"`
		Notes   *string   `json:"notes"`
		Photos  *[]string `json:"photos"`
		Mileage *int      `json:"mileage"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.Mileage != nil && *req.Mileage < 0 {
		respond.Error(w, http.StatusBadRequest, "mileage must be >= 0")
		return
	}

	vehicle, found, err := h.svc.UpdateVehicle(r.Context(), id, userID, VehicleUpdate{
		Make:    req.Make,
		Model:   req.Model,
		Year:    req.Year,
		Color:   req.Color,
		Notes:   req.Notes,
		Photos:  req.Photos,
		Mileage: req.Mileage,
	})
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "nothing to update")
		return
	}
	if !found {
		respond.Error(w, http.StatusNotFound, "not found")
		return
	}
	respond.JSON(w, http.StatusOK, vehicle)
}

// AddServiceItem starts tracking a maintenance parameter for a vehicle.
func (h *Handler) AddServiceItem(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	vehicleID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	var req struct {
		Kind       string `json:"kind"`
		Title      string `json:"title"`
		IntervalKm int    `json:"interval_km"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.Kind == "" {
		respond.Error(w, http.StatusBadRequest, "kind is required")
		return
	}
	if req.IntervalKm < 0 {
		req.IntervalKm = 0
	}

	item, found, err := h.svc.AddServiceItem(r.Context(), vehicleID, userID, req.Kind, req.Title, req.IntervalKm)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !found {
		respond.Error(w, http.StatusNotFound, "vehicle not found")
		return
	}
	respond.JSON(w, http.StatusCreated, item)
}

// UpdateServiceItem edits a tracked item's title and/or interval.
func (h *Handler) UpdateServiceItem(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	itemID, err := uuid.Parse(chi.URLParam(r, "itemID"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	var req struct {
		Title      string `json:"title"`
		IntervalKm int    `json:"interval_km"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.IntervalKm < 0 {
		req.IntervalKm = 0
	}

	item, found, err := h.svc.UpdateServiceItem(r.Context(), itemID, userID, req.Title, req.IntervalKm)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !found {
		respond.Error(w, http.StatusNotFound, "not found")
		return
	}
	respond.JSON(w, http.StatusOK, item)
}

// ResetServiceItem records a completed service (resets the counter to the
// current mileage and increments the done count).
func (h *Handler) ResetServiceItem(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	itemID, err := uuid.Parse(chi.URLParam(r, "itemID"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	item, found, err := h.svc.ResetServiceItem(r.Context(), itemID, userID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !found {
		respond.Error(w, http.StatusNotFound, "not found")
		return
	}
	respond.JSON(w, http.StatusOK, item)
}

// DeleteServiceItem stops tracking a maintenance parameter.
func (h *Handler) DeleteServiceItem(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	itemID, err := uuid.Parse(chi.URLParam(r, "itemID"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	found, err := h.svc.DeleteServiceItem(r.Context(), itemID, userID)
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
