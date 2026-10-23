package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/golgimed/mimic/internal/shared/clock"
)

func clockView() map[string]any {
	c := clock.Get()
	view := map[string]any{"mode": c.Mode(), "now": c.Now().Format(time.RFC3339Nano)}
	if c.Mode() == clock.Accelerated {
		view["speed"] = c.Speed()
	}
	return view
}

// handleGetClock serves GET /admin/clock.
func handleGetClock(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, clockView())
}

// handleAdvanceClock serves POST /admin/clock/advance {"duration":"5m"}:
// moves simulation time forward, running every scheduled job that falls
// due on the way (in due-time order), before responding.
func handleAdvanceClock(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Duration string `json:"duration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeValidation(w, "body", "invalid JSON body")
		return
	}
	d, err := time.ParseDuration(body.Duration)
	if err != nil || d < 0 {
		writeValidation(w, "duration", `must be a non-negative Go duration such as "90s", "5m" or "24h"`)
		return
	}
	if _, err := clock.Advance(r.Context(), d); err != nil {
		if errors.Is(err, clock.ErrRealtime) {
			writeError(w, http.StatusConflict, "REALTIME_CLOCK", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, clockView())
}
