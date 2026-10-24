package simulation_test

import (
	"strings"
	"testing"
	"time"

	"github.com/golgimed/mimic/internal/shared/clock"
	"github.com/golgimed/mimic/internal/testutil"
)

type replayResult struct {
	Simulation struct {
		ID           string `json:"id"`
		Seed         int64  `json:"seed"`
		Profile      string `json:"profile"`
		ReplayedFrom string `json:"replayedFrom"`
	} `json:"simulation"`
	Timing   string `json:"timing"`
	Matched  int    `json:"matched"`
	Total    int    `json:"total"`
	Requests []struct {
		OriginalStatus int `json:"originalStatus"`
		ReplayStatus   int `json:"replayStatus"`
	} `json:"requests"`
}

func replay(t *testing.T, app *testutil.App, id string) replayResult {
	t.Helper()
	rec := app.Do(t, "POST", "/admin/simulations/"+id+"/replay", nil, nil)
	if rec.Code != 201 {
		t.Fatalf("replay: %d %s", rec.Code, rec.Body.String())
	}
	var res replayResult
	testutil.DecodeJSON(t, rec, &res)
	return res
}

func useVirtualClock(t *testing.T) {
	t.Helper()
	c, _ := clock.New(clock.Virtual, 1)
	clock.Set(c)
	t.Cleanup(func() { rt, _ := clock.New(clock.Realtime, 1); clock.Set(rt) })
}

func TestReplayReproducesDeterministicDecisions(t *testing.T) {
	app := testutil.New(t, 0)
	useVirtualClock(t)
	id, seed := createSimulation(t, app, map[string]any{"profile": "chaos", "seed": 11})
	headers := map[string]string{"X-API-TOKEN": "test-token", "X-Mimic-Simulation": id}

	var original []int
	for range 40 {
		original = append(original, app.Do(t, "GET", "/zenvia/subscriptions", headers, nil).Code)
	}
	app.Do(t, "POST", "/zenvia/channels/sms/messages", headers, map[string]any{
		"from": "acct", "to": "5511999999999", "contents": []map[string]any{{"type": "text", "text": "hi"}},
	})

	res := replay(t, app, id)
	if res.Simulation.ID == id || res.Simulation.ReplayedFrom != id || res.Simulation.Seed != seed || res.Simulation.Profile != "chaos" {
		t.Fatalf("replay simulation = %+v; want a new simulation with the original's seed and profile", res.Simulation)
	}
	if res.Total != 41 || res.Matched != res.Total {
		t.Fatalf("replay matched %d/%d requests: %+v", res.Matched, res.Total, res.Requests)
	}
	failures := 0
	for i, code := range original {
		if code == 503 {
			failures++
		}
		if res.Requests[i].OriginalStatus != code {
			t.Fatalf("request %d recorded status %d, want %d", i, res.Requests[i].OriginalStatus, code)
		}
	}
	if failures == 0 {
		t.Fatal("chaos produced no failures to reproduce; pick another seed")
	}

	// The original is unchanged: replaying it again matches again.
	if again := replay(t, app, id); again.Total != 41 || again.Matched != 41 {
		t.Fatalf("second replay matched %d/%d", again.Matched, again.Total)
	}
}

func TestReplayRunsScheduledWorkAtOriginalOffsets(t *testing.T) {
	app := testutil.New(t, 10*time.Minute)
	useVirtualClock(t)
	id, _ := createSimulation(t, app, map[string]any{"seed": 3})
	headers := map[string]string{"X-API-TOKEN": "test-token", "X-Mimic-Simulation": id}

	app.Do(t, "POST", "/zenvia/channels/sms/messages", headers, map[string]any{
		"from": "acct", "to": "5511999999999", "contents": []map[string]any{{"type": "text", "text": "hi"}},
	})
	app.Do(t, "POST", "/admin/clock/advance", nil, map[string]any{"duration": "25m"})
	app.Do(t, "GET", "/zenvia/subscriptions", headers, nil)

	res := replay(t, app, id)
	if !strings.Contains(res.Timing, "original simulated offsets") {
		t.Fatalf("timing = %q", res.Timing)
	}

	var tl struct {
		Content []struct{ Type string } `json:"content"`
	}
	testutil.DecodeJSON(t, app.Do(t, "GET", "/admin/timeline?simulation_id="+res.Simulation.ID, nil, nil), &tl)
	var types []string
	for _, e := range tl.Content {
		if e.Type == "StateChanged" || e.Type == "RequestReceived" || e.Type == "ReplayStarted" {
			types = append(types, e.Type)
		}
	}
	want := "ReplayStarted RequestReceived StateChanged StateChanged RequestReceived"
	if strings.Join(types, " ") != want {
		t.Fatalf("replay timeline = %v, want %s (both hops before the 25m request)", types, want)
	}
}

func TestReplayOnRealtimeClockSaysTimingMayDiffer(t *testing.T) {
	app := testutil.New(t, 0)
	id, _ := createSimulation(t, app, map[string]any{})
	app.Do(t, "GET", "/zenvia/subscriptions", map[string]string{"X-API-TOKEN": "t", "X-Mimic-Simulation": id}, nil)
	res := replay(t, app, id)
	if res.Total != 1 || res.Matched != 1 || !strings.Contains(res.Timing, "realtime") {
		t.Fatalf("replay = %+v", res)
	}
	if rec := app.Do(t, "POST", "/admin/simulations/sim_missing/replay", nil, nil); rec.Code != 404 {
		t.Fatalf("unknown simulation: %d, want 404", rec.Code)
	}
}
