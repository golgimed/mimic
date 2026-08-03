package zenvia_test

import (
	"testing"
	"time"

	"github.com/golgimed/mimic/internal/testutil"
)

func TestCreateSmsMessage(t *testing.T) {
	app := testutil.New(t, 2*time.Second)

	rec := app.Do(t, "POST", "/zenvia/channels/sms/messages",
		map[string]string{"X-API-TOKEN": "test-token"},
		map[string]any{
			"from":     "sms-account",
			"to":       "55108888888888",
			"contents": []map[string]any{{"type": "text", "text": "Hi Zenvia!"}},
		},
	)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	testutil.DecodeJSON(t, rec, &body)

	if _, ok := body["id"].(string); !ok {
		t.Errorf("expected string id, got %v", body["id"])
	}
	if body["from"] != "sms-account" {
		t.Errorf("from = %v", body["from"])
	}
	if body["to"] != "55108888888888" {
		t.Errorf("to = %v", body["to"])
	}
	if body["direction"] != "OUT" {
		t.Errorf("direction = %v", body["direction"])
	}
	if body["channel"] != "sms" {
		t.Errorf("channel = %v", body["channel"])
	}
	if _, hasStatus := body["status"]; hasStatus {
		t.Errorf("status should be omitted, got %v", body["status"])
	}
}

func TestCreateTemplateSmsMessage(t *testing.T) {
	app := testutil.New(t, 2*time.Second)

	rec := app.Do(t, "POST", "/zenvia/channels/sms/messages",
		map[string]string{"X-API-TOKEN": "test-token"},
		map[string]any{
			"from": "sms-account",
			"to":   "55108888888888",
			"contents": []map[string]any{
				{"type": "template", "templateId": "template_id", "fields": map[string]string{"name": "Jhon"}},
			},
		},
	)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Contents []map[string]any `json:"contents"`
	}
	testutil.DecodeJSON(t, rec, &body)
	if body.Contents[0]["templateId"] != "template_id" {
		t.Errorf("templateId = %v", body.Contents[0]["templateId"])
	}
}

func TestRejectsWithoutAPIToken(t *testing.T) {
	app := testutil.New(t, 2*time.Second)

	rec := app.Do(t, "POST", "/zenvia/channels/sms/messages", nil, map[string]any{
		"from":     "sms-account",
		"to":       "55108888888888",
		"contents": []map[string]any{{"type": "text", "text": "Hi Zenvia!"}},
	})

	if rec.Code != 401 {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestRejectsInvalidPayload(t *testing.T) {
	app := testutil.New(t, 2*time.Second)

	rec := app.Do(t, "POST", "/zenvia/channels/sms/messages",
		map[string]string{"X-API-TOKEN": "test-token"},
		map[string]any{"from": "sms-account"},
	)

	if rec.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	testutil.DecodeJSON(t, rec, &body)
	if body["code"] != "VALIDATION_ERROR" {
		t.Errorf("code = %v", body["code"])
	}
	if _, ok := body["details"].([]any); !ok {
		t.Errorf("expected details array, got %v", body["details"])
	}
}
