package admin_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/golgimed/mimic/internal/testutil"
)

func createSmsMessage(t *testing.T, app *testutil.App) *httptest.ResponseRecorder {
	t.Helper()
	return app.Do(t, "POST", "/zenvia/channels/sms/messages",
		map[string]string{"X-API-TOKEN": "test-token"},
		map[string]any{
			"from":     "sms-account",
			"to":       "55108888888888",
			"contents": []map[string]any{{"type": "text", "text": "Hi!"}},
		},
	)
}

func TestFaultDelayMs(t *testing.T) {
	app := testutil.New(t, 0)

	app.Do(t, "PUT", "/admin/faults", nil, map[string]any{
		"provider": "zenvia", "routePattern": "/zenvia/channels/sms/messages",
		"faultKind": "delay_ms", "faultValue": "80",
	})

	start := time.Now()
	rec := createSmsMessage(t, app)
	elapsed := time.Since(start)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if elapsed < 75*time.Millisecond {
		t.Errorf("expected delay >= 75ms, got %v", elapsed)
	}
}

func TestFaultHTTPStatus(t *testing.T) {
	app := testutil.New(t, 0)

	app.Do(t, "PUT", "/admin/faults", nil, map[string]any{
		"provider": "zenvia", "routePattern": "/zenvia/channels/sms/messages",
		"faultKind": "http_status", "faultValue": "503",
	})

	rec := createSmsMessage(t, app)
	if rec.Code != 503 {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
	var body struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	testutil.DecodeJSON(t, rec, &body)
	if body.Error.Code != 503 {
		t.Errorf("error.code = %v", body.Error.Code)
	}
}

func TestFaultInvalidPayload(t *testing.T) {
	app := testutil.New(t, 0)

	app.Do(t, "PUT", "/admin/faults", nil, map[string]any{
		"provider": "zenvia", "routePattern": "/zenvia/channels/sms/messages",
		"faultKind": "invalid_payload",
	})

	rec := createSmsMessage(t, app)
	var v any
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err == nil {
		t.Errorf("expected invalid JSON body, got valid: %s", rec.Body.String())
	}
}

func TestFaultTimeoutNeverResolves(t *testing.T) {
	app := testutil.New(t, 0)

	app.Do(t, "PUT", "/admin/faults", nil, map[string]any{
		"provider": "zenvia", "routePattern": "/zenvia/channels/sms/messages",
		"faultKind": "timeout",
	})

	done := make(chan struct{})
	go func() {
		createSmsMessage(t, app)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("expected request to hang, but it completed")
	case <-time.After(150 * time.Millisecond):
		// expected: still hanging
	}
}

func TestFaultConsumedOnce(t *testing.T) {
	app := testutil.New(t, 0)

	app.Do(t, "PUT", "/admin/faults", nil, map[string]any{
		"provider": "zenvia", "routePattern": "/zenvia/channels/sms/messages",
		"faultKind": "http_status", "faultValue": "500", "times": 1,
	})

	first := createSmsMessage(t, app)
	if first.Code != 500 {
		t.Fatalf("expected first request 500, got %d", first.Code)
	}

	second := createSmsMessage(t, app)
	if second.Code != 200 {
		t.Fatalf("expected second request 200, got %d", second.Code)
	}
}

func TestFaultConsumedOnceUnderConcurrency(t *testing.T) {
	app := testutil.New(t, 0)

	app.Do(t, "PUT", "/admin/faults", nil, map[string]any{
		"provider": "zenvia", "routePattern": "/zenvia/channels/sms/messages",
		"faultKind": "http_status", "faultValue": "500", "times": 1,
	})

	const n = 20
	codes := make([]int, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			codes[i] = createSmsMessage(t, app).Code
		}(i)
	}
	wg.Wait()

	fired := 0
	for _, c := range codes {
		if c == 500 {
			fired++
		} else if c != 200 {
			t.Fatalf("unexpected status code %d", c)
		}
	}
	if fired != 1 {
		t.Fatalf("expected exactly 1 request to observe the fault, got %d", fired)
	}
}

