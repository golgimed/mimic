package openapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadSpecString(t *testing.T, spec string) (*LoadedSpec, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "spec.yaml")
	if err := os.WriteFile(path, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	return Load(path)
}

const refSpec = `
openapi: 3.0.0
info:
  title: Ref Spec
paths:
  /signatures/{id}:
    get:
      parameters:
        - $ref: '#/components/parameters/ID'
      responses:
        '200':
          $ref: '#/components/responses/SignatureOK'
  /signatures:
    post:
      requestBody:
        $ref: '#/components/requestBodies/CreateSignature'
      responses:
        '201':
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Signature'
  /nodes:
    get:
      responses:
        '200':
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Node'
components:
  parameters:
    ID:
      name: id
      in: path
      schema:
        $ref: '#/components/schemas/Signature/properties/id'
  requestBodies:
    CreateSignature:
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/Signature'
  responses:
    SignatureOK:
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/Signature'
  schemas:
    Signature:
      type: object
      properties:
        id:
          type: string
        signer:
          $ref: '#/components/schemas/Signer'
    Signer:
      type: object
      properties:
        name:
          type: string
    Node:
      type: object
      properties:
        value:
          type: string
        next:
          $ref: '#/components/schemas/Node'
        children:
          type: array
          items:
            $ref: '#/components/schemas/Node'
`

func TestLoadResolvesNestedRefs(t *testing.T) {
	loaded, err := loadSpecString(t, refSpec)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Warnings) != 0 {
		t.Fatalf("Warnings = %v, want none", loaded.Warnings)
	}

	get := loaded.doc.Paths["/signatures/{id}"]["get"]
	mt := get.Responses["200"].Content["application/json"]
	if mt.Schema == nil || mt.Schema.Properties["signer"] == nil {
		t.Fatalf("response schema not resolved: %+v", mt.Schema)
	}
	if got := mt.Schema.Properties["signer"].Properties["name"]; got == nil || got.Type != "string" {
		t.Fatalf("signer.name = %+v, want string schema", got)
	}
	if p := get.Parameters[0]; p.Name != "id" || p.Schema == nil || p.Schema.Type != "string" {
		t.Fatalf("parameter not resolved: %+v", p)
	}

	post := loaded.doc.Paths["/signatures"]["post"]
	if post.RequestBody == nil || post.RequestBody.Content["application/json"].Schema.Properties["id"] == nil {
		t.Fatalf("request body not resolved: %+v", post.RequestBody)
	}
	if loaded.doc.Components.Schemas["Signer"] == nil {
		t.Fatal("components.schemas.Signer missing from internal model")
	}
}

func TestResolvedSchemaDrivesResponseGeneration(t *testing.T) {
	loaded, err := loadSpecString(t, refSpec)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	routes := BuildRoutes(loaded, "sig", false)
	for _, r := range routes {
		if r.Method != "GET" || !strings.HasSuffix(r.Path, "/signatures/{id}") {
			continue
		}
		body, ok := r.Plan.Generated.(map[string]any)
		if !ok {
			t.Fatalf("Generated = %#v, want object", r.Plan.Generated)
		}
		signer, ok := body["signer"].(map[string]any)
		if !ok || signer["name"] == nil {
			t.Fatalf("signer = %#v, want object with name", body["signer"])
		}
		return
	}
	t.Fatal("GET /signatures/{id} route not found")
}

func TestRecursiveSchemaTerminates(t *testing.T) {
	loaded, err := loadSpecString(t, refSpec)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	node := loaded.doc.Components.Schemas["Node"]
	if node.Properties["next"] != node || node.Properties["children"].Items != node {
		t.Fatal("recursive refs should point back at the same schema")
	}

	routes := BuildRoutes(loaded, "sig", false)
	for _, r := range routes {
		if strings.HasSuffix(r.Path, "/nodes") {
			body := r.Plan.Generated.(map[string]any)
			if body["next"] != nil {
				t.Fatalf("next = %#v, want nil at the recursion point", body["next"])
			}
			if kids, _ := body["children"].([]any); len(kids) != 0 {
				t.Fatalf("children = %#v, want empty", body["children"])
			}
			return
		}
	}
	t.Fatal("GET /nodes route not found")
}

func TestLoadRefErrors(t *testing.T) {
	cases := map[string]struct {
		ref  string
		want string
	}{
		"unresolved": {"#/components/schemas/Missing", "unresolved reference"},
		"external":   {"other.yaml#/components/schemas/Foo", "only local references"},
		"remote":     {"https://example.com/spec.yaml#/Foo", "only local references"},
		"wrong kind": {"#/definitions/Foo", "expected #/components/schemas/"},
		"bad path":   {"#/components/schemas/Ok/properties/nope", "unresolved reference"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			spec := `
openapi: 3.0.0
info:
  title: Bad Ref
paths:
  /x:
    get:
      responses:
        '200':
          content:
            application/json:
              schema:
                $ref: '` + tc.ref + `'
components:
  schemas:
    Ok:
      type: object
`
			_, err := loadSpecString(t, spec)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Load error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestLoadRejectsCircularRefChain(t *testing.T) {
	spec := `
openapi: 3.0.0
info:
  title: Loop
paths: {}
components:
  schemas:
    A:
      $ref: '#/components/schemas/B'
    B:
      $ref: '#/components/schemas/A'
`
	_, err := loadSpecString(t, spec)
	if err == nil || !strings.Contains(err.Error(), "circular reference") {
		t.Fatalf("Load error = %v, want circular reference", err)
	}
}
