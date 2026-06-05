package middleware

import (
	"net/http"
	"strings"
)

// CORS allows browser clients from the given origins to call the API.
// `origins` is a comma-separated list, or "*" (or empty) for any origin.
// Auth travels in the Authorization header (not cookies), so echoing the
// request origin — even under a wildcard — is safe and avoids the
// "*"-with-credentials restriction.
func CORS(origins string) func(http.Handler) http.Handler {
	wildcard := strings.TrimSpace(origins) == "" || strings.TrimSpace(origins) == "*"
	allowed := map[string]bool{}
	for _, o := range strings.Split(origins, ",") {
		if o = strings.TrimSpace(o); o != "" {
			allowed[o] = true
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (wildcard || allowed[origin]) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
