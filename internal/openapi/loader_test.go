package openapi

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWarnsOnUnsupportedRef(t *testing.T) {
	spec := `
openapi: 3.0.0
info:
  title: Ref Spec
paths:
  /pets:
    get:
      responses:
        '200':
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Pet'
`
	path := filepath.Join(t.TempDir(), "spec.yaml")
	if err := os.WriteFile(path, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want 1 entry mentioning $ref", loaded.Warnings)
	}
}

func TestLoadNoWarningsWithoutUnsupportedConstructs(t *testing.T) {
	spec := `
openapi: 3.0.0
info:
  title: Plain Spec
paths:
  /pets:
    get:
      responses:
        '200':
          content:
            application/json:
              schema:
                type: object
                properties:
                  name:
                    type: string
`
	path := filepath.Join(t.TempDir(), "spec.yaml")
	if err := os.WriteFile(path, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Warnings) != 0 {
		t.Fatalf("Warnings = %v, want none", loaded.Warnings)
	}
}
