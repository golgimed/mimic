package timeline_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/golgimed/mimic/internal/testutil"
)

func TestCorrelationIDFollowsAsyncLifecycle(t *testing.T) {
	app := testutil.New(t, 0)

	var mu sync.Mutex
	var webhookIDs []string
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		webhookIDs = append(webhookIDs, r.Header.Get("X-Mimic-Request-ID"))
		mu.Unlock()
		w.WriteHeader(204)
	}))
	t.Cleanup(receiver.Close)

	app.Do(t, "POST", "/zenvia/subscriptions", token, map[string]any{
		"eventType": "MESSAGE_STATUS",
		"webhook":   map[string]any{"url": receiver.URL},
		"criteria":  map[string]any{"channel": "sms"},
	})
	headers := map[string]string{"X-API-TOKEN": "test-token", "X-Request-ID": "req-abc-123"}
	rec := app.Do(t, "POST", "/zenvia/channels/sms/messages", headers, map[string]any{
		"from": "acct", "to": "5511999999999",
		"contents": []map[string]any{{"type": "text", "text": "hi"}},
	})
	if got := rec.Header().Get("X-Mimic-Request-ID"); got != "req-abc-123" {
		t.Fatalf("X-Mimic-Request-ID = %q, want client's X-Request-ID echoed", got)
	}

	app.Tick(t)
	app.Tick(t)

	events := listTimeline(t, app, "?request_id=req-abc-123")
	counts := map[string]int{}
	for _, e := range events {
		counts[e.Type]++
	}
	want := map[string]int{
		"RequestReceived": 1, "ResponseReturned": 1, "ResourceCreated": 1,
		"JobScheduled": 2, "JobStarted": 2, "JobCompleted": 2,
		"StateChanged": 2, "WebhookDelivered": 2,
	}
	for typ, n := range want {
		if counts[typ] != n {
			t.Errorf("%s events for request = %d, want %d (all: %v)", typ, counts[typ], n, counts)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(webhookIDs) != 2 || webhookIDs[0] != "req-abc-123" || webhookIDs[1] != "req-abc-123" {
		t.Errorf("webhook X-Mimic-Request-ID headers = %v", webhookIDs)
	}
}

func TestCorrelationIDGeneratedWhenMissingOrInvalid(t *testing.T) {
	app := testutil.New(t, 0)
	first := app.Do(t, "GET", "/zenvia/subscriptions", token, nil).Header().Get("X-Mimic-Request-ID")
	second := app.Do(t, "GET", "/zenvia/subscriptions", map[string]string{"X-API-TOKEN": "t", "X-Request-ID": "has space"}, nil).Header().Get("X-Mimic-Request-ID")
	if first == "" || second == "" || first == second || second == "has space" {
		t.Fatalf("generated IDs = %q, %q; want two distinct fresh IDs", first, second)
	}
	if n := len(listTimeline(t, app, "?request_id="+first)); n != 2 {
		t.Errorf("events for generated ID = %d, want 2", n)
	}
}
