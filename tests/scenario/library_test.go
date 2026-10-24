package scenario_test

import (
	"strings"
	"testing"

	"github.com/golgimed/mimic/internal/shared/clock"
	"github.com/golgimed/mimic/internal/testutil"
)

// Every built-in scenario has a test here; TestEveryScenarioIsTested keeps
// it that way.
var tested = map[string]bool{}

func covers(name string) { tested[name] = true }

// zenviaLifecycle sends one SMS in a simulation running scenario and
// returns the MESSAGE_STATUS codes webhooks received.
func zenviaLifecycle(t *testing.T, scenario string) []string {
	t.Helper()
	app := testutil.New(t, 0)
	recv, url := newReceiver(t)
	subscribe(t, app, url)
	sendSMS(t, app, newSimulation(t, app, scenario))
	for range 5 {
		app.Tick(t)
	}
	var codes []string
	for _, s := range recv.statuses() {
		codes = append(codes, s["code"].(string))
	}
	return codes
}

func TestZenviaLifecycleScenarios(t *testing.T) {
	for name, want := range map[string]string{
		"message-delivered":     "SENT DELIVERED",
		"message-not-delivered": "SENT NOT_DELIVERED",
		"message-rejected":      "REJECTED",
		"message-read":          "SENT DELIVERED READ",
	} {
		covers(name)
		t.Run(name, func(t *testing.T) {
			if got := strings.Join(zenviaLifecycle(t, name), " "); got != want {
				t.Fatalf("webhook statuses = %q, want %q", got, want)
			}
		})
	}
}

func TestWebhookDeliveryFailedScenario(t *testing.T) {
	covers("webhook-delivery-failed")
	app := testutil.New(t, 0)
	recv, url := newReceiver(t)
	subscribe(t, app, url)
	sendSMS(t, app, newSimulation(t, app, "webhook-delivery-failed"))
	app.Tick(t)
	app.Tick(t)
	if n := len(recv.statuses()); n != 0 {
		t.Fatalf("%d webhooks arrived, want all dropped", n)
	}

	var tl struct {
		Content []struct{ Type string } `json:"content"`
	}
	testutil.DecodeJSON(t, app.Do(t, "GET", "/admin/timeline?provider=zenvia", nil, nil), &tl)
	dropped, changed := 0, 0
	for _, e := range tl.Content {
		switch e.Type {
		case "WebhookDropped":
			dropped++
		case "StateChanged":
			changed++
		}
	}
	if dropped != 2 || changed != 2 {
		t.Fatalf("timeline: %d dropped webhooks, %d state changes; want 2 and 2 (lifecycle unaffected)", dropped, changed)
	}
}

func TestRequestRateLimitedScenario(t *testing.T) {
	covers("request-rate-limited")
	app := testutil.New(t, 0)
	headers := newSimulation(t, app, "request-rate-limited")
	var codes []int
	for range 3 {
		codes = append(codes, app.Do(t, "POST", "/zenvia/channels/sms/messages", headers, map[string]any{
			"from": "acct", "to": "5511999999999", "contents": []map[string]any{{"type": "text", "text": "hi"}},
		}).Code)
	}
	if codes[0] != 200 || codes[1] != 200 || codes[2] != 429 {
		t.Fatalf("statuses = %v, want [200 200 429]", codes)
	}
	// Other routes and other simulations are not limited.
	if rec := app.Do(t, "GET", "/zenvia/subscriptions", headers, nil); rec.Code != 200 {
		t.Fatalf("subscriptions in simulation: %d", rec.Code)
	}
}

func TestUnavailableScenarios(t *testing.T) {
	for name, path := range map[string]string{
		"zenvia-unavailable":     "/zenvia/subscriptions",
		"integraicp-unavailable": "/integraicp/c/ch1/icp/v3/authentications",
	} {
		covers(name)
		t.Run(name, func(t *testing.T) {
			app := testutil.New(t, 0)
			if rec := app.Do(t, "GET", path, newSimulation(t, app, name), nil); rec.Code != 503 {
				t.Fatalf("status = %d, want 503", rec.Code)
			}
		})
	}
}

func TestBryScadScenarios(t *testing.T) {
	for name, want := range map[string]string{
		"collection-completed": "CONCLUIDO",
		"collection-rejected":  "REJEITADA",
		"collection-cancelled": "CANCELADO",
	} {
		covers(name)
		t.Run(name, func(t *testing.T) {
			app := testutil.New(t, 0)
			virtual, _ := clock.New(clock.Virtual, 1)
			clock.Set(virtual)
			t.Cleanup(func() { rt, _ := clock.New(clock.Realtime, 1); clock.Set(rt) })

			rec := app.Do(t, "POST", "/bry-scad/coletas/cadastrar", newSimulation(t, app, name), map[string]any{
				"nome": "Contrato", "participantes": []map[string]any{{"email": "a@b.c", "assinante": true}},
			})
			var created map[string]any
			testutil.DecodeJSON(t, rec, &created)
			chave := created["chaveWorkflow"].(string)

			app.Do(t, "POST", "/admin/clock/advance", nil, map[string]any{"duration": "30s"})
			var detail struct {
				Payload map[string]any `json:"payload"`
			}
			testutil.DecodeJSON(t, app.Do(t, "GET", "/admin/items/bry-scad/"+chave, nil, nil), &detail)
			if got := detail.Payload["situacao"].(map[string]any)["chave"]; got != want {
				t.Fatalf("situacao = %v, want %s", got, want)
			}
		})
	}
}

func TestEveryScenarioIsTested(t *testing.T) {
	// Run the scenario tests first so their names are recorded.
	TestZenviaLifecycleScenarios(t)
	TestWebhookDeliveryFailedScenario(t)
	TestRequestRateLimitedScenario(t)
	TestUnavailableScenarios(t)
	TestBryScadScenarios(t)

	app := testutil.New(t, 0)
	var body struct {
		Content []struct{ Name string } `json:"content"`
	}
	testutil.DecodeJSON(t, app.Do(t, "GET", "/admin/scenarios", nil, nil), &body)
	for _, s := range body.Content {
		if !tested[s.Name] {
			t.Errorf("scenario %q has no test in library_test.go", s.Name)
		}
	}
}
