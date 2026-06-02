package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"riders-connect/internal/respond"
)

type contextKey string

const UserIDKey contextKey = "userID"

type Auth struct {
	secret []byte
}

func NewAuth(secret string) *Auth {
	return &Auth{secret: []byte(secret)}
}

func (a *Auth) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := extractToken(r)
		if raw == "" {
			respond.Error(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		t, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return a.secret, nil
		})
		if err != nil || !t.Valid {
			respond.Error(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		sub, err := t.Claims.GetSubject()
		if err != nil {
			respond.Error(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		id, err := uuid.Parse(sub)
		if err != nil {
			respond.Error(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		ctx := context.WithValue(r.Context(), UserIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetUserID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(UserIDKey).(uuid.UUID)
	return id, ok
}

func extractToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	// also check query param for WS connections
	return r.URL.Query().Get("token")
}
