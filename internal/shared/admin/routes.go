package admin

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/golgimed/mimic/internal/registry"
	"github.com/golgimed/mimic/internal/shared/storage"
)

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

// RegisterRoutes mounts the /admin/* control-plane endpoints on mux.
// enabledProviders filters which registered providers are aggregated (nil/empty means all).
func RegisterRoutes(mux *http.ServeMux, db *sql.DB, store *Store, reg *registry.Registry, enabledProviders []string) {
	mux.HandleFunc("GET /admin/items", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"content": ListItems(reg, enabledProviders)})
	})

	mux.HandleFunc("GET /admin/items/{provider}/{id}", func(w http.ResponseWriter, r *http.Request) {
		detail, ok := GetItemDetail(reg, r.PathValue("provider"), r.PathValue("id"))
		if !ok {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Item not found")
			return
		}
		writeJSON(w, http.StatusOK, detail)
	})

	mux.HandleFunc("PUT /admin/faults", func(w http.ResponseWriter, r *http.Request) {
		var wire createFaultWire
		if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": map[string]any{"code": "VALIDATION_ERROR", "issues": []validationIssue{{Path: "body", Message: "invalid JSON body"}}},
			})
			return
		}
		input, issues := wire.toInput()
		if len(issues) > 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"code": "VALIDATION_ERROR", "issues": issues}})
			return
		}
		fault, err := store.CreateFault(input)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, fault)
	})

	mux.HandleFunc("GET /admin/faults", func(w http.ResponseWriter, r *http.Request) {
		list, err := store.ListFaults()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"content": list})
	})

	mux.HandleFunc("DELETE /admin/faults/{id}", func(w http.ResponseWriter, r *http.Request) {
		deleted, err := store.DeleteFault(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		if !deleted {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Fault not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /admin/flush", func(w http.ResponseWriter, r *http.Request) {
		if err := storage.Flush(db); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
