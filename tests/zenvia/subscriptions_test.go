package zenvia_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/golgimed/mimic/internal/testutil"
)

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

func (r *webhookReceiver) URL() string {
	return r.server.URL + "/webhook"
}

func (r *webhookReceiver) Events() []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]map[string]any, len(r.events))
	copy(out, r.events)
	return out
}

func createSmsMessage(t *testing.T, app *testutil.App) *httptest.ResponseRecorder {
	t.Helper()
	return app.Do(t, "POST", "/zenvia/channels/sms/messages",
		map[string]string{"X-API-TOKEN": "test-token"},
		map[string]any{
			"from":     "sms-account",
			"to":       "55108888888888",
			"contents": []map[string]any{{"type": "text", "text": "Hi Zenvia!"}},
		},
	)
}

func TestMessageStatusWebhookFlow(t *testing.T) {
	app := testutil.New(t, 0)
	receiver := newWebhookReceiver(t)

	subRec := app.Do(t, "POST", "/zenvia/subscriptions",
		map[string]string{"X-API-TOKEN": "test-token"},
		map[string]any{
			"eventType": "MESSAGE_STATUS",
			"webhook":   map[string]any{"url": receiver.URL()},
			"criteria":  map[string]any{"channel": "sms"},
		},
	)
	if subRec.Code != 200 {
		t.Fatalf("subscribe: expected 200, got %d: %s", subRec.Code, subRec.Body.String())
	}

	msgRec := createSmsMessage(t, app)
	if msgRec.Code != 200 {
		t.Fatalf("create message: expected 200, got %d: %s", msgRec.Code, msgRec.Body.String())
	}
	var msg map[string]any
	testutil.DecodeJSON(t, msgRec, &msg)
	messageID := msg["id"]

	app.Tick(t) // ACCEPTED -> SENT
	app.Tick(t) // SENT -> DELIVERED

	events := receiver.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d: %+v", len(events), events)
	}
	if events[0]["type"] != "MESSAGE_STATUS" {
		t.Errorf("event[0].type = %v", events[0]["type"])
	}
	status0 := events[0]["messageStatus"].(map[string]any)
	if status0["code"] != "SENT" {
		t.Errorf("event[0].messageStatus.code = %v", status0["code"])
	}
	message0 := events[0]["message"].(map[string]any)
	if message0["id"] != messageID {
		t.Errorf("event[0].message.id = %v, want %v", message0["id"], messageID)
	}
	status1 := events[1]["messageStatus"].(map[string]any)
	if status1["code"] != "DELIVERED" {
		t.Errorf("event[1].messageStatus.code = %v", status1["code"])
	}
}

func TestListAndDeleteSubscription(t *testing.T) {
	app := testutil.New(t, 0)
	receiver := newWebhookReceiver(t)

	createRec := app.Do(t, "POST", "/zenvia/subscriptions",
		map[string]string{"X-API-TOKEN": "test-token"},
		map[string]any{
			"eventType": "MESSAGE_STATUS",
			"webhook":   map[string]any{"url": receiver.URL()},
			"criteria":  map[string]any{"channel": "sms"},
		},
	)
	var created map[string]any
	testutil.DecodeJSON(t, createRec, &created)
	id := created["id"].(string)

	listRec := app.Do(t, "GET", "/zenvia/subscriptions", map[string]string{"X-API-TOKEN": "test-token"}, nil)
	var list struct {
		Content []map[string]any `json:"content"`
	}
	testutil.DecodeJSON(t, listRec, &list)
	if len(list.Content) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(list.Content))
	}

	deleteRec := app.Do(t, "DELETE", "/zenvia/subscriptions/"+id, map[string]string{"X-API-TOKEN": "test-token"}, nil)
	if deleteRec.Code != 204 {
		t.Fatalf("expected 204, got %d", deleteRec.Code)
	}

	getRec := app.Do(t, "GET", "/zenvia/subscriptions/"+id, map[string]string{"X-API-TOKEN": "test-token"}, nil)
	if getRec.Code != 404 {
		t.Fatalf("expected 404, got %d", getRec.Code)
	}
}

func TestUnknownSubscription404(t *testing.T) {
	app := testutil.New(t, 0)
	rec := app.Do(t, "GET", "/zenvia/subscriptions/does-not-exist", map[string]string{"X-API-TOKEN": "test-token"}, nil)
	if rec.Code != 404 {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestSkipsSubscriptionWithMismatchedDirection(t *testing.T) {
	app := testutil.New(t, 0)
	receiver := newWebhookReceiver(t)

	app.Do(t, "POST", "/zenvia/subscriptions",
		map[string]string{"X-API-TOKEN": "test-token"},
		map[string]any{
			"eventType": "MESSAGE_STATUS",
			"webhook":   map[string]any{"url": receiver.URL()},
			"criteria":  map[string]any{"channel": "sms", "direction": "IN"},
		},
	)

	createSmsMessage(t, app)

	app.Tick(t)
	app.Tick(t)

	if len(receiver.Events()) != 0 {
		t.Fatalf("expected 0 events, got %d", len(receiver.Events()))
	}
}

func TestDeliversToDirectionAll(t *testing.T) {
	app := testutil.New(t, 0)
	receiver := newWebhookReceiver(t)

	app.Do(t, "POST", "/zenvia/subscriptions",
		map[string]string{"X-API-TOKEN": "test-token"},
		map[string]any{
			"eventType": "MESSAGE_STATUS",
			"webhook":   map[string]any{"url": receiver.URL()},
			"criteria":  map[string]any{"channel": "sms", "direction": "ALL"},
		},
	)

	createSmsMessage(t, app)
	app.Tick(t)

	if len(receiver.Events()) != 1 {
		t.Fatalf("expected 1 event, got %d", len(receiver.Events()))
	}
}
