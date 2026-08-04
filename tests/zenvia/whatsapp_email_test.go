package zenvia_test

import (
	"testing"
	"time"

	"github.com/golgimed/mimic/internal/testutil"
)

func TestWhatsappTextMessage(t *testing.T) {
	app := testutil.New(t, 2*time.Second)

	rec := app.Do(t, "POST", "/zenvia/channels/whatsapp/messages",
		map[string]string{"X-API-TOKEN": "test-token"},
		map[string]any{
			"from":     "whatsapp-account",
			"to":       "55119999999999",
			"contents": []map[string]any{{"type": "text", "text": "Hello via WhatsApp"}},
		},
	)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	testutil.DecodeJSON(t, rec, &body)
	if body["channel"] != "whatsapp" {
		t.Errorf("channel = %v", body["channel"])
	}
}

func TestWhatsappFileMessage(t *testing.T) {
	app := testutil.New(t, 2*time.Second)

	rec := app.Do(t, "POST", "/zenvia/channels/whatsapp/messages",
		map[string]string{"X-API-TOKEN": "test-token"},
		map[string]any{
			"from":       "whatsapp-account",
			"to":         "55119999999999",
			"idRef":      "7390113b-e120-41b5-8a07-c4567726abc2",
			"contentRef": 0,
			"contents": []map[string]any{
				{"type": "file", "fileUrl": "https://example.com/file.pdf", "fileCaption": "Prescription"},
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
	if body.Contents[0]["fileUrl"] != "https://example.com/file.pdf" {
		t.Errorf("fileUrl = %v", body.Contents[0]["fileUrl"])
	}
}

func TestWhatsappRejectsUnknownContentType(t *testing.T) {
	app := testutil.New(t, 2*time.Second)

	rec := app.Do(t, "POST", "/zenvia/channels/whatsapp/messages",
		map[string]string{"X-API-TOKEN": "test-token"},
		map[string]any{
			"from":     "whatsapp-account",
			"to":       "55119999999999",
			"contents": []map[string]any{{"type": "sticker", "stickerId": "abc"}},
		},
	)

	if rec.Code != 400 {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestEmailMessage(t *testing.T) {
	app := testutil.New(t, 2*time.Second)

	rec := app.Do(t, "POST", "/zenvia/channels/email/messages",
		map[string]string{"X-API-TOKEN": "test-token"},
		map[string]any{
			"from":           "no-reply@example.com",
			"to":             "patient@example.com",
			"representative": map[string]any{"name": "GolgiMed"},
			"contents":       []map[string]any{{"type": "email", "subject": "Your results", "html": "<b>Hi!</b>"}},
		},
	)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Channel  string           `json:"channel"`
		Contents []map[string]any `json:"contents"`
	}
	testutil.DecodeJSON(t, rec, &body)
	if body.Channel != "email" {
		t.Errorf("channel = %v", body.Channel)
	}
	if body.Contents[0]["subject"] != "Your results" {
		t.Errorf("subject = %v", body.Contents[0]["subject"])
	}
}

func TestEmailTemplateMessage(t *testing.T) {
	app := testutil.New(t, 2*time.Second)

	rec := app.Do(t, "POST", "/zenvia/channels/email/messages",
		map[string]string{"X-API-TOKEN": "test-token"},
		map[string]any{
			"from":     "no-reply@example.com",
			"to":       "patient@example.com",
			"contents": []map[string]any{{"type": "template", "templateId": "welcome-email", "fields": map[string]string{"name": "Alex"}}},
		},
	)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestEmailRejectsMissingSubject(t *testing.T) {
	app := testutil.New(t, 2*time.Second)

	rec := app.Do(t, "POST", "/zenvia/channels/email/messages",
		map[string]string{"X-API-TOKEN": "test-token"},
		map[string]any{
			"from":     "no-reply@example.com",
			"to":       "patient@example.com",
			"contents": []map[string]any{{"type": "email", "html": "<b>Hi!</b>"}},
		},
	)

	if rec.Code != 400 {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
