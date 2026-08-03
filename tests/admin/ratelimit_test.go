package admin_test

import (
	"testing"
	"time"

	"github.com/golgimed/mimic/internal/testutil"
)

func TestFaultRateLimitedBlocksOverBudget(t *testing.T) {
	app := testutil.New(t, 0)

	app.Do(t, "PUT", "/admin/faults", nil, map[string]any{
		"provider": "zenvia", "routePattern": "/zenvia/channels/sms/messages",
		"faultKind": "rate_limited", "faultValue": "3/500ms",
	})

	for i := 0; i < 3; i++ {
		rec := createSmsMessage(t, app)
		if rec.Code != 200 {
			t.Fatalf("request %d: expected 200, got %d", i+1, rec.Code)
		}
	}

	for i := 0; i < 2; i++ {
		rec := createSmsMessage(t, app)
		if rec.Code != 429 {
			t.Fatalf("request over budget: expected 429, got %d", rec.Code)
		}
		if rec.Header().Get("Retry-After") == "" {
			t.Errorf("expected Retry-After header to be set")
		}
	}
}

func TestFaultRateLimitedResetsAfterWindow(t *testing.T) {
	app := testutil.New(t, 0)

	app.Do(t, "PUT", "/admin/faults", nil, map[string]any{
		"provider": "zenvia", "routePattern": "/zenvia/channels/sms/messages",
		"faultKind": "rate_limited", "faultValue": "1/200ms",
	})

	first := createSmsMessage(t, app)
	if first.Code != 200 {
		t.Fatalf("expected first request 200, got %d", first.Code)
	}
	second := createSmsMessage(t, app)
	if second.Code != 429 {
		t.Fatalf("expected second request 429, got %d", second.Code)
	}

	time.Sleep(220 * time.Millisecond)

	third := createSmsMessage(t, app)
	if third.Code != 200 {
		t.Fatalf("expected request after window reset to be 200, got %d", third.Code)
	}
}
