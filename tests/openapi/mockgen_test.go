package openapi_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rluders/lane"

	"github.com/golgimed/mimic/internal/core"
	"github.com/golgimed/mimic/internal/openapi"
	"github.com/golgimed/mimic/internal/providers"
	"github.com/golgimed/mimic/internal/registry"
	"github.com/golgimed/mimic/internal/shared/admin"
	"github.com/golgimed/mimic/internal/shared/storage"
)

func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func newApp(t *testing.T, specDir string, conflict openapi.ConflictMode) http.Handler {
	t.Helper()
	h, _ := newAppPersist(t, specDir, conflict, false)
	return h
}

func newAppPersist(t *testing.T, specDir string, conflict openapi.ConflictMode, persist bool) (http.Handler, *sql.DB) {
	t.Helper()

	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := storage.RunMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	reg := registry.New()
	faultStore := admin.NewStore(db)
	if _, _, err := providers.RegisterOpenAPI(reg, db, faultStore, specDir, nil, conflict, persist, testLogger()); err != nil {
		t.Fatalf("register openapi: %v", err)
	}

	health := &lane.HealthState{}
	health.SetReady(true)
	return core.WithCORS(core.NewMux(reg, db, faultStore, nil, health)), db
}

func newRegistry(t *testing.T) (*registry.Registry, *sql.DB, *admin.Store) {
	t.Helper()

	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := storage.RunMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	return registry.New(), db, admin.NewStore(db)
}

func do(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func doBody(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	return rec
}

func TestServesExampleVerbatim(t *testing.T) {
	h := newApp(t, "testdata", openapi.ConflictStrict)

	rec := do(t, h, "GET", "/petstore-example/pets")
	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if want := `{"pets":[{"id":"1","name":"Rex"}]}`; rec.Body.String() != want+"\n" {
		t.Errorf("body = %q, want %q", rec.Body.String(), want)
	}
}

func TestGeneratesBodyFromSchema(t *testing.T) {
	h := newApp(t, "testdata", openapi.ConflictStrict)

	rec := do(t, h, "POST", "/petstore-example/pets")
	if rec.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body["id"].(string); !ok {
		t.Errorf("generated id = %v, want a string", body["id"])
	}
	if _, ok := body["name"].(string); !ok {
		t.Errorf("generated name = %v, want a string", body["name"])
	}
	if _, ok := body["age"].(float64); !ok {
		t.Errorf("generated age = %v, want a number", body["age"])
	}
}

func TestGeneratedBodyIsStableAcrossRequests(t *testing.T) {
	h := newApp(t, "testdata", openapi.ConflictStrict)

	first := do(t, h, "POST", "/petstore-example/pets").Body.String()
	second := do(t, h, "POST", "/petstore-example/pets").Body.String()
	if first != second {
		t.Errorf("static schema-stub body changed across requests: %q vs %q", first, second)
	}
}

func TestFallsBackToEmptyObject(t *testing.T) {
	h := newApp(t, "testdata", openapi.ConflictStrict)

	rec := do(t, h, "GET", "/petstore-example/pets/1")
	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "{}\n" {
		t.Errorf("body = %q, want {}", rec.Body.String())
	}
	if rec.Header().Get("X-Mimic-Warning") == "" {
		t.Error("expected X-Mimic-Warning header when no example/schema exists")
	}
}

func TestUnknownSpecDirErrors(t *testing.T) {
	reg, db, faultStore := newRegistry(t)
	if _, _, err := providers.RegisterOpenAPI(reg, db, faultStore, "does-not-exist", nil, openapi.ConflictStrict, false, testLogger()); err == nil {
		t.Fatal("expected error for missing spec dir")
	}
}

func TestDuplicatePrefixErrors(t *testing.T) {
	reg, db, faultStore := newRegistry(t)
	if _, _, err := providers.RegisterOpenAPI(reg, db, faultStore, "fixtures/collisions/duplicate", nil, openapi.ConflictStrict, false, testLogger()); err == nil {
		t.Fatal("expected error for duplicate spec prefixes")
	}
}

func TestReservedNamePrefixErrors(t *testing.T) {
	reg, db, faultStore := newRegistry(t)
	reg.Register(&registry.Provider{Name: "petstore-example", Register: func(*http.ServeMux) {}})

	if _, _, err := providers.RegisterOpenAPI(reg, db, faultStore, "testdata", nil, openapi.ConflictStrict, false, testLogger()); err == nil {
		t.Fatal("expected error for spec prefix colliding with an existing provider")
	}
}

func TestMimicNameOverridesPrefix(t *testing.T) {
	h := newApp(t, "fixtures/collisions/renamed", openapi.ConflictStrict)

	rec := do(t, h, "GET", "/renamed-pets/pets")
	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListItemsAndGetItemDetail(t *testing.T) {
	reg, db, faultStore := newRegistry(t)
	if _, _, err := providers.RegisterOpenAPI(reg, db, faultStore, "testdata", nil, openapi.ConflictStrict, false, testLogger()); err != nil {
		t.Fatalf("register openapi: %v", err)
	}

	p, ok := reg.Get("openapi")
	if !ok {
		t.Fatal("expected \"openapi\" provider to be registered")
	}

	items := p.ListItems()
	if len(items) != 1 {
		t.Fatalf("expected 1 dashboard item, got %d", len(items))
	}
	item := items[0]
	if item.ID != "petstore-example" || item.Type != "spec" || item.Status == "" {
		t.Errorf("unexpected dashboard item: %+v", item)
	}

	detail, ok := p.GetItemDetail("petstore-example")
	if !ok {
		t.Fatal("expected detail for petstore-example")
	}
	payload, ok := detail.Payload.(map[string]any)
	if !ok || payload["checksum"] == "" {
		t.Errorf("unexpected detail payload: %+v", detail.Payload)
	}
}

// --- Persistence ---

func TestPersistenceDisabledByDefaultKeepsStaticBehavior(t *testing.T) {
	h := newApp(t, "testdata", openapi.ConflictStrict)

	created := do(t, h, "POST", "/petstore-example/pets")
	if created.Code != 201 {
		t.Fatalf("expected 201, got %d", created.Code)
	}

	// GET /pets/{id} has no example/schema in this spec, so it always falls
	// back to {} regardless of what was just POSTed — nothing is persisted.
	rec := do(t, h, "GET", "/petstore-example/pets/whatever")
	if rec.Body.String() != "{}\n" {
		t.Errorf("expected static fallback body, got %q", rec.Body.String())
	}
}

func TestPersistedCreateThenReadRoundTrip(t *testing.T) {
	h, _ := newAppPersist(t, "fixtures/crud", openapi.ConflictStrict, true)

	created := doBody(t, h, "POST", "/crud-example/widgets", map[string]any{"name": "Sprocket"})
	if created.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", created.Code, created.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(created.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	id, ok := body["id"].(string)
	if !ok || id == "" {
		t.Fatalf("expected a generated id, got %v", body["id"])
	}
	if body["name"] != "Sprocket" {
		t.Errorf("expected client-supplied name to survive, got %v", body["name"])
	}

	read := do(t, h, "GET", "/crud-example/widgets/"+id)
	if read.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", read.Code, read.Body.String())
	}
	var readBody map[string]any
	if err := json.Unmarshal(read.Body.Bytes(), &readBody); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if readBody["id"] != id || readBody["name"] != "Sprocket" {
		t.Errorf("GET after Create = %v, want id=%q name=Sprocket", readBody, id)
	}
}

func TestPersistedListReflectsWrites(t *testing.T) {
	h, _ := newAppPersist(t, "fixtures/crud", openapi.ConflictStrict, true)

	if rec := do(t, h, "GET", "/crud-example/widgets"); rec.Body.String() != "[]\n" {
		t.Fatalf("expected empty list, got %q", rec.Body.String())
	}

	doBody(t, h, "POST", "/crud-example/widgets", map[string]any{"name": "A"})
	doBody(t, h, "POST", "/crud-example/widgets", map[string]any{"name": "B"})

	rec := do(t, h, "GET", "/crud-example/widgets")
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 widgets, got %d", len(list))
	}
}

