package openapi

import (
	"encoding/json"
	"hash/fnv"
	"net/http"
	"time"

	"github.com/brianvoe/gofakeit/v7"
)

// handler() builds the http.HandlerFunc for a ResponsePlan. Body selection is
// a plain 3-branch fallback, not a pluggable strategy: an OpenAPI spec can
// only ever give us an example, a schema, or nothing.
func (p ResponsePlan) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		contentType := p.ContentType
		if contentType == "" {
			contentType = "application/json"
		}
		w.Header().Set("Content-Type", contentType)

		switch {
		case p.Example != nil:
			w.WriteHeader(p.StatusCode)
			_ = json.NewEncoder(w).Encode(p.Example)

		case p.Schema != nil:
			w.WriteHeader(p.StatusCode)
			_ = json.NewEncoder(w).Encode(p.Generated)

		default:
			w.Header().Set("X-Mimic-Warning", "no example or schema found in spec for this response")
			w.WriteHeader(p.StatusCode)
			_ = json.NewEncoder(w).Encode(map[string]any{})
		}
	}
}

// seededFaker returns a gofakeit.Faker deterministically seeded from key.
// Used for static (non-persisted) schema-stub bodies: the same route must
// keep returning the same generated body across repeated requests and
// across process restarts, since nothing anchors it to real state — that's
// what CLAUDE.md's "deterministic behavior" rule requires here. Persisted
// CRUD creates instead draw from the request's simulation seed
// (sim.Faker), so each new resource gets fresh values that are still
// reproducible for a given seed.
func seededFaker(key string) *gofakeit.Faker {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return gofakeit.New(h.Sum64())
}

// generateFromSchema builds a JSON instance from a JSON Schema fragment
// using f for values. It's intentionally simple: one representative value
// per type/format. allOf is already merged into s at load time; for
// oneOf/anyOf the first alternative is used. Object properties are
// visited in sorted order so f's call sequence — and therefore the
// generated values — stay stable regardless of Go's randomized map
// iteration order.
func generateFromSchema(s *schema, f *gofakeit.Faker) any {
	return generate(s, f, map[*schema]bool{})
}

// generate is generateFromSchema with the set of schemas on the current
// descent path: a recursive schema (resolved from a self-referencing $ref)
// stops at its first repetition, yielding null there instead of looping.
func generate(s *schema, f *gofakeit.Faker, onPath map[*schema]bool) any {
	if s == nil || onPath[s] {
		return nil
	}
	onPath[s] = true
	defer delete(onPath, s)
	if len(s.Enum) > 0 {
		return s.Enum[0]
	}
	if len(s.Variants) > 0 && s.Type == "" {
		return generate(s.Variants[0], f, onPath)
	}

	switch s.Type {
	case "object":
		return generateObject(s, f, onPath)
	case "array":
		return generateArray(s, f, onPath)
	default:
		return generateScalar(s, f)
	}
}

// generateObject builds an object from s's properties (sorted, for a
// stable faker call order), plus the first oneOf/anyOf alternative's own
// properties when s has both.
func generateObject(s *schema, f *gofakeit.Faker, onPath map[*schema]bool) map[string]any {
	out := make(map[string]any, len(s.Properties))
	for _, name := range sortedKeys(s.Properties) {
		out[name] = generate(s.Properties[name], f, onPath)
	}
	if len(s.Variants) == 0 {
		return out
	}
	extra, _ := generate(s.Variants[0], f, onPath).(map[string]any)
	for k, v := range extra {
		if _, exists := out[k]; !exists {
			out[k] = v
		}
	}
	return out
}

// generateArray builds a one-item array, or an empty one when the item
// schema is missing or recursive.
func generateArray(s *schema, f *gofakeit.Faker, onPath map[*schema]bool) []any {
	if s.Items == nil {
		return []any{}
	}
	item := generate(s.Items, f, onPath)
	if item == nil {
		return []any{}
	}
	return []any{item}
}

func generateScalar(s *schema, f *gofakeit.Faker) any {
	switch s.Type {
	case "string":
		return fakeString(s.Format, f)
	case "integer":
		return f.Number(0, 1000)
	case "number":
		return f.Float64Range(0, 1000)
	case "boolean":
		return f.Bool()
	default:
		return nil
	}
}

// fakeString generates a value for a JSON Schema string, using the declared
// format when recognized and falling back to a generic word otherwise.
func fakeString(format string, f *gofakeit.Faker) string {
	switch format {
	case "email":
		return f.Email()
	case "date-time":
		return f.Date().Format(time.RFC3339)
	case "date":
		return f.Date().Format("2006-01-02")
	case "uuid":
		return f.UUID()
	case "uri", "url":
		return f.URL()
	case "hostname":
		return f.DomainName()
	case "ipv4":
		return f.IPv4Address()
	default:
		return f.Word()
	}
}
