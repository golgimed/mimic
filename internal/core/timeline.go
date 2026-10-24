package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golgimed/mimic/internal/shared/correlation"
	"github.com/golgimed/mimic/internal/shared/sim"
	"github.com/golgimed/mimic/internal/shared/timeline"
)

// resolveSimulation returns the simulation named by the X-Mimic-Simulation
// header, or creates an implicit one for this request.
func resolveSimulation(r *http.Request, sims *sim.Store) (*sim.Simulation, error) {
	if id := r.Header.Get(sim.HeaderID); id != "" {
		s, err := sims.Get(r.Context(), id)
		if err != nil {
			return nil, fmt.Errorf("simulation %q: %w", id, err)
		}
		return s, nil
	}
	return sims.Create(r.Context(), 0, "", true)
}

// withTimeline sets up each simulated request's context and records the
// request and its response on the simulation timeline:
//   - a correlation ID (the client's X-Request-ID if usable, else a fresh
//     UUID), echoed as X-Mimic-Request-ID;
//   - the simulation it runs in: the one named by X-Mimic-Simulation, or a
//     fresh implicit one, reported as X-Mimic-Simulation-ID / X-Mimic-Seed;
//   - the timeline recorder.
func withTimeline(rec *timeline.Recorder, sims *sim.Store, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := correlation.FromClient(r.Header.Get(correlation.ClientHeader))
		w.Header().Set(correlation.Header, requestID)

		simulation, err := resolveSimulation(r, sims)
		if err != nil {
			status, code := http.StatusInternalServerError, "INTERNAL_ERROR"
			if errors.Is(err, sim.ErrNotFound) {
				status, code = http.StatusBadRequest, "UNKNOWN_SIMULATION"
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": err.Error()}})
			return
		}
		w.Header().Set(sim.ResponseHeaderID, simulation.ID)
		w.Header().Set(sim.HeaderSeed, strconv.FormatInt(simulation.Seed, 10))

		ctx := correlation.WithRequestID(timeline.WithRecorder(r.Context(), rec), requestID)
		ctx = sim.WithSimulation(ctx, simulation)
		r = r.WithContext(ctx)
		provider, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
		start := time.Now()

		timeline.Record(ctx, timeline.Event{
			Type:     timeline.RequestReceived,
			Provider: provider,
			Data:     map[string]any{"method": r.Method, "path": r.URL.Path},
		})
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		timeline.Record(ctx, timeline.Event{
			Type:     timeline.ResponseReturned,
			Provider: provider,
			Data: map[string]any{
				"method":     r.Method,
				"path":       r.URL.Path,
				"status":     sw.status,
				"durationMs": time.Since(start).Milliseconds(),
			},
		})
	})
}

// statusWriter captures the response status code.
type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.status, w.wroteHeader = code, true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	w.wroteHeader = true
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
