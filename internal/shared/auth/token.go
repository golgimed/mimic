// Package auth provides the simulator's trivial API-key check: both
// providers require a token header on real requests. The simulator only
// checks presence, not a real credential, since it isn't reproducing
// provider auth/authorization.
package auth

import (
	"encoding/json"
	"net/http"
)

// RequireAPIToken returns middleware that 401s if headerName is absent.
func RequireAPIToken(headerName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get(headerName) == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"code":    "UNAUTHORIZED",
					"message": "Missing " + headerName + " header",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
