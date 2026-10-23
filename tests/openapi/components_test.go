package openapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/golgimed/mimic/internal/openapi"
)

// A spec built from reusable components serves bodies with the effective
// (fully $ref-resolved) structure.
func TestComponentRefsDriveGeneratedBodies(t *testing.T) {
	h := newApp(t, "fixtures/components", openapi.ConflictStrict)

	rec := do(t, h, http.MethodGet, "/signatures/signatures/abc")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var sig struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Signer struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"signer"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &sig); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if sig.ID == "" || sig.Status != "PENDING" || sig.Signer.Name == "" || !strings.Contains(sig.Signer.Email, "@") {
		t.Fatalf("body missing resolved fields: %s", rec.Body)
	}

	rec = do(t, h, http.MethodGet, "/signatures/signers")
	var signers []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &signers); err != nil || len(signers) != 1 || signers[0]["email"] == nil {
		t.Fatalf("array of $ref items not resolved: %s (err %v)", rec.Body, err)
	}
}
