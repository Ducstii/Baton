package daemon

import (
	"net/http"
	"strings"
)

// auth wraps an http.HandlerFunc with Bearer token authentication.
// The health endpoint is registered without this wrapper.
func (d *Daemon) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.config.Token == "" {
			writeError(w, http.StatusUnauthorized, "auth_disabled", "no token configured")
			return
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid Authorization header")
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		if token != d.config.Token {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid token")
			return
		}
		next(w, r)
	}
}
