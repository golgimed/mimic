package zenvia

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"github.com/golgimed/mimic/internal/shared/admin"
	"github.com/golgimed/mimic/internal/shared/lifecycle"
	"github.com/golgimed/mimic/internal/shared/scheduler"
	"github.com/golgimed/mimic/internal/shared/webhooks"
)

// messageLifecycle is a Zenvia message's status path. ACCEPTED -> SENT ->
// DELIVERED is the default (first-listed) path; each hop runs statusDelay
// after the previous one.
var messageLifecycle = lifecycle.NewMachine("message", map[string][]string{
	"ACCEPTED": {"SENT"},
	"SENT":     {"DELIVERED"},
})

// RegisterScheduler wires the message lifecycle onto the job scheduler and
// returns a ScheduleAdvance closure handlers use to start a freshly created
// (ACCEPTED) message down its status path.
func RegisterScheduler(sched *scheduler.Scheduler, db *sql.DB, store *Store, faultStore *admin.Store, statusDelay time.Duration) func(messageID string) error {
	var runner *lifecycle.Runner
	scheduleNext := func(messageID, from string) error {
		next, ok := messageLifecycle.Next(from)
		if !ok {
			return nil
		}
		return runner.Schedule(lifecycle.Transition{ID: messageID, From: from, To: next, After: statusDelay})
	}

	current := func(_ context.Context, id string) (string, bool, error) {
		message, err := store.GetMessage(id)
		if err != nil || message == nil {
			return "", false, err
		}
		return message.Status, true, nil
	}
	apply := func(ctx context.Context, t lifecycle.Transition) error {
		if err := advanceMessage(ctx, db, store, faultStore, t); err != nil {
			return err
		}
		return scheduleNext(t.ID, t.To)
	}
	runner = lifecycle.NewRunner(Name, messageLifecycle, sched, current, apply)

	return func(messageID string) error { return scheduleNext(messageID, "ACCEPTED") }
}

// advanceMessage persists one status hop and notifies every matching
// subscription with a MESSAGE_STATUS event.
func advanceMessage(ctx context.Context, db *sql.DB, store *Store, faultStore *admin.Store, t lifecycle.Transition) error {
	if err := store.UpdateMessageStatus(t.ID, t.To); err != nil {
		return err
	}
	message, err := store.GetMessage(t.ID)
	if err != nil || message == nil {
		return err
	}

	subscriptions, err := store.FindActiveSubscriptionsForChannel(message.Channel)
	if err != nil {
		return err
	}
	for _, sub := range subscriptions {
		if err := notifySubscription(ctx, db, faultStore, sub, message, t.To); err != nil {
			return err
		}
	}
	return nil
}

// notifySubscription delivers (or fault-injects) one MESSAGE_STATUS webhook
// event to sub, unless sub's direction criteria excludes message.
func notifySubscription(ctx context.Context, db *sql.DB, faultStore *admin.Store, sub Subscription, message *Message, next string) error {
	if sub.CriteriaDirection != nil && *sub.CriteriaDirection != "ALL" && *sub.CriteriaDirection != message.Direction {
		return nil
	}

	eventPayload := statusEventPayload(sub, message, next)

	webhookFault, err := faultStore.ConsumeMatchingFault(Name, "webhook")
	if err != nil {
		return err
	}

	if webhookFault != nil && webhookFault.FaultKind == admin.FaultWebhookDropped {
		return webhooks.LogDropped(db, webhooks.DeliverInput{
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

func statusEventPayload(sub Subscription, message *Message, next string) map[string]any {
	return map[string]any{
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
}
