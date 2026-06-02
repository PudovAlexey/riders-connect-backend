package profile

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"riders-connect/internal/middleware"
	"riders-connect/internal/models"
	"riders-connect/internal/respond"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	user, err := h.svc.GetUser(r.Context(), userID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user == nil {
		respond.Error(w, http.StatusNotFound, "not found")
		return
	}
	respond.JSON(w, http.StatusOK, user)
}

func (h *Handler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	var req struct {
		Name      string  `json:"name"`
		AvatarURL string  `json:"avatar_url"`
		Username  *string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	if req.Username != nil {
		if _, err := h.svc.UpdateUsername(r.Context(), userID, *req.Username); err != nil {
			switch err {
			case ErrUsernameTaken:
				respond.Error(w, http.StatusConflict, "username taken")
			case ErrInvalidUsername:
				respond.Error(w, http.StatusBadRequest, "invalid username")
			default:
				respond.Error(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
	}

	user, err := h.svc.UpdateUser(r.Context(), userID, req.Name, req.AvatarURL)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	respond.JSON(w, http.StatusOK, user)
}

func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	users, err := h.svc.SearchUsers(r.Context(), r.URL.Query().Get("q"), userID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if users == nil {
		users = []*models.User{}
	}
	respond.JSON(w, http.StatusOK, users)
}

func (h *Handler) GetByUsername(w http.ResponseWriter, r *http.Request) {
	user, err := h.svc.GetByUsername(r.Context(), chi.URLParam(r, "username"))
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user == nil {
		respond.Error(w, http.StatusNotFound, "not found")
		return
	}
	respond.JSON(w, http.StatusOK, user)
}

func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid user id")
		return
	}
	user, err := h.svc.GetUser(r.Context(), id)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user == nil {
		respond.Error(w, http.StatusNotFound, "not found")
		return
	}
	respond.JSON(w, http.StatusOK, user)
}
