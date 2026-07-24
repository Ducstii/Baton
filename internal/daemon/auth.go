package daemon

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// auth wraps an http.HandlerFunc with Bearer token authentication.
func (d *Daemon) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.config.Token == "" {
			writeError(w, http.StatusUnauthorized, "auth_disabled", "no token configured")
			return
		}
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid Authorization header")
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(d.config.Token)) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid token")
			return
		}
		next(w, r)
	}
}