func TestPersistedUpdateAndDelete(t *testing.T) {
	h, _ := newAppPersist(t, "fixtures/crud", openapi.ConflictStrict, true)

	created := doBody(t, h, "POST", "/crud-example/widgets", map[string]any{"name": "A"})
	var body map[string]any
	json.Unmarshal(created.Body.Bytes(), &body)
	id := body["id"].(string)

	updated := doBody(t, h, "PUT", "/crud-example/widgets/"+id, map[string]any{"name": "B"})
	if updated.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", updated.Code, updated.Body.String())
	}
	var updatedBody map[string]any
	json.Unmarshal(updated.Body.Bytes(), &updatedBody)
	if updatedBody["name"] != "B" {
		t.Errorf("expected updated name, got %v", updatedBody["name"])
	}

	deleted := do(t, h, "DELETE", "/crud-example/widgets/"+id)
	if deleted.Code != 204 {
		t.Fatalf("expected 204, got %d", deleted.Code)
	}

	missing := do(t, h, "GET", "/crud-example/widgets/"+id)
	if missing.Code != 404 {
		t.Fatalf("expected 404 after delete, got %d", missing.Code)
	}
}

func TestPersistedUpdateAndDeleteOfMissingResourceIs404(t *testing.T) {
	h, _ := newAppPersist(t, "fixtures/crud", openapi.ConflictStrict, true)

	if rec := doBody(t, h, "PUT", "/crud-example/widgets/nope", map[string]any{}); rec.Code != 404 {
		t.Errorf("expected 404, got %d", rec.Code)
	}
	if rec := do(t, h, "DELETE", "/crud-example/widgets/nope"); rec.Code != 404 {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestMimicPersistOverridesGlobalDefault(t *testing.T) {
	// Global default is false, but the spec sets x-mimic-persist: true.
	h, _ := newAppPersist(t, "fixtures/crud-persist-true", openapi.ConflictStrict, false)

	created := doBody(t, h, "POST", "/crud-persist-true/widgets", map[string]any{"name": "A"})
	var body map[string]any
	json.Unmarshal(created.Body.Bytes(), &body)
	id := body["id"].(string)

	rec := do(t, h, "GET", "/crud-persist-true/widgets/"+id)
	if rec.Code != 200 {
		t.Fatalf("expected persisted resource to be readable, got %d", rec.Code)
	}
}

func TestFlushClearsPersistedResources(t *testing.T) {
	h, db := newAppPersist(t, "fixtures/crud", openapi.ConflictStrict, true)

	doBody(t, h, "POST", "/crud-example/widgets", map[string]any{"name": "A"})

	if err := storage.Flush(db); err != nil {
		t.Fatalf("flush: %v", err)
	}

	rec := do(t, h, "GET", "/crud-example/widgets")
	if rec.Body.String() != "[]\n" {
		t.Errorf("expected empty list after flush, got %q", rec.Body.String())
	}
}
