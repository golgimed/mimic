package simulation_test

import (
	"strconv"
	"testing"

	"github.com/golgimed/mimic/internal/testutil"
)

var token = map[string]string{"X-API-TOKEN": "test-token"}

func createSimulation(t *testing.T, app *testutil.App, body map[string]any) (string, int64) {
	t.Helper()
	rec := app.Do(t, "POST", "/admin/simulations", nil, body)
	if rec.Code != 201 {
		t.Fatalf("create simulation: %d %s", rec.Code, rec.Body.String())
	}
	var s struct {
		ID   string `json:"id"`
		Seed int64  `json:"seed"`
	}
	testutil.DecodeJSON(t, rec, &s)
	return s.ID, s.Seed
}

// faultPattern runs n requests in a fresh app under seed against a 50%
// provider-wide 503 fault and returns which ones were faulted.
func faultPattern(t *testing.T, seed int64, n int) string {
	t.Helper()
	app := testutil.New(t, 0)
	app.Do(t, "PUT", "/admin/faults", nil, map[string]any{
		"provider": "zenvia", "faultKind": "http_status", "faultValue": "503", "probability": 0.5,
	})
	id, _ := createSimulation(t, app, map[string]any{"seed": seed})
	headers := map[string]string{"X-API-TOKEN": "test-token", "X-Mimic-Simulation": id}

	out := make([]byte, n)
	for i := range out {
		rec := app.Do(t, "GET", "/zenvia/subscriptions", headers, nil)
		switch rec.Code {
		case 503:
			out[i] = 'F'
		case 200:
			out[i] = '.'
		default:
			t.Fatalf("unexpected status %d", rec.Code)
		}
	}
	return string(out)
}

func TestSameSeedSameFaultDecisions(t *testing.T) {
	a := faultPattern(t, 42, 40)
	b := faultPattern(t, 42, 40)
	if a != b {
		t.Fatalf("same seed gave different fault decisions:\n%s\n%s", a, b)
	}
	if c := faultPattern(t, 43, 40); c == a {
		t.Errorf("seeds 42 and 43 produced identical 40-roll patterns %s (vanishingly unlikely)", a)
	}
	faulted := 0
	for _, ch := range a {
		if ch == 'F' {
			faulted++
		}
	}
	if faulted == 0 || faulted == len(a) {
		t.Errorf("pattern %s: 50%% fault never/always fired", a)
	}
}

func TestSimulationSeedIsGeneratedAndExposed(t *testing.T) {
	app := testutil.New(t, 0)
	id, seed := createSimulation(t, app, map[string]any{})
	if id == "" || seed <= 0 {
		t.Fatalf("generated simulation = %q seed %d", id, seed)
	}

	rec := app.Do(t, "GET", "/zenvia/subscriptions", map[string]string{"X-API-TOKEN": "t", "X-Mimic-Simulation": id}, nil)
	if rec.Header().Get("X-Mimic-Simulation-ID") != id || rec.Header().Get("X-Mimic-Seed") != strconv.FormatInt(seed, 10) {
		t.Fatalf("headers = %v, want simulation %s seed %d", rec.Header(), id, seed)
	}

	get := app.Do(t, "GET", "/admin/simulations/"+id, nil, nil)
	if get.Code != 200 {
		t.Fatalf("GET simulation: %d", get.Code)
	}

	// Without the header, each request runs in its own implicit simulation.
	r1 := app.Do(t, "GET", "/zenvia/subscriptions", token, nil)
	r2 := app.Do(t, "GET", "/zenvia/subscriptions", token, nil)
	if r1.Header().Get("X-Mimic-Simulation-ID") == "" || r1.Header().Get("X-Mimic-Simulation-ID") == r2.Header().Get("X-Mimic-Simulation-ID") {
		t.Fatalf("implicit simulations not distinct per request")
	}

	var list struct {
		Content []map[string]any `json:"content"`
	}
	testutil.DecodeJSON(t, app.Do(t, "GET", "/admin/simulations", nil, nil), &list)
	if len(list.Content) != 1 {
		t.Errorf("explicit simulations listed = %d, want 1 (implicit hidden)", len(list.Content))
	}
}

func TestSimulationRequestValidation(t *testing.T) {
	app := testutil.New(t, 0)
	if rec := app.Do(t, "POST", "/admin/simulations", nil, map[string]any{"seed": 0}); rec.Code != 400 {
		t.Errorf("seed 0: status %d, want 400", rec.Code)
	}
	rec := app.Do(t, "GET", "/zenvia/subscriptions", map[string]string{"X-API-TOKEN": "t", "X-Mimic-Simulation": "sim_nope"}, nil)
	if rec.Code != 400 {
		t.Fatalf("unknown simulation: status %d, want 400", rec.Code)
	}
	var body struct {
		Error struct{ Code string } `json:"error"`
	}
	testutil.DecodeJSON(t, rec, &body)
	if body.Error.Code != "UNKNOWN_SIMULATION" {
		t.Errorf("error code = %q", body.Error.Code)
	}
}

func TestSeedFollowsScheduledJobs(t *testing.T) {
	app := testutil.New(t, 0)
	id, _ := createSimulation(t, app, map[string]any{"seed": 7})
	headers := map[string]string{"X-API-TOKEN": "test-token", "X-Mimic-Simulation": id}
	app.Do(t, "POST", "/zenvia/channels/sms/messages", headers, map[string]any{
		"from": "acct", "to": "5511999999999",
		"contents": []map[string]any{{"type": "text", "text": "hi"}},
	})
	app.Tick(t)
	app.Tick(t)

	var tl struct {
		Content []struct {
			Type         string `json:"type"`
			SimulationID string `json:"simulationId"`
		} `json:"content"`
	}
	testutil.DecodeJSON(t, app.Do(t, "GET", "/admin/timeline?simulation_id="+id, nil, nil), &tl)
	jobs := 0
	for _, e := range tl.Content {
		if e.Type == "JobStarted" {
			jobs++
		}
	}
	if jobs != 2 {
		t.Fatalf("JobStarted events in simulation = %d, want 2 (jobs must run in the scheduling simulation)", jobs)
	}
}
