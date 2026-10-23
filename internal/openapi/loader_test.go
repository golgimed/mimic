package openapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSpec(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "spec.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	return path
}

func TestLoadResolvesComponentRefs(t *testing.T) {
	path := writeSpec(t, `
openapi: 3.0.3
info: {title: Ref Spec, version: "1"}
paths:
  /pets/{id}:
    parameters:
      - $ref: '#/components/parameters/PetID'
    get:
      responses:
        '200':
          $ref: '#/components/responses/PetResponse'
    put:
      requestBody:
        $ref: '#/components/requestBodies/PetBody'
      responses:
        '204': {description: ok}
components:
  parameters:
    PetID: {name: id, in: path, required: true, schema: {type: string}}
  requestBodies:
    PetBody:
      required: true
      content:
        application/json:
          schema: {$ref: '#/components/schemas/Pet'}
  responses:
    PetResponse:
      description: a pet
      content:
        application/json:
          schema: {$ref: '#/components/schemas/Pet'}
  schemas:
    Pet:
      type: object
      properties:
        name: {type: string}
        owner: {$ref: '#/components/schemas/Owner'}
    Owner:
      type: object
      properties:
        email: {type: string, format: email}
`)
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Warnings) != 0 {
		t.Fatalf("Warnings = %v, want none", loaded.Warnings)
	}

	get := loaded.doc.Paths["/pets/{id}"]["get"]
	s := get.Responses["200"].Content["application/json"].Schema
	if s == nil || s.Type != "object" || s.Properties["owner"] == nil ||
		s.Properties["owner"].Properties["email"].Format != "email" {
		t.Fatalf("response schema not resolved through nested $ref: %+v", s)
	}
	if len(get.Parameters) != 1 || get.Parameters[0].Name != "id" || !get.Parameters[0].Required {
		t.Fatalf("path-item $ref parameter not inherited: %+v", get.Parameters)
	}

	put := loaded.doc.Paths["/pets/{id}"]["put"]
	if put.RequestBody == nil || !put.RequestBody.Required ||
		put.RequestBody.Content["application/json"].Schema.Properties["name"] == nil {
		t.Fatalf("request body $ref not resolved: %+v", put.RequestBody)
	}
}

func TestLoadUnresolvableRefFails(t *testing.T) {
	path := writeSpec(t, `
openapi: 3.0.0
info: {title: Broken, version: "1"}
paths:
  /pets:
    get:
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Missing'}
`)
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("Load error = %v, want an error naming %s", err, path)
	}
}

func TestLoadKeepsSchemaViolationsAsWarnings(t *testing.T) {
	// Missing response description is invalid OpenAPI, but still usable.
	path := writeSpec(t, `
openapi: 3.0.0
info: {title: Sloppy, version: "1"}
paths:
  /pets:
    get:
      responses:
        '200':
          content:
            application/json:
              schema: {type: object, properties: {name: {type: string}}}
`)
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want 1", loaded.Warnings)
	}
	if loaded.doc.Paths["/pets"]["get"].Responses["200"].Content["application/json"].Schema == nil {
		t.Fatal("schema dropped despite warning")
	}
}

func TestLoadReadsMimicExtensions(t *testing.T) {
	path := writeSpec(t, `
openapi: 3.0.0
info: {title: Ext, version: "1"}
x-mimic-name: custom
x-mimic-persist: true
paths:
  /pets:
    get:
      x-mimic-behavior: {faultKind: http_status, faultValue: "503", probability: 0.5}
      responses:
        '200': {description: ok}
`)
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.MimicName != "custom" || loaded.Persist == nil || !*loaded.Persist {
		t.Fatalf("extensions not read: name=%q persist=%v", loaded.MimicName, loaded.Persist)
	}
	b := loaded.doc.Paths["/pets"]["get"].Behavior
	if b == nil || b.FaultKind != "http_status" || *b.FaultValue != "503" || *b.Probability != 0.5 {
		t.Fatalf("x-mimic-behavior not decoded: %+v", b)
	}
}

func TestRecursiveSchemaTerminates(t *testing.T) {
	path := writeSpec(t, `
openapi: 3.0.3
info: {title: Tree, version: "1"}
paths:
  /nodes:
    get:
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Node'}
components:
  schemas:
    Node:
      type: object
      properties:
        name: {type: string}
        parent: {$ref: '#/components/schemas/Node'}
        children:
          type: array
          items: {$ref: '#/components/schemas/Node'}
`)
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	s := loaded.doc.Paths["/nodes"]["get"].Responses["200"].Content["application/json"].Schema
	got, ok := generateFromSchema(s, seededFaker("k")).(map[string]any)
	if !ok || got["name"] == nil {
		t.Fatalf("generated = %#v, want object with name", got)
	}
	if got["parent"] != nil {
		t.Fatalf("parent = %#v, want nil at first recursion", got["parent"])
	}
	if children, _ := got["children"].([]any); len(children) != 0 {
		t.Fatalf("children = %#v, want empty array at first recursion", got["children"])
	}
}