type webhookReceiver struct {
	server *httptest.Server
	mu     sync.Mutex
	events []map[string]any
}

func newWebhookReceiver(t *testing.T) *webhookReceiver {
	t.Helper()
	r := &webhookReceiver{}
	r.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		var event map[string]any
		_ = json.Unmarshal(body, &event)
		r.mu.Lock()
		r.events = append(r.events, event)
		r.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(r.server.Close)
	return r
}

func (r *webhookReceiver) URL() string { return r.server.URL + "/webhook" }

func (r *webhookReceiver) Events() []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]map[string]any, len(r.events))
	copy(out, r.events)
	return out
}

func TestWebhookDroppedSkipsDelivery(t *testing.T) {
	app := testutil.New(t, 0)
	receiver := newWebhookReceiver(t)

	app.Do(t, "POST", "/zenvia/subscriptions", map[string]string{"X-API-TOKEN": "test-token"}, map[string]any{
		"eventType": "MESSAGE_STATUS",
		"webhook":   map[string]any{"url": receiver.URL()},
		"criteria":  map[string]any{"channel": "sms"},
	})
	app.Do(t, "PUT", "/admin/faults", nil, map[string]any{
		"provider": "zenvia", "routePattern": "webhook", "faultKind": "webhook_dropped",
	})

	createSmsMessage(t, app)
	app.Tick(t)

	if len(receiver.Events()) != 0 {
		t.Fatalf("expected 0 events, got %d", len(receiver.Events()))
	}
}

func TestWebhookInvalidDeliversMangledPayload(t *testing.T) {
	app := testutil.New(t, 0)
	receiver := newWebhookReceiver(t)

	app.Do(t, "POST", "/zenvia/subscriptions", map[string]string{"X-API-TOKEN": "test-token"}, map[string]any{
		"eventType": "MESSAGE_STATUS",
		"webhook":   map[string]any{"url": receiver.URL()},
		"criteria":  map[string]any{"channel": "sms"},
	})
	app.Do(t, "PUT", "/admin/faults", nil, map[string]any{
		"provider": "zenvia", "routePattern": "webhook", "faultKind": "webhook_invalid",
	})

	createSmsMessage(t, app)
	app.Tick(t)

	events := receiver.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if _, hasStatus := events[0]["messageStatus"]; hasStatus {
		t.Errorf("expected messageStatus to be absent, got %v", events[0]["messageStatus"])
	}
	if events[0]["malformed"] != true {
		t.Errorf("expected malformed=true, got %v", events[0]["malformed"])
	}
}

func TestListsAndDeletesFaults(t *testing.T) {
	app := testutil.New(t, 0)

	createRec := app.Do(t, "PUT", "/admin/faults", nil, map[string]any{
		"provider": "integraicp", "faultKind": "delay_ms", "faultValue": "10",
	})
	var created struct {
		ID string `json:"id"`
	}
	testutil.DecodeJSON(t, createRec, &created)

	listRec := app.Do(t, "GET", "/admin/faults", nil, nil)
	var list struct {
		Content []map[string]any `json:"content"`
	}
	testutil.DecodeJSON(t, listRec, &list)
	if len(list.Content) != 1 {
		t.Fatalf("expected 1 fault, got %d", len(list.Content))
	}

	deleteRec := app.Do(t, "DELETE", "/admin/faults/"+created.ID, nil, nil)
	if deleteRec.Code != 204 {
		t.Fatalf("expected 204, got %d", deleteRec.Code)
	}

	listAfterRec := app.Do(t, "GET", "/admin/faults", nil, nil)
	var listAfter struct {
		Content []map[string]any `json:"content"`
	}
	testutil.DecodeJSON(t, listAfterRec, &listAfter)
	if len(listAfter.Content) != 0 {
		t.Fatalf("expected 0 faults, got %d", len(listAfter.Content))
	}
}
