package openapi_test

import (
	"testing"

	"github.com/golgimed/mimic/internal/openapi"
)

func TestXMimicBehaviorSeedsFaultAndFires(t *testing.T) {
	h := newApp(t, "fixtures/behavior", openapi.ConflictStrict)

	rec := do(t, h, "GET", "/behavior-spec/widgets")
	if rec.Code != 503 {
		t.Fatalf("expected 503 from x-mimic-behavior, got %d: %s", rec.Code, rec.Body.String())
	}
}
