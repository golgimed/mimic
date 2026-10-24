package zenvia

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"github.com/golgimed/mimic/internal/shared/admin"
	"github.com/golgimed/mimic/internal/shared/clock"
	"github.com/golgimed/mimic/internal/shared/lifecycle"
	"github.com/golgimed/mimic/internal/shared/scenario"
	"github.com/golgimed/mimic/internal/shared/scheduler"
	"github.com/golgimed/mimic/internal/shared/webhooks"
)

// messageLifecycle lists the message status changes Mimic simulates, using
// Zenvia's documented MESSAGE_STATUS codes (SENT, REJECTED, DELIVERED,
// NOT_DELIVERED, READ). ACCEPTED is Mimic's internal initial state.
var messageLifecycle = lifecycle.NewMachine("message", map[string][]string{
	"ACCEPTED":  {"SENT", "REJECTED"},
	"SENT":      {"DELIVERED", "NOT_DELIVERED"},
	"DELIVERED": {"READ"},
})

// defaultNext is the path a message follows when no scenario says
// otherwise: ACCEPTED -> SENT -> DELIVERED, one hop every statusDelay.
var defaultNext = map[string]string{
	"ACCEPTED": "SENT",
	"SENT":     "DELIVERED",
}

// RegisterScheduler wires the message lifecycle onto the job scheduler and
// returns a ScheduleAdvance closure handlers use to start a freshly created
// (ACCEPTED) message down its status path.
func RegisterScheduler(sched *scheduler.Scheduler, db *sql.DB, store *Store, faultStore *admin.Store, statusDelay time.Duration) func(ctx context.Context, messageID string) error {
	var runner *lifecycle.Runner
	// scheduleNext follows the simulation's scenario if one applies to
	// messages, else the default path.
	scheduleNext := func(ctx context.Context, messageID, from string) error {
		t := lifecycle.Transition{ID: messageID, From: from, After: statusDelay}
		if sc, ok := scenario.Active(ctx, Name, "message"); ok {
			step, ok := sc.Next(from)
			if !ok {
				return nil
			}
			t.To, t.Meta = step.To, step.Meta
			if step.After != nil {
				t.After = time.Duration(*step.After)
			}
		} else if next, ok := defaultNext[from]; ok {
			t.To = next
		} else {
			return nil
		}
		return runner.Schedule(ctx, t)
	}

	current := func(_ context.Context, id string) (string, bool, error) {
		message, err := store.GetMessage(id)
		if err != nil || message == nil {
			return "", false, err
		}
		return message.Status, true, nil
	}
	apply := func(_ context.Context, t lifecycle.Transition) error {
		return store.UpdateMessageStatus(t.ID, t.To)
	}
	runner = lifecycle.NewRunner(Name, messageLifecycle, sched, current, apply)
	runner.OnChanged = func(ctx context.Context, t lifecycle.Transition) error {
		if err := notifyStatus(ctx, db, store, faultStore, t); err != nil {
			return err
		}
		return scheduleNext(ctx, t.ID, t.To)
	}

	return func(ctx context.Context, messageID string) error { return scheduleNext(ctx, messageID, "ACCEPTED") }
}

// notifyStatus notifies every subscription matching the message's channel
// of its new status with a MESSAGE_STATUS event.
func notifyStatus(ctx context.Context, db *sql.DB, store *Store, faultStore *admin.Store, t lifecycle.Transition) error {
	message, err := store.GetMessage(t.ID)
	if err != nil || message == nil {
		return err
	}

	subscriptions, err := store.FindActiveSubscriptionsForChannel(message.Channel)
	if err != nil {
		return err
	}
	for _, sub := range subscriptions {
		if err := notifySubscription(ctx, db, faultStore, sub, message, t); err != nil {
			return err
		}
	}
	return nil
}

// notifySubscription delivers (or fault-injects) one MESSAGE_STATUS webhook
// event to sub, unless sub's direction criteria excludes message.
func notifySubscription(ctx context.Context, db *sql.DB, faultStore *admin.Store, sub Subscription, message *Message, t lifecycle.Transition) error {
	if sub.CriteriaDirection != nil && *sub.CriteriaDirection != "ALL" && *sub.CriteriaDirection != message.Direction {
		return nil
	}

	eventPayload := statusEventPayload(sub, message, t)

	webhookFault, err := faultStore.ConsumeMatchingFault(ctx, Name, "webhook")
	if err != nil {
		return err
	}

	if webhookFault != nil && webhookFault.FaultKind == admin.FaultWebhookDropped {
		return webhooks.LogDropped(ctx, db, webhooks.DeliverInput{
			Provider: Name, ResourceType: "message", ResourceID: message.ID,
			URL: sub.WebhookURL, Payload: eventPayload,
		})
	}

	if webhookFault != nil && webhookFault.FaultKind == admin.FaultWebhookInvalid {
		delete(eventPayload, "messageStatus")
		eventPayload["malformed"] = true
	}

	return webhooks.Deliver(ctx, db, webhooks.DeliverInput{
		Provider: Name, ResourceType: "message", ResourceID: message.ID,
		URL: sub.WebhookURL, Headers: sub.WebhookHeaders, Payload: eventPayload,
	})
}

// statusEventPayload builds a MESSAGE_STATUS event. A scenario step's meta
// (description, reason, details, channelErrorCode) fills the documented
// messageStatus.description and messageStatus.causes fields.
func statusEventPayload(sub Subscription, message *Message, t lifecycle.Transition) map[string]any {
	status := map[string]any{
		"code":      t.To,
		"timestamp": clock.Now().Format(time.RFC3339Nano),
		"channel":   message.Channel,
		"direction": message.Direction,
	}
	if d := t.Meta["description"]; d != "" {
		status["description"] = d
	}
	cause := map[string]any{}
	for _, k := range []string{"reason", "details", "channelErrorCode"} {
		if v := t.Meta[k]; v != "" {
			cause[k] = v
		}
	}
	if len(cause) > 0 {
		status["causes"] = []any{cause}
	}
	return map[string]any{
		"id":             uuid.NewString(),
		"timestamp":      clock.Now().Format(time.RFC3339Nano),
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
		"messageStatus": status,
	}
}
