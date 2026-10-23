package admin

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/golgimed/mimic/internal/shared/sim"
)

type createSimulationWire struct {
	Seed *int64 `json:"seed"`
}

// handleCreateSimulation serves POST /admin/simulations {"seed": n}. The
// seed is optional; one is generated (and returned) when omitted.
func handleCreateSimulation(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	var wire createSimulationWire
	if err := json.NewDecoder(r.Body).Decode(&wire); err != nil && !errors.Is(err, io.EOF) {
		writeValidation(w, "body", "invalid JSON body")
		return
	}
	var seed int64
	if wire.Seed != nil {
		if !sim.ValidSeed(*wire.Seed) {
			writeValidation(w, "seed", "must be an integer between 1 and 9007199254740991")
			return
		}
		seed = *wire.Seed
	}
	s, err := sim.StoreFor(db).Create(r.Context(), seed, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, s)
}

// handleListSimulations serves GET /admin/simulations?implicit=true&limit=,
// newest first. Implicit (single-request) simulations are hidden unless
// implicit=true.
func handleListSimulations(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n <= 0 {
			writeValidation(w, "limit", "must be a positive integer")
			return
		}
		limit = n
	}
	list, err := sim.StoreFor(db).List(r.Context(), r.URL.Query().Get("implicit") == "true", limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"content": list})
}

func handleGetSimulation(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	s, err := sim.StoreFor(db).Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, sim.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Simulation not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func writeValidation(w http.ResponseWriter, path, message string) {
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error": map[string]any{"code": "VALIDATION_ERROR", "issues": []validationIssue{{Path: path, Message: message}}},
	})
}
