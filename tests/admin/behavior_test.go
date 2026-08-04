package admin_test

import (
	"testing"
	"time"

	"github.com/golgimed/mimic/internal/testutil"
)

func TestFaultProbabilityZeroNeverFires(t *testing.T) {
	app := testutil.New(t, 0)

	app.Do(t, "PUT", "/admin/faults", nil, map[string]any{
		"provider": "zenvia", "routePattern": "/zenvia/channels/sms/messages",
		"faultKind": "http_status", "faultValue": "503", "probability": 0,
	})

	for i := 0; i < 10; i++ {
		rec := createSmsMessage(t, app)
		if rec.Code != 200 {
			t.Fatalf("expected 200 with probability 0, got %d", rec.Code)
		}
	}
}

func TestFaultProbabilityOneAlwaysFires(t *testing.T) {
	app := testutil.New(t, 0)

	app.Do(t, "PUT", "/admin/faults", nil, map[string]any{
		"provider": "zenvia", "routePattern": "/zenvia/channels/sms/messages",
		"faultKind": "http_status", "faultValue": "503", "probability": 1,
	})

	for i := 0; i < 5; i++ {
		rec := createSmsMessage(t, app)
		if rec.Code != 503 {
			t.Fatalf("expected 503 with probability 1, got %d", rec.Code)
		}
	}
}

func TestFaultProbabilitySkipDoesNotConsumeUses(t *testing.T) {
	app := testutil.New(t, 0)

	app.Do(t, "PUT", "/admin/faults", nil, map[string]any{
		"provider": "zenvia", "routePattern": "/zenvia/channels/sms/messages",
		"faultKind": "http_status", "faultValue": "503", "probability": 0, "times": 1,
	})

	createSmsMessage(t, app)

	listRec := app.Do(t, "GET", "/admin/faults", nil, nil)
	var list struct {
		Content []map[string]any `json:"content"`
	}
	testutil.DecodeJSON(t, listRec, &list)
	if len(list.Content) != 1 {
		t.Fatalf("expected fault to remain unconsumed, got %d faults", len(list.Content))
	}
}

func TestFaultDelayDistributionUniformBounds(t *testing.T) {
	app := testutil.New(t, 0)

	app.Do(t, "PUT", "/admin/faults", nil, map[string]any{
		"provider": "zenvia", "routePattern": "/zenvia/channels/sms/messages",
		"faultKind": "delay_ms", "delayDistribution": `{"kind":"uniform","minMs":50,"maxMs":60}`,
	})

	start := time.Now()
	rec := createSmsMessage(t, app)
	elapsed := time.Since(start)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if elapsed < 45*time.Millisecond {
		t.Errorf("expected delay within uniform bounds, got %v", elapsed)
	}
}
