package auth

import (
	"encoding/json"
	"net/http"
	"strings"

	"riders-connect/internal/respond"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) SendCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if !strings.Contains(req.Email, "@") {
		respond.Error(w, http.StatusBadRequest, "invalid email")
		return
	}
	if err := h.svc.SendCode(r.Context(), req.Email); err != nil {
		respond.Error(w, http.StatusInternalServerError, "failed to send code")
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"message": "code sent"})
}

func (h *Handler) Verify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	token, user, err := h.svc.Verify(r.Context(), req.Email, strings.TrimSpace(req.Code))
	if err == ErrInvalidCode {
		respond.Error(w, http.StatusUnauthorized, "invalid or expired code")
		return
	}
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "verification failed")
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user":  user,
	})
}
