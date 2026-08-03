package zenvia

import (
	"encoding/json"
	"net/http"

	"github.com/golgimed/mimic/internal/shared/httpx"
)

func writeValidationError(w http.ResponseWriter) {
	httpx.WriteJSON(w, http.StatusBadRequest, map[string]any{
		"code":    "VALIDATION_ERROR",
		"message": "Validation error",
		"details": []map[string]string{{"path": "body", "message": "invalid or missing fields"}},
	})
}

type messageResponse struct {
	ID         string  `json:"id"`
	ExternalID *string `json:"externalId,omitempty"`
	From       string  `json:"from"`
	To         string  `json:"to"`
	Direction  string  `json:"direction"`
	Channel    string  `json:"channel"`
	Contents   []any   `json:"contents"`
	Timestamp  string  `json:"timestamp"`
}

func toMessageResponse(m *Message) messageResponse {
	return messageResponse{
		ID: m.ID, ExternalID: m.ExternalID, From: m.From, To: m.To,
		Direction: m.Direction, Channel: m.Channel, Contents: m.Contents, Timestamp: m.CreatedAt,
	}
}

func createMessageHandler(channel string, store *Store, scheduleAdvance func(string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var wire createMessageWire
		if err := json.NewDecoder(r.Body).Decode(&wire); err != nil || !wire.valid(channel) {
			writeValidationError(w)
			return
		}

		message, err := store.CreateMessage(channel, wire.ExternalID, wire.From, wire.To, wire.toContents())
		if err != nil {
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"code": "INTERNAL_ERROR", "message": err.Error()})
			return
		}

		if err := scheduleAdvance(message.ID); err != nil {
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"code": "INTERNAL_ERROR", "message": err.Error()})
			return
		}

		httpx.WriteJSON(w, http.StatusOK, toMessageResponse(message))
	}
}

type subscriptionResponse struct {
	ID        string `json:"id"`
	EventType string `json:"eventType"`
	Webhook   struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers,omitempty"`
	} `json:"webhook"`
	Criteria struct {
		Channel   string  `json:"channel"`
		Direction *string `json:"direction,omitempty"`
	} `json:"criteria"`
	Status    string `json:"status"`
	Version   string `json:"version"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func toSubscriptionResponse(s *Subscription) subscriptionResponse {
	var resp subscriptionResponse
	resp.ID = s.ID
	resp.EventType = s.EventType
	resp.Webhook.URL = s.WebhookURL
	resp.Webhook.Headers = s.WebhookHeaders
	resp.Criteria.Channel = s.CriteriaChannel
	resp.Criteria.Direction = s.CriteriaDirection
	resp.Status = s.Status
	resp.Version = "v2"
	resp.CreatedAt = s.CreatedAt
	resp.UpdatedAt = s.UpdatedAt
	return resp
}

func createSubscriptionHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var wire createSubscriptionWire
		if err := json.NewDecoder(r.Body).Decode(&wire); err != nil || !wire.valid() {
			writeValidationError(w)
			return
		}

		sub, err := store.CreateSubscription(CreateSubscriptionInput{
			EventType:         wire.EventType,
			WebhookURL:        wire.Webhook.URL,
			WebhookHeaders:    wire.Webhook.Headers,
			CriteriaChannel:   wire.Criteria.Channel,
			CriteriaDirection: wire.Criteria.Direction,
		})
		if err != nil {
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"code": "INTERNAL_ERROR", "message": err.Error()})
			return
		}

		httpx.WriteJSON(w, http.StatusOK, toSubscriptionResponse(sub))
	}
}

func listSubscriptionsHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subs, err := store.ListSubscriptions()
		if err != nil {
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"code": "INTERNAL_ERROR", "message": err.Error()})
			return
		}
		content := make([]subscriptionResponse, len(subs))
		for i := range subs {
			content[i] = toSubscriptionResponse(&subs[i])
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"content": content})
	}
}

func getSubscriptionHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sub, err := store.GetSubscription(r.PathValue("subscriptionId"))
		if err != nil {
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"code": "INTERNAL_ERROR", "message": err.Error()})
			return
		}
		if sub == nil {
			httpx.WriteJSON(w, http.StatusNotFound, map[string]string{"code": "NOT_FOUND", "message": "Subscription not found"})
			return
		}
		httpx.WriteJSON(w, http.StatusOK, toSubscriptionResponse(sub))
	}
}

func deleteSubscriptionHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deleted, err := store.DeleteSubscription(r.PathValue("subscriptionId"))
		if err != nil {
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]string{"code": "INTERNAL_ERROR", "message": err.Error()})
			return
		}
		if !deleted {
			httpx.WriteJSON(w, http.StatusNotFound, map[string]string{"code": "NOT_FOUND", "message": "Subscription not found"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
