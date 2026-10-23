package timeline_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golgimed/mimic/internal/testutil"
)

var token = map[string]string{"X-API-TOKEN": "test-token"}

type event struct {
	Type         string         `json:"type"`
	Provider     string         `json:"provider"`
	ResourceType string         `json:"resourceType"`
	ResourceID   string         `json:"resourceId"`
	JobID        int64          `json:"jobId"`
	Data         map[string]any `json:"data"`
}

func listTimeline(t *testing.T, app *testutil.App, query string) []event {
	t.Helper()
	rec := app.Do(t, "GET", "/admin/timeline"+query, nil, nil)
	if rec.Code != 200 {
		t.Fatalf("GET /admin/timeline%s: %d %s", query, rec.Code, rec.Body.String())
	}
	var body struct {
		Content []event `json:"content"`
	}
	testutil.DecodeJSON(t, rec, &body)
	return body.Content
}

func types(events []event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.Type
	}
	return out
}

func TestTimelineRecordsZenviaLifecycle(t *testing.T) {
	app := testutil.New(t, 0)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	t.Cleanup(receiver.Close)

	app.Do(t, "POST", "/zenvia/subscriptions", token, map[string]any{
		"eventType": "MESSAGE_STATUS",
		"webhook":   map[string]any{"url": receiver.URL},
		"criteria":  map[string]any{"channel": "sms"},
	})
	msgRec := app.Do(t, "POST", "/zenvia/channels/sms/messages", token, map[string]any{
		"from": "acct", "to": "5511999999999",
		"contents": []map[string]any{{"type": "text", "text": "hi"}},
	})
	var msg map[string]any
	testutil.DecodeJSON(t, msgRec, &msg)
	id := msg["id"].(string)

	app.Tick(t) // ACCEPTED -> SENT
	app.Tick(t) // SENT -> DELIVERED

	got := types(listTimeline(t, app, "?resource_id="+id))
	want := []string{"ResourceCreated", "StateChanged", "WebhookDelivered", "StateChanged", "WebhookDelivered"}
	if len(got) != len(want) {
		t.Fatalf("resource timeline = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("resource timeline = %v, want %v", got, want)
		}
	}

	all := listTimeline(t, app, "?provider=zenvia")
	counts := map[string]int{}
	for _, e := range all {
		counts[e.Type]++
		if e.Type == "StateChanged" && e.Data["from"] == "SENT" && e.Data["to"] != "DELIVERED" {
			t.Errorf("unexpected transition %+v", e.Data)
		}
	}
	// 2 requests (subscribe + send), each received and returned.
	if counts["RequestReceived"] != 2 || counts["ResponseReturned"] != 2 {
		t.Errorf("request events = %v", counts)
	}

	jobs := listTimeline(t, app, "")
	jobCounts := map[string]int{}
	for _, e := range jobs {
		jobCounts[e.Type]++
	}
	if jobCounts["JobScheduled"] != 2 || jobCounts["JobStarted"] != 2 || jobCounts["JobCompleted"] != 2 {
		t.Errorf("job events = %v", jobCounts)
	}
}

func TestTimelineRecordsFaultsAndExcludesAdmin(t *testing.T) {
	app := testutil.New(t, 0)
	app.Do(t, "PUT", "/admin/faults", nil, map[string]any{
		"provider": "zenvia", "routePattern": "/zenvia/subscriptions", "faultKind": "http_status", "faultValue": "503", "times": 1,
	})
	rec := app.Do(t, "GET", "/zenvia/subscriptions", token, nil)
	if rec.Code != 503 {
		t.Fatalf("status = %d, want injected 503", rec.Code)
	}

	events := listTimeline(t, app, "")
	got := types(events)
	want := []string{"RequestReceived", "FaultApplied", "ResponseReturned"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("timeline = %v, want %v (admin calls excluded)", got, want)
	}
	if events[1].Data["kind"] != "http_status" || events[2].Data["status"] != float64(503) {
		t.Errorf("fault/response data = %+v / %+v", events[1].Data, events[2].Data)
	}

	if rec := app.Do(t, "GET", "/admin/timeline?limit=0", nil, nil); rec.Code != 400 {
		t.Errorf("limit=0 status = %d, want 400", rec.Code)
	}

	app.Do(t, "POST", "/admin/flush", nil, nil)
	if n := len(listTimeline(t, app, "")); n != 0 {
		t.Errorf("after flush: %d events, want 0", n)
	}
}
