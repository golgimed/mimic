package openapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golgimed/mimic/internal/openapi"
)

// createdIDs creates n widgets in a fresh app inside a simulation seeded
// with seed and returns their server-assigned ids.
func createdIDs(t *testing.T, seed int64, n int) []string {
	t.Helper()
	h, _ := newAppPersist(t, "fixtures/crud-persist-true", openapi.ConflictStrict, false)

	rec := doBody(t, h, http.MethodPost, "/admin/simulations", map[string]any{"seed": seed})
	var s struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil || s.ID == "" {
		t.Fatalf("create simulation: %d %s", rec.Code, rec.Body)
	}

	ids := make([]string, n)
	for i := range ids {
		req := httptest.NewRequest(http.MethodPost, "/crud-persist-true/widgets", bytes.NewBufferString(`{"name":"w"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Mimic-Simulation", s.ID)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var body map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		ids[i], _ = body["id"].(string)
	}
	return ids
}

func TestSeededCreatesAreReproducible(t *testing.T) {
	a := createdIDs(t, 99, 3)
	b := createdIDs(t, 99, 3)
	for i := range a {
		if a[i] == "" || a[i] != b[i] {
			t.Fatalf("ids differ under the same seed: %v vs %v", a, b)
		}
	}
	if a[0] == a[1] {
		t.Fatalf("successive creates got the same id %q", a[0])
	}
}
