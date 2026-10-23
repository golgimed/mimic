package clock_test

import (
	"testing"
	"time"

	"github.com/golgimed/mimic/internal/shared/clock"
	"github.com/golgimed/mimic/internal/testutil"
)

var token = map[string]string{"X-API-TOKEN": "test-token"}

func useVirtualClock(t *testing.T) {
	t.Helper()
	c, _ := clock.New(clock.Virtual, 1)
	clock.Set(c)
	t.Cleanup(func() {
		rt, _ := clock.New(clock.Realtime, 1)
		clock.Set(rt)
	})
}

func advance(t *testing.T, app *testutil.App, d string) {
	t.Helper()
	rec := app.Do(t, "POST", "/admin/clock/advance", nil, map[string]any{"duration": d})
	if rec.Code != 200 {
		t.Fatalf("advance %s: %d %s", d, rec.Code, rec.Body.String())
	}
}

func messageStatus(t *testing.T, app *testutil.App, id string) string {
	t.Helper()
	var detail struct {
		Payload map[string]any `json:"payload"`
	}
	testutil.DecodeJSON(t, app.Do(t, "GET", "/admin/items/zenvia/"+id, nil, nil), &detail)
	s, _ := detail.Payload["status"].(string)
	return s
}

func sendSMS(t *testing.T, app *testutil.App) string {
	t.Helper()
	rec := app.Do(t, "POST", "/zenvia/channels/sms/messages", token, map[string]any{
		"from": "acct", "to": "5511999999999",
		"contents": []map[string]any{{"type": "text", "text": "hi"}},
	})
	var msg map[string]any
	testutil.DecodeJSON(t, rec, &msg)
	return msg["id"].(string)
}

func TestVirtualClockDrivesScheduledTransitions(t *testing.T) {
	app := testutil.New(t, 10*time.Minute)
	useVirtualClock(t)

	id := sendSMS(t, app)
	app.Tick(t) // nothing is due: virtual time hasn't moved
	if s := messageStatus(t, app, id); s != "ACCEPTED" {
		t.Fatalf("before advancing: %s, want ACCEPTED", s)
	}

	advance(t, app, "10m")
	if s := messageStatus(t, app, id); s != "SENT" {
		t.Fatalf("after 10m: %s, want SENT", s)
	}
	advance(t, app, "9m")
	if s := messageStatus(t, app, id); s != "SENT" {
		t.Fatalf("after 19m: %s, want still SENT", s)
	}
	advance(t, app, "1m")
	if s := messageStatus(t, app, id); s != "DELIVERED" {
		t.Fatalf("after 20m: %s, want DELIVERED", s)
	}
}

func TestOneAdvanceRunsChainedJobsAtTheirDueTimes(t *testing.T) {
	app := testutil.New(t, 10*time.Minute)
	useVirtualClock(t)
	start := clock.Now()

	id := sendSMS(t, app)
	advance(t, app, "24h")
	if s := messageStatus(t, app, id); s != "DELIVERED" {
		t.Fatalf("after 24h: %s, want DELIVERED (both hops)", s)
	}

	var tl struct {
		Content []struct {
			Type string         `json:"type"`
			Ts   string         `json:"ts"`
			Data map[string]any `json:"data"`
		} `json:"content"`
	}
	testutil.DecodeJSON(t, app.Do(t, "GET", "/admin/timeline?resource_id="+id, nil, nil), &tl)
	var at []time.Duration
	for _, e := range tl.Content {
		if e.Type == "StateChanged" {
			ts, _ := time.Parse(time.RFC3339Nano, e.Ts)
			at = append(at, ts.Sub(start).Round(time.Second))
		}
	}
	if len(at) != 2 || at[0] != 10*time.Minute || at[1] != 20*time.Minute {
		t.Fatalf("transitions happened at %v, want [10m 20m] of simulated time", at)
	}

	var c struct {
		Mode string `json:"mode"`
		Now  string `json:"now"`
	}
	testutil.DecodeJSON(t, app.Do(t, "GET", "/admin/clock", nil, nil), &c)
	now, _ := time.Parse(time.RFC3339Nano, c.Now)
	if c.Mode != "virtual" || now.Sub(start).Round(time.Second) != 24*time.Hour {
		t.Fatalf("clock = %+v, want virtual at start+24h", c)
	}
}

func TestAdvanceRejectedOnRealtimeClock(t *testing.T) {
	app := testutil.New(t, 0)
	rec := app.Do(t, "POST", "/admin/clock/advance", nil, map[string]any{"duration": "1m"})
	if rec.Code != 409 {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	if rec := app.Do(t, "POST", "/admin/clock/advance", nil, map[string]any{"duration": "soon"}); rec.Code != 400 {
		t.Fatalf("bad duration: %d, want 400", rec.Code)
	}
}
