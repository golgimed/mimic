// Package testutil builds a fresh, fully-wired Mimic app (in-memory DB,
// fresh registry) for integration tests, mirroring what tests/*.test.ts did
// with vi.resetModules() + buildServer() against DB_PATH=":memory:".
package testutil

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rluders/lane"

	"github.com/golgimed/mimic/internal/core"
	"github.com/golgimed/mimic/internal/providers"
	"github.com/golgimed/mimic/internal/registry"
	"github.com/golgimed/mimic/internal/shared/admin"
	"github.com/golgimed/mimic/internal/shared/faults"
	"github.com/golgimed/mimic/internal/shared/scheduler"
	"github.com/golgimed/mimic/internal/shared/storage"
)

type App struct {
	Handler http.Handler
	DB      *sql.DB
	Sched   *scheduler.Scheduler
}

// Tick drives one scheduler pass synchronously, equivalent to importing and
// calling scheduler.tick() directly in the TS tests.
func (a *App) Tick(t *testing.T) {
	t.Helper()
	if err := a.Sched.Tick(context.Background()); err != nil {
		t.Fatalf("scheduler tick failed: %v", err)
	}
}

// New builds a fresh app against an in-memory SQLite DB. zenviaStatusDelay
// defaults to 0 (advance jobs runnable immediately) unless overridden.
func New(t *testing.T, zenviaStatusDelay time.Duration) *App {
	t.Helper()

	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := storage.RunMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	faults.SetDefaultDelay(0)

	reg := registry.New()
	faultStore := admin.NewStore(db)
	sched := scheduler.New(db)
	// bry-scad completion webhook is unused by in-process Go tests today — the
	// real-Mimic BRy loop is exercised against a running container instead
	// (golgimed's *_mimic_e2e_test.go, gated on MIMIC_BASE_URL).
	providers.RegisterAll(reg, db, faultStore, sched, zenviaStatusDelay, "")

	health := &lane.HealthState{}
	health.SetReady(true)
	mux := core.NewMux(reg, db, faultStore, nil, health)

	return &App{Handler: core.WithCORS(mux), DB: db, Sched: sched}
}

// Do performs an in-process HTTP request against the app, JSON-encoding body
// when non-nil. Equivalent to Fastify's app.inject() in the original tests.
func (a *App) Do(t *testing.T, method, path string, headers map[string]string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	a.Handler.ServeHTTP(rec, req)
	return rec
}

// DecodeJSON decodes rec's body into v, failing the test on error.
func DecodeJSON(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("decode JSON response (body=%q): %v", rec.Body.String(), err)
	}
}
