package profile_test

import (
	"testing"
	"time"

	"github.com/golgimed/mimic/internal/shared/clock"
	"github.com/golgimed/mimic/internal/shared/sim"
	"github.com/golgimed/mimic/internal/testutil"
)

func simHeaders(t *testing.T, app *testutil.App, body map[string]any) map[string]string {
	t.Helper()
	rec := app.Do(t, "POST", "/admin/simulations", nil, body)
	if rec.Code != 201 {
		t.Fatalf("create simulation %v: %d %s", body, rec.Code, rec.Body.String())
	}
	var s struct {
		ID      string `json:"id"`
		Profile string `json:"profile"`
	}
	testutil.DecodeJSON(t, rec, &s)
	if s.Profile != body["profile"] {
		t.Fatalf("simulation profile = %q, want %v", s.Profile, body["profile"])
	}
	return map[string]string{"X-API-TOKEN": "test-token", "X-Mimic-Simulation": s.ID}
}

func useVirtualClock(t *testing.T) {
	t.Helper()
	c, _ := clock.New(clock.Virtual, 1)
	clock.Set(c)
	t.Cleanup(func() { rt, _ := clock.New(clock.Realtime, 1); clock.Set(rt) })
}

func messageStatus(t *testing.T, app *testutil.App, headers map[string]string) func() string {
	t.Helper()
	rec := app.Do(t, "POST", "/zenvia/channels/sms/messages", headers, map[string]any{
		"from": "acct", "to": "5511999999999", "contents": []map[string]any{{"type": "text", "text": "hi"}},
	})
	var msg map[string]any
	testutil.DecodeJSON(t, rec, &msg)
	id := msg["id"].(string)
	return func() string {
		var d struct {
			Payload map[string]any `json:"payload"`
		}
		testutil.DecodeJSON(t, app.Do(t, "GET", "/admin/items/zenvia/"+id, nil, nil), &d)
		return d.Payload["status"].(string)
	}
}

func TestFastProfileMakesTransitionsImmediate(t *testing.T) {
	app := testutil.New(t, 10*time.Minute)
	useVirtualClock(t)

	status := messageStatus(t, app, simHeaders(t, app, map[string]any{"profile": "fast"}))
	app.Tick(t)
	app.Tick(t)
	if s := status(); s != "DELIVERED" {
		t.Fatalf("fast profile: %s after two ticks, want DELIVERED despite a 10m provider delay", s)
	}

	// Same provider, no profile: still waiting on the 10m delay.
	slow := messageStatus(t, app, map[string]string{"X-API-TOKEN": "t"})
	app.Tick(t)
	if s := slow(); s != "ACCEPTED" {
		t.Fatalf("no profile: %s, want ACCEPTED", s)
	}
}

func TestProfileComposesWithScenario(t *testing.T) {
	app := testutil.New(t, 10*time.Minute)
	useVirtualClock(t)
	status := messageStatus(t, app, simHeaders(t, app, map[string]any{"profile": "fast", "scenario": "message-not-delivered"}))
	app.Tick(t)
	app.Tick(t)
	if s := status(); s != "NOT_DELIVERED" {
		t.Fatalf("fast + message-not-delivered: %s, want NOT_DELIVERED", s)
	}
}

// chaosPattern sends n requests under the chaos profile with seed and
// returns which failed.
func chaosPattern(t *testing.T, seed int64, n int) string {
	t.Helper()
	app := testutil.New(t, 0)
	useVirtualClock(t) // chaos latency doesn't sleep on a virtual clock
	headers := simHeaders(t, app, map[string]any{"profile": "chaos", "seed": seed})
	out := make([]byte, n)
	for i := range out {
		switch rec := app.Do(t, "GET", "/zenvia/subscriptions", headers, nil); rec.Code {
		case 503:
			out[i] = 'F'
		case 200:
			out[i] = '.'
		default:
			t.Fatalf("status %d", rec.Code)
		}
	}
	return string(out)
}

func TestChaosProfileFailsSomeRequestsReproducibly(t *testing.T) {
	a := chaosPattern(t, 5, 200)
	if b := chaosPattern(t, 5, 200); a != b {
		t.Fatalf("chaos under the same seed differs:\n%s\n%s", a, b)
	}
	failed := 0
	for _, c := range a {
		if c == 'F' {
			failed++
		}
	}
	if failed < 5 || failed > 45 {
		t.Fatalf("%d/200 failed under a 10%% failure rate", failed)
	}
}

func TestExplicitFaultOverridesProfile(t *testing.T) {
	app := testutil.New(t, 0)
	useVirtualClock(t)
	app.Do(t, "PUT", "/admin/faults", nil, map[string]any{"provider": "zenvia", "faultKind": "http_status", "faultValue": "418"})
	headers := simHeaders(t, app, map[string]any{"profile": "chaos"})
	for range 30 {
		if rec := app.Do(t, "GET", "/zenvia/subscriptions", headers, nil); rec.Code != 418 {
			t.Fatalf("status %d, want the explicit 418 fault every time", rec.Code)
		}
	}
}

func TestRealisticProfileAddsLatency(t *testing.T) {
	app := testutil.New(t, 0)
	headers := simHeaders(t, app, map[string]any{"profile": "realistic", "seed": 1})
	start := time.Now()
	app.Do(t, "GET", "/zenvia/subscriptions", headers, nil)
	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Fatalf("request took %v, want the profile's ~150ms latency", elapsed)
	}
}

func TestProfileCatalogAndValidation(t *testing.T) {
	app := testutil.New(t, 0)
	var body struct {
		Content []struct{ Name string } `json:"content"`
	}
	testutil.DecodeJSON(t, app.Do(t, "GET", "/admin/profiles", nil, nil), &body)
	if len(body.Content) != 3 {
		t.Fatalf("profiles = %+v", body.Content)
	}
	if rec := app.Do(t, "POST", "/admin/simulations", nil, map[string]any{"profile": "turbo"}); rec.Code != 400 {
		t.Fatalf("unknown profile: %d, want 400", rec.Code)
	}
}

func TestDefaultProfileAppliesToImplicitSimulations(t *testing.T) {
	app := testutil.New(t, 0)
	if err := sim.SetDefaultProfile("fast"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sim.SetDefaultProfile("") })

	rec := app.Do(t, "GET", "/zenvia/subscriptions", map[string]string{"X-API-TOKEN": "t"}, nil)
	id := rec.Header().Get("X-Mimic-Simulation-ID")
	var s struct{ Profile string }
	testutil.DecodeJSON(t, app.Do(t, "GET", "/admin/simulations/"+id, nil, nil), &s)
	if s.Profile != "fast" {
		t.Fatalf("implicit simulation profile = %q, want the MIMIC_PROFILE default", s.Profile)
	}
}
