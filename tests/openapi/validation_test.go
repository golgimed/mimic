package openapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rluders/lane"

	"github.com/golgimed/mimic/internal/core"
	"github.com/golgimed/mimic/internal/openapi"
	"github.com/golgimed/mimic/internal/providers"
)

func newValidatingApp(t *testing.T, mode openapi.ValidationMode) http.Handler {
	t.Helper()
	reg, db, faultStore := newRegistry(t)
	opts := providers.OpenAPIOptions{SpecDir: "fixtures/validation", ConflictMode: openapi.ConflictStrict, ValidationDefault: mode}
	if _, _, err := providers.RegisterOpenAPI(reg, db, faultStore, opts, testLogger()); err != nil {
		t.Fatalf("register openapi: %v", err)
	}
	health := &lane.HealthState{}
	health.SetReady(true)
	return core.NewMux(reg, db, faultStore, nil, health)
}

type issue struct {
	In     string `json:"in"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

func send(t *testing.T, h http.Handler, method, path, body string, header map[string]string) (*httptest.ResponseRecorder, []issue) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out struct {
		Error struct {
			Code   string  `json:"code"`
			Issues []issue `json:"issues"`
		} `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out.Error.Issues
}

func hasIssue(issues []issue, in, name string) bool {
	for _, i := range issues {
		if i.In == in && i.Name == name {
			return true
		}
	}
	return false
}

const validBody = `{"document":"doc","signer":{"email":"a@b.c"},"priority":"high"}`

var tenant = map[string]string{"X-Tenant": "t1"}

func TestStrictValidationAcceptsValidRequest(t *testing.T) {
	h := newValidatingApp(t, openapi.ValidationStrict)
	rec, _ := send(t, h, http.MethodPost, "/validated/signatures?mode=async", validBody, tenant)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
}

func TestStrictValidationRejectsContractViolations(t *testing.T) {
	h := newValidatingApp(t, openapi.ValidationStrict)

	cases := []struct {
		name, path, body string
		header           map[string]string
		in, field        string
	}{
		{"missing header", "/validated/signatures?mode=async", validBody, nil, "header", "X-Tenant"},
		{"missing query", "/validated/signatures", validBody, tenant, "query", "mode"},
		{"bad query enum", "/validated/signatures?mode=later", validBody, tenant, "query", "mode"},
		{"missing body", "/validated/signatures?mode=async", "", tenant, "body", ""},
		{"missing required field", "/validated/signatures?mode=async", `{"signer":{"email":"a@b.c"}}`, tenant, "body", "document"},
		{"nested required field", "/validated/signatures?mode=async", `{"document":"d","signer":{}}`, tenant, "body", "signer.email"},
		{"wrong type", "/validated/signatures?mode=async", `{"document":1,"signer":{"email":"a"}}`, tenant, "body", "document"},
		{"bad body enum", "/validated/signatures?mode=async", `{"document":"d","signer":{"email":"a"},"priority":"urgent"}`, tenant, "body", "priority"},
		{"bad JSON", "/validated/signatures?mode=async", `{`, tenant, "body", ""},
		{"bad path param type", "/validated/signatures/abc", "", nil, "path", "id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			method := http.MethodPost
			if tc.in == "path" {
				method = http.MethodGet
			}
			rec, issues := send(t, h, method, tc.path, tc.body, tc.header)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body)
			}
			if !hasIssue(issues, tc.in, tc.field) {
				t.Fatalf("issues = %+v, want one at %s %q", issues, tc.in, tc.field)
			}
		})
	}
}

func TestPermissiveValidationServesWithWarning(t *testing.T) {
	h := newValidatingApp(t, openapi.ValidationPermissive)

	rec, _ := send(t, h, http.MethodPost, "/validated/signatures", `{}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 in permissive mode", rec.Code)
	}
	if rec.Header().Get("X-Mimic-Validation-Warning") == "" {
		t.Fatal("missing X-Mimic-Validation-Warning header")
	}

	rec, _ = send(t, h, http.MethodPost, "/validated/signatures?mode=sync", validBody, tenant)
	if rec.Header().Get("X-Mimic-Validation-Warning") != "" {
		t.Fatalf("valid request got warning %q", rec.Header().Get("X-Mimic-Validation-Warning"))
	}
}

func TestSpecLevelValidationOverride(t *testing.T) {
	h := newApp(t, "fixtures/validation-strict", openapi.ConflictStrict)
	rec, issues := send(t, h, http.MethodPost, "/strict-spec/items", `{}`, nil)
	if rec.Code != http.StatusBadRequest || !hasIssue(issues, "body", "name") {
		t.Fatalf("status = %d issues = %+v, want x-mimic-validation: strict to reject", rec.Code, issues)
	}
}
