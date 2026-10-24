package admin

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/golgimed/mimic/internal/shared/scenario"
	"github.com/golgimed/mimic/internal/shared/sim"
)

type createSimulationWire struct {
	Seed     *int64 `json:"seed"`
	Scenario string `json:"scenario"`
}

// handleCreateSimulation serves POST /admin/simulations {"seed": n,
// "scenario": name}. Both are optional; a seed is generated (and returned)
// when omitted. A scenario's faults are created scoped to the new
// simulation.
func handleCreateSimulation(w http.ResponseWriter, r *http.Request, db *sql.DB, faults *Store) {
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
	var sc *scenario.Scenario
	var faultInputs []CreateFaultInput
	if wire.Scenario != "" {
		found, ok := scenario.Lookup(wire.Scenario)
		if !ok {
			writeValidation(w, "scenario", "unknown scenario; see GET /admin/scenarios")
			return
		}
		sc = &found
		for i, f := range sc.Faults {
			input, issues := createFaultWire{
				Provider: sc.Provider, RoutePattern: f.Route, FaultKind: f.Kind, FaultValue: f.Value,
				Times: f.Times, Probability: f.Probability,
			}.toInput()
			if len(issues) > 0 {
				writeError(w, http.StatusInternalServerError, "INVALID_SCENARIO",
					fmt.Sprintf("scenario %q fault #%d: %s: %s", sc.Name, i+1, issues[0].Path, issues[0].Message))
				return
			}
			faultInputs = append(faultInputs, input)
		}
	}

	s, err := sim.StoreFor(db).Create(r.Context(), seed, wire.Scenario, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	for _, input := range faultInputs {
		input.SimulationID = s.ID
		if _, err := faults.CreateFault(input); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
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

// handleListScenarios serves GET /admin/scenarios: every scenario the
// enabled providers define.
func handleListScenarios(w http.ResponseWriter, _ *http.Request) {
	list := scenario.All()
	out := make([]map[string]any, 0, len(list))
	for _, s := range list {
		out = append(out, map[string]any{
			"name": s.Name, "provider": s.Provider, "resource": s.Resource, "description": s.Description,
			"lifecycle": s.Lifecycle(), "transitions": s.Steps, "webhooks": s.Webhooks,
			"faults": s.Faults, "requires": s.Requires,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"content": out})
}

func writeValidation(w http.ResponseWriter, path, message string) {
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error": map[string]any{"code": "VALIDATION_ERROR", "issues": []validationIssue{{Path: path, Message: message}}},
	})
}
