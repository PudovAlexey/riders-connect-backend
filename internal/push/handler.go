package push

import (
	"encoding/json"
	"net/http"
	"strings"

	"riders-connect/internal/middleware"
	"riders-connect/internal/respond"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// PublicKey returns the VAPID public key the browser needs to subscribe.
func (h *Handler) PublicKey(w http.ResponseWriter, r *http.Request) {
	respond.JSON(w, http.StatusOK, map[string]string{"public_key": h.svc.PublicKey()})
}

// subscriptionRequest matches the browser's PushSubscription.toJSON() shape.
type subscriptionRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// Register stores (or refreshes) the caller's push subscription.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	var req subscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	if strings.TrimSpace(req.Endpoint) == "" || req.Keys.P256dh == "" || req.Keys.Auth == "" {
		respond.Error(w, http.StatusBadRequest, "endpoint and keys required")
		return
	}
	if err := h.svc.Save(r.Context(), userID, req.Endpoint, req.Keys.P256dh, req.Keys.Auth); err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Unregister removes a subscription by its endpoint.
func (h *Handler) Unregister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Endpoint) == "" {
		respond.Error(w, http.StatusBadRequest, "endpoint required")
		return
	}
	if err := h.svc.Delete(r.Context(), req.Endpoint); err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
