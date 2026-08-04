package openapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/google/uuid"
)

const errNotFound = "not found"

// handler picks the http.HandlerFunc that serves this route: the static
// example/schema-stub handler when persistence is off (or the route isn't
// CRUD-shaped), otherwise a Store-backed CRUD handler for route.Op.
func (route Route) handler(store *Store) http.HandlerFunc {
	if !route.Persist {
		return route.Plan.handler()
	}

	switch route.Op {
	case OpList:
		return route.listHandler(store)
	case OpCreate:
		return route.createHandler(store)
	case OpRead:
		return route.readHandler(store)
	case OpUpdate:
		return route.updateHandler(store)
	case OpDelete:
		return route.deleteHandler(store)
	default:
		return route.Plan.handler()
	}
}

// listHandler always returns a raw JSON array of persisted resources,
// regardless of any response envelope declared in the spec (e.g.
// {"pets": [...]}) — matching an arbitrary declared envelope shape while
// also reflecting live state isn't worth the complexity here. Documented in
// specs/README.md.
func (route Route) listHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resources, err := store.List(route.SpecName, route.ResourceType)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		out := make([]map[string]any, 0, len(resources))
		for _, res := range resources {
			out = append(out, res.Payload)
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (route Route) createHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := decodeBody(r)
		if err != nil {
			http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
			return
		}
		id := resourceID(body, route.Plan.Schema)
		body["id"] = id
		fillMissingFields(body, route.Plan.Schema)

		res, err := store.Create(route.SpecName, route.ResourceType, id, body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		status := route.Plan.StatusCode
		if status == 0 {
			status = http.StatusCreated
		}
		writeJSON(w, status, res.Payload)
	}
}

func (route Route) readHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := store.Get(route.SpecName, route.ResourceType, r.PathValue(route.IDParam))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if res == nil {
			http.Error(w, errNotFound, http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, res.Payload)
	}
}

func (route Route) updateHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue(route.IDParam)

		existing, err := store.Get(route.SpecName, route.ResourceType, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if existing == nil {
			http.Error(w, errNotFound, http.StatusNotFound)
			return
		}

		body, err := decodeBody(r)
		if err != nil {
			http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
			return
		}
		body["id"] = id
		res, err := store.Update(route.SpecName, route.ResourceType, id, body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, res.Payload)
	}
}

func (route Route) deleteHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deleted, err := store.Delete(route.SpecName, route.ResourceType, r.PathValue(route.IDParam))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !deleted {
			http.Error(w, errNotFound, http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// decodeBody parses the request body as a JSON object. A missing/empty body
// is treated as {} (create/update handlers fill in any declared-but-absent
// fields), but malformed JSON is a client error.
func decodeBody(r *http.Request) (map[string]any, error) {
	body := map[string]any{}
	if r.Body == nil {
		return body, nil
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if body == nil {
		body = map[string]any{}
	}
	return body, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// resourceID picks the id for a newly created resource: whatever the client
// supplied under "id" in the request body, or a freshly generated one
// matching the response schema's declared type for "id" (string -> uuid,
// integer -> random int) — it must round-trip through the path template on
// later requests. Convention: the JSON field is always named "id",
// independent of whatever the spec calls the path parameter (e.g. "petId").
func resourceID(body map[string]any, respSchema *schema) string {
	if v, ok := body["id"]; ok {
		switch t := v.(type) {
		case string:
			if t != "" {
				return t
			}
		case float64:
			return strconv.FormatFloat(t, 'f', -1, 64)
		}
	}
	if respSchema != nil {
		if idSchema, ok := respSchema.Properties["id"]; ok && idSchema.Type == "integer" {
			return strconv.Itoa(gofakeit.Number(1, 1_000_000))
		}
	}
	return uuid.NewString()
}

// fillMissingFields fills any schema-declared property the client didn't
// supply in the request body with a realistic fake value (server-assigned
// fields like createdAt/status on Create). Deliberately not seeded — this
// is genuinely new data for a genuinely new resource, same as zenvia's own
// uuid.NewString() ids; see seededFaker's doc comment for why that's fine.
func fillMissingFields(body map[string]any, s *schema) {
	if s == nil || s.Type != "object" {
		return
	}
	for name, prop := range s.Properties {
		if _, exists := body[name]; exists {
			continue
		}
		body[name] = generateFromSchema(prop, gofakeit.GlobalFaker)
	}
}
