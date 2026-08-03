package core_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golgimed/mimic/internal/testutil"
)

func TestHealthEndpoint(t *testing.T) {
	app := testutil.New(t, 0)
	rec := app.Do(t, "GET", "/health", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestReadyEndpointPingsDB(t *testing.T) {
	app := testutil.New(t, 0)
	rec := app.Do(t, "GET", "/ready", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	app.DB.Close()
	rec = app.Do(t, "GET", "/ready", nil, nil)
	if rec.Code == http.StatusOK {
		t.Fatalf("expected non-200 once the DB is closed, got %d", rec.Code)
	}
}

func TestDashboardServesHTML(t *testing.T) {
	app := testutil.New(t, 0)
	rec := app.Do(t, "GET", "/dashboard", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html" {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

func TestCORSHeadersOnRegularRequest(t *testing.T) {
	app := testutil.New(t, 0)
	req := httptest.NewRequest("GET", "/health", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	app.Handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want https://example.com", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want true", got)
	}
}

func TestCORSPreflightOptions(t *testing.T) {
	app := testutil.New(t, 0)
	req := httptest.NewRequest("OPTIONS", "/health", nil)
	rec := httptest.NewRecorder()
	app.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("expected Access-Control-Allow-Methods to be set")
	}
}
