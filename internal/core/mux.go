// Package core wires the HTTP surface together: health/dashboard routes,
// each enabled provider's own routes, and the admin control-plane routes.
// It owns no provider business logic itself.
package core

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/rluders/lane"

	"github.com/golgimed/mimic/dashboard"
	"github.com/golgimed/mimic/internal/registry"
	"github.com/golgimed/mimic/internal/shared/admin"
)

// NewMux builds the root ServeMux: /health, /ready, /dashboard, every enabled
// provider's routes under /{provider.name}/..., and /admin/*. enabledProviders
// filters which registered providers are served (nil/empty means all).
func NewMux(reg *registry.Registry, db *sql.DB, store *admin.Store, enabledProviders []string, health *lane.HealthState) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", lane.LivenessHandler())
	mux.HandleFunc("GET /ready", lane.ReadinessHandler(health, func(ctx context.Context) error {
		return db.PingContext(ctx)
	}))

	mux.HandleFunc("GET /dashboard", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(dashboard.HTML))
	})

	for _, p := range reg.Enabled(enabledProviders) {
		p.Register(mux)
	}

	admin.RegisterRoutes(mux, db, store, reg, enabledProviders)

	return mux
}

// WithCORS wraps a handler with permissive CORS headers, equivalent to
// Fastify's @fastify/cors with { origin: true }.
func WithCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Vary", "Origin")

		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-TOKEN")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
