package points

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

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
	case ErrInvalidCategory, ErrNameRequired, ErrInvalidCoords:
		respond.Error(w, http.StatusBadRequest, err.Error())
	default:
		respond.Error(w, http.StatusInternalServerError, "internal error")
	}
}

// List returns points inside the map viewport, optionally filtered by categories.
//
//	GET /points?min_lat=&max_lat=&min_lon=&max_lon=&categories=towing,bar
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	minLat, err1 := strconv.ParseFloat(q.Get("min_lat"), 64)
	maxLat, err2 := strconv.ParseFloat(q.Get("max_lat"), 64)
	minLon, err3 := strconv.ParseFloat(q.Get("min_lon"), 64)
	maxLon, err4 := strconv.ParseFloat(q.Get("max_lon"), 64)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		respond.Error(w, http.StatusBadRequest, "min_lat, max_lat, min_lon, max_lon are required")
		return
	}

	categories := parseCategories(q.Get("categories"))

	points, err := h.svc.ListInBounds(r.Context(), Bounds{
		MinLat: minLat, MaxLat: maxLat, MinLon: minLon, MaxLon: maxLon,
	}, categories)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if points == nil {
		points = []*Point{}
	}
	respond.JSON(w, http.StatusOK, points)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	var req struct {
		Category    string   `json:"category"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Lat         float64  `json:"lat"`
		Lon         float64  `json:"lon"`
		Address     string   `json:"address"`
		Phone       string   `json:"phone"`
		Website     string   `json:"website"`
		Hours       string   `json:"hours"`
		Photos      []string `json:"photos"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	point, err := h.svc.Create(r.Context(), userID, CreateParams{
		Category:    req.Category,
		Name:        req.Name,
		Description: req.Description,
		Lat:         req.Lat,
		Lon:         req.Lon,
		Address:     req.Address,
		Phone:       req.Phone,
		Website:     req.Website,
		Hours:       req.Hours,
		Photos:      req.Photos,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	respond.JSON(w, http.StatusCreated, point)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	point, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	respond.JSON(w, http.StatusOK, point)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	var req struct {
		Category    *string   `json:"category"`
		Name        *string   `json:"name"`
		Description *string   `json:"description"`
		Lat         *float64  `json:"lat"`
		Lon         *float64  `json:"lon"`
		Address     *string   `json:"address"`
		Phone       *string   `json:"phone"`
		Website     *string   `json:"website"`
		Hours       *string   `json:"hours"`
		Photos      *[]string `json:"photos"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	point, err := h.svc.Update(r.Context(), id, userID, UpdateParams{
		Category:    req.Category,
		Name:        req.Name,
		Description: req.Description,
		Lat:         req.Lat,
		Lon:         req.Lon,
		Address:     req.Address,
		Phone:       req.Phone,
		Website:     req.Website,
		Hours:       req.Hours,
		Photos:      req.Photos,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	respond.JSON(w, http.StatusOK, point)
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

// parseCategories splits a comma-separated list, trimming blanks.
func parseCategories(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
