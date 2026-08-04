package openapi

import (
	"encoding/json"
	"hash/fnv"
	"net/http"
	"sort"
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
// CRUD resources don't need this: once created, the stored row is the
// source of truth, and new resources getting fresh random values (ids,
// timestamps) on each Create is correct REST behavior, not a determinism
// violation — it already matches how zenvia's uuid.NewString() works.
func seededFaker(key string) *gofakeit.Faker {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return gofakeit.New(h.Sum64())
}

// generateFromSchema builds a JSON instance from a JSON Schema fragment
// using f for values. It's intentionally simple: one representative value
// per type/format, no combinators (oneOf/allOf/anyOf). Object properties are
// visited in sorted order so f's call sequence — and therefore the
// generated values — stay stable regardless of Go's randomized map
// iteration order.
func generateFromSchema(s *schema, f *gofakeit.Faker) any {
	if s == nil {
		return nil
	}
	if len(s.Enum) > 0 {
		return s.Enum[0]
	}

	switch s.Type {
	case "object":
		names := make([]string, 0, len(s.Properties))
		for name := range s.Properties {
			names = append(names, name)
		}
		sort.Strings(names)

		out := make(map[string]any, len(s.Properties))
		for _, name := range names {
			out[name] = generateFromSchema(s.Properties[name], f)
		}
		return out
	case "array":
		if s.Items == nil {
			return []any{}
		}
		return []any{generateFromSchema(s.Items, f)}
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
