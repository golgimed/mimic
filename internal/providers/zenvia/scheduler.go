package zenvia

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/golgimed/mimic/internal/shared/admin"
	"github.com/golgimed/mimic/internal/shared/scheduler"
	"github.com/golgimed/mimic/internal/shared/webhooks"
)

const advanceJobKind = "zenvia:advance"

var nextStatus = map[string]string{
	"ACCEPTED": "SENT",
	"SENT":     "DELIVERED",
}

type advanceJobPayload struct {
	MessageID string `json:"messageId"`
}

// RegisterScheduler wires the zenvia:advance job handler and returns a
// ScheduleAdvance closure providers/handlers use to enqueue the next hop
// (ACCEPTED -> SENT -> DELIVERED) for a freshly created message.
func RegisterScheduler(sched *scheduler.Scheduler, db *sql.DB, store *Store, faultStore *admin.Store, statusDelay time.Duration) func(messageID string) error {
	scheduleAdvance := func(messageID string) error {
		return sched.ScheduleJob(advanceJobKind, advanceJobPayload{MessageID: messageID}, time.Now().Add(statusDelay))
	}

	sched.RegisterJobHandler(advanceJobKind, func(ctx context.Context, raw json.RawMessage) error {
		var payload advanceJobPayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			return err
		}

		message, err := store.GetMessage(payload.MessageID)
		if err != nil || message == nil {
			return err
		}

		next, ok := nextStatus[message.Status]
		if !ok {
			return nil
		}

		if err := store.UpdateMessageStatus(payload.MessageID, next); err != nil {
			return err
		}

		subscriptions, err := store.FindActiveSubscriptionsForChannel(message.Channel)
		if err != nil {
			return err
		}

		for _, sub := range subscriptions {
			if sub.CriteriaDirection != nil && *sub.CriteriaDirection != "ALL" && *sub.CriteriaDirection != message.Direction {
				continue
			}

			eventPayload := map[string]any{
				"id":             uuid.NewString(),
				"timestamp":      time.Now().UTC().Format(time.RFC3339Nano),
				"subscriptionId": sub.ID,
				"type":           "MESSAGE_STATUS",
				"channel":        message.Channel,
				"message": map[string]any{
					"id":         message.ID,
					"externalId": message.ExternalID,
					"direction":  message.Direction,
					"from":       message.From,
					"to":         message.To,
				},
				"messageStatus": map[string]any{
					"code":      next,
					"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
					"channel":   message.Channel,
					"direction": message.Direction,
				},
			}

			webhookFault, err := faultStore.ConsumeMatchingFault(Name, "webhook")
			if err != nil {
				return err
			}

			if webhookFault != nil && webhookFault.FaultKind == admin.FaultWebhookDropped {
				if err := webhooks.LogDropped(db, webhooks.DeliverInput{
					Provider: Name, ResourceType: "message", ResourceID: message.ID,
					URL: sub.WebhookURL, Payload: eventPayload,
				}); err != nil {
					return err
				}
				continue
			}

			if webhookFault != nil && webhookFault.FaultKind == admin.FaultWebhookInvalid {
				delete(eventPayload, "messageStatus")
				eventPayload["malformed"] = true
			}

			if err := webhooks.Deliver(ctx, db, webhooks.DeliverInput{
				Provider: Name, ResourceType: "message", ResourceID: message.ID,
				URL: sub.WebhookURL, Headers: sub.WebhookHeaders, Payload: eventPayload,
			}); err != nil {
				return err
			}
		}

		if _, hasNext := nextStatus[next]; hasNext {
			return scheduleAdvance(payload.MessageID)
		}
		return nil
	})

	return scheduleAdvance
}
