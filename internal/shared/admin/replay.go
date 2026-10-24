package admin

import (
	"bytes"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/golgimed/mimic/internal/shared/clock"
	"github.com/golgimed/mimic/internal/shared/sim"
	"github.com/golgimed/mimic/internal/shared/timeline"
)

type replayedRequest struct {
	Seq            int    `json:"seq"`
	Method         string `json:"method"`
	Path           string `json:"path"`
	OffsetMS       int64  `json:"offsetMs"`
	OriginalStatus int    `json:"originalStatus"`
	ReplayStatus   int    `json:"replayStatus"`
	Match          bool   `json:"match"`
}

// handleReplaySimulation serves POST /admin/simulations/{id}/replay: it
// creates a new simulation with the original's seed, scenario and profile
// (replayedFrom set) and re-sends the original's recorded requests, in
// order, through the simulator itself. The original is left untouched.
//
// Timing: on a virtual or accelerated clock the clock is advanced to each
// request's original simulated offset first, so scheduled work between
// requests runs as it did. On a realtime clock requests are re-sent back to
// back; the response says so, since time-dependent behavior (delays,
// expiry, rate-limit windows) may then differ.
func handleReplaySimulation(w http.ResponseWriter, r *http.Request, db *sql.DB, faults *Store, root http.Handler) {
	sims := sim.StoreFor(db)
	original, err := sims.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, sim.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Simulation not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	recorded, err := sims.Requests(r.Context(), original.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	replay, err := sims.Create(r.Context(), sim.Params{
		Seed: original.Seed, Scenario: original.Scenario, Profile: original.Profile, ReplayedFrom: original.ID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	// Scenario faults are per simulation: create them afresh, as defined,
	// for the replay.
	if err := createScenarioFaults(faults, replay); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	// Admin requests are off the timeline, so record with an explicit
	// recorder in the replay's simulation.
	timeline.Record(timeline.WithRecorder(sim.WithSimulation(r.Context(), replay), timeline.NewRecorder(db)), timeline.Event{
		Type: timeline.ReplayStarted, Data: map[string]any{"replayedFrom": original.ID, "requests": len(recorded)},
	})

	timed := clock.Get().Mode() != clock.Realtime
	start := clock.Now()
	results := make([]replayedRequest, 0, len(recorded))
	matched := 0
	for _, rr := range recorded {
		if timed {
			if wait := start.Add(time.Duration(rr.OffsetMS) * time.Millisecond).Sub(clock.Now()); wait > 0 {
				if _, err := clock.Advance(r.Context(), wait); err != nil {
					writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
					return
				}
			}
		}
		req := httptest.NewRequestWithContext(r.Context(), rr.Method, rr.Path, bytes.NewReader(rr.Body))
		req.Header = rr.Header.Clone()
		req.Header.Set(sim.HeaderID, replay.ID)
		rec := httptest.NewRecorder()
		root.ServeHTTP(rec, req)

		res := replayedRequest{
			Seq: rr.Seq, Method: rr.Method, Path: rr.Path, OffsetMS: rr.OffsetMS,
			OriginalStatus: rr.Status, ReplayStatus: rec.Code, Match: rr.Status == rec.Code,
		}
		if res.Match {
			matched++
		}
		results = append(results, res)
	}

	timing := "replayed at the original simulated offsets (" + string(clock.Get().Mode()) + " clock)"
	if !timed {
		timing = "realtime clock: requests re-sent back to back; time-dependent behavior (delays, expiry, rate-limit windows) may differ from the original"
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"simulation":   replay,
		"replayedFrom": original.ID,
		"timing":       timing,
		"requests":     results,
		"matched":      matched,
		"total":        len(results),
	})
}
