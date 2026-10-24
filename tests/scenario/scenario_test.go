package scenario_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/golgimed/mimic/internal/shared/clock"
	"github.com/golgimed/mimic/internal/testutil"
)

var token = map[string]string{"X-API-TOKEN": "test-token"}

func newSimulation(t *testing.T, app *testutil.App, scenario string) map[string]string {
	t.Helper()
	rec := app.Do(t, "POST", "/admin/simulations", nil, map[string]any{"scenario": scenario, "seed": 1})
	if rec.Code != 201 {
		t.Fatalf("create simulation %q: %d %s", scenario, rec.Code, rec.Body.String())
	}
	var s struct {
		ID       string `json:"id"`
		Scenario string `json:"scenario"`
	}
	testutil.DecodeJSON(t, rec, &s)
	if s.Scenario != scenario {
		t.Fatalf("simulation scenario = %q", s.Scenario)
	}
	return map[string]string{"X-API-TOKEN": "test-token", "Authorization": "Bearer t", "X-Mimic-Simulation": s.ID}
}

type receiver struct {
	mu     sync.Mutex
	events []map[string]any
}

func newReceiver(t *testing.T) (*receiver, string) {
	r := &receiver{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var e map[string]any
		_ = json.NewDecoder(req.Body).Decode(&e)
		r.mu.Lock()
		r.events = append(r.events, e)
		r.mu.Unlock()
		w.WriteHeader(204)
	}))
	t.Cleanup(srv.Close)
	return r, srv.URL
}

func (r *receiver) statuses() []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []map[string]any{}
	for _, e := range r.events {
		if s, ok := e["messageStatus"].(map[string]any); ok {
			out = append(out, s)
		}
	}
	return out
}

func sendSMS(t *testing.T, app *testutil.App, headers map[string]string) {
	t.Helper()
	rec := app.Do(t, "POST", "/zenvia/channels/sms/messages", headers, map[string]any{
		"from": "acct", "to": "5511999999999",
		"contents": []map[string]any{{"type": "text", "text": "hi"}},
	})
	if rec.Code != 200 {
		t.Fatalf("send: %d %s", rec.Code, rec.Body.String())
	}
}

func subscribe(t *testing.T, app *testutil.App, url string) {
	t.Helper()
	app.Do(t, "POST", "/zenvia/subscriptions", token, map[string]any{
		"eventType": "MESSAGE_STATUS", "webhook": map[string]any{"url": url}, "criteria": map[string]any{"channel": "sms"},
	})
}

func TestScenarioCatalogIsDiscoverable(t *testing.T) {
	app := testutil.New(t, 0)
	var body struct {
		Content []struct {
			Name      string   `json:"name"`
			Provider  string   `json:"provider"`
			Lifecycle []string `json:"lifecycle"`
		} `json:"content"`
	}
	testutil.DecodeJSON(t, app.Do(t, "GET", "/admin/scenarios", nil, nil), &body)
	got := map[string][]string{}
	for _, s := range body.Content {
		got[s.Provider+"/"+s.Name] = s.Lifecycle
	}
	for _, want := range []string{"zenvia/message-delivered", "zenvia/message-not-delivered", "zenvia/zenvia-unavailable", "bry-scad/collection-completed", "bry-scad/collection-rejected"} {
		if _, ok := got[want]; !ok {
			t.Errorf("scenario %s not listed (got %v)", want, got)
		}
	}
}

func TestMessageNotDeliveredScenario(t *testing.T) {
	app := testutil.New(t, 0)
	recv, url := newReceiver(t)
	subscribe(t, app, url)

	sendSMS(t, app, newSimulation(t, app, "message-not-delivered"))
	app.Tick(t)
	app.Tick(t)
	app.Tick(t) // nothing further: NOT_DELIVERED is terminal

	statuses := recv.statuses()
	if len(statuses) != 2 || statuses[0]["code"] != "SENT" || statuses[1]["code"] != "NOT_DELIVERED" {
		t.Fatalf("statuses = %+v, want SENT then NOT_DELIVERED", statuses)
	}
	causes, _ := statuses[1]["causes"].([]any)
	if len(causes) != 1 || causes[0].(map[string]any)["reason"] == nil || statuses[1]["description"] == nil {
		t.Errorf("NOT_DELIVERED event lacks description/causes: %+v", statuses[1])
	}
	if statuses[0]["causes"] != nil {
		t.Errorf("SENT event has causes: %+v", statuses[0])
	}
}

func TestDefaultPathWithoutScenario(t *testing.T) {
	app := testutil.New(t, 0)
	recv, url := newReceiver(t)
	subscribe(t, app, url)

	sendSMS(t, app, token)
	app.Tick(t)
	app.Tick(t)
	statuses := recv.statuses()
	if len(statuses) != 2 || statuses[1]["code"] != "DELIVERED" {
		t.Fatalf("statuses = %+v, want default SENT then DELIVERED", statuses)
	}
}

func TestScenarioFaultsAreScopedToTheSimulation(t *testing.T) {
	app := testutil.New(t, 0)
	headers := newSimulation(t, app, "zenvia-unavailable")
	if rec := app.Do(t, "GET", "/zenvia/subscriptions", headers, nil); rec.Code != 503 {
		t.Fatalf("in simulation: %d, want 503", rec.Code)
	}
	if rec := app.Do(t, "GET", "/zenvia/subscriptions", headers, nil); rec.Code != 503 {
		t.Fatalf("second request in simulation: %d, want 503 (fault persists)", rec.Code)
	}
	if rec := app.Do(t, "GET", "/zenvia/subscriptions", token, nil); rec.Code != 200 {
		t.Fatalf("outside simulation: %d, want 200", rec.Code)
	}
}

func TestCollectionCompletedScenario(t *testing.T) {
	app := testutil.New(t, 0)
	virtual, _ := clock.New(clock.Virtual, 1)
	clock.Set(virtual)
	t.Cleanup(func() { rt, _ := clock.New(clock.Realtime, 1); clock.Set(rt) })

	headers := newSimulation(t, app, "collection-completed")
	rec := app.Do(t, "POST", "/bry-scad/coletas/cadastrar", headers, map[string]any{
		"nome": "Contrato", "participantes": []map[string]any{{"email": "a@b.c", "assinante": true}},
	})
	var created map[string]any
	testutil.DecodeJSON(t, rec, &created)
	chave := created["chaveWorkflow"].(string)

	situacao := func() string {
		var detail struct {
			Payload map[string]any `json:"payload"`
		}
		testutil.DecodeJSON(t, app.Do(t, "GET", "/admin/items/bry-scad/"+chave, nil, nil), &detail)
		sit, _ := detail.Payload["situacao"].(map[string]any)
		s, _ := sit["chave"].(string)
		return s
	}
	if s := situacao(); s != "PENDENTE" {
		t.Fatalf("after create: %s", s)
	}
	app.Do(t, "POST", "/admin/clock/advance", nil, map[string]any{"duration": "30s"})
	if s := situacao(); s != "CONCLUIDO" {
		t.Fatalf("after 30s: %s, want CONCLUIDO", s)
	}
}

func TestUnknownScenarioRejected(t *testing.T) {
	app := testutil.New(t, 0)
	if rec := app.Do(t, "POST", "/admin/simulations", nil, map[string]any{"scenario": "nope"}); rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
