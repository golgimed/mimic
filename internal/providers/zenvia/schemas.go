package zenvia

import "net/url"

// Contract taken from docs/vendor/zenvia-openapi-v2.json:
// components.schemas["message.base"], ["content.base"], ["content.text"],
// ["content.template"], ["content.file"], ["content.email"],
// ["subscription.base"], ["subscription.partial-status.message-status-subscription"]

var contentTypesByChannel = map[string]map[string]bool{
	"sms":      {"text": true, "template": true},
	"whatsapp": {"text": true, "template": true, "file": true},
	"email":    {"email": true, "template": true},
}

type createMessageWire struct {
	ExternalID *string          `json:"externalId"`
	From       string           `json:"from"`
	To         string           `json:"to"`
	Contents   []map[string]any `json:"contents"`
	IDRef      *string          `json:"idRef,omitempty"`
	ContentRef *float64         `json:"contentRef,omitempty"`
}

func isString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

func validateContentItem(channel string, item map[string]any) bool {
	typ, ok := isString(item["type"])
	if !ok || !contentTypesByChannel[channel][typ] {
		return false
	}

	switch typ {
	case "text":
		_, ok := isString(item["text"])
		return ok
	case "template":
		_, ok := isString(item["templateId"])
		return ok
	case "file":
		fileURL, ok := isString(item["fileUrl"])
		if !ok {
			return false
		}
		_, err := url.ParseRequestURI(fileURL)
		return err == nil
	case "email":
		subject, ok := isString(item["subject"])
		return ok && subject != ""
	default:
		return false
	}
}

func (w createMessageWire) valid(channel string) bool {
	if w.From == "" || len(w.From) > 64 {
		return false
	}
	if w.To == "" || len(w.To) > 64 {
		return false
	}
	if len(w.Contents) == 0 {
		return false
	}
	for _, item := range w.Contents {
		if !validateContentItem(channel, item) {
			return false
		}
	}
	return true
}

func (w createMessageWire) toContents() []any {
	out := make([]any, len(w.Contents))
	for i, c := range w.Contents {
		out[i] = c
	}
	return out
}

type createSubscriptionWire struct {
	EventType string `json:"eventType"`
	Webhook   struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	} `json:"webhook"`
	Criteria struct {
		Channel   string  `json:"channel"`
		Direction *string `json:"direction"`
	} `json:"criteria"`
}

func (w createSubscriptionWire) valid() bool {
	if w.EventType != "MESSAGE_STATUS" {
		return false
	}
	if w.Webhook.URL == "" {
		return false
	}
	if _, err := url.ParseRequestURI(w.Webhook.URL); err != nil {
		return false
	}
	if w.Criteria.Channel == "" {
		return false
	}
	if w.Criteria.Direction != nil {
		d := *w.Criteria.Direction
		if d != "IN" && d != "OUT" && d != "ALL" {
			return false
		}
	}
	return true
}
