package bryscad

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/golgimed/mimic/internal/shared/lifecycle"
	"github.com/golgimed/mimic/internal/shared/scenario"
	"github.com/golgimed/mimic/internal/shared/scheduler"
	"github.com/golgimed/mimic/internal/shared/webhooks"
)

// collectionLifecycle: a collection starts PENDENTE and ends in exactly one
// terminal state. CONCLUIDO is reached through the Mimic-only /concluir
// helper; CANCELADO and REJEITADA through /cancelar and /rejeitar.
var collectionLifecycle = lifecycle.NewMachine("coleta", map[string][]string{
	"PENDENTE": {"CONCLUIDO", "CANCELADO", "REJEITADA"},
})

// newCollectionRunner builds the collection lifecycle runner. Scenario
// transitions (scheduled by startScenario) deliver the completion webhook
// themselves; the /concluir helper delivers it after responding.
func newCollectionRunner(store *Store, sched *scheduler.Scheduler, db *sql.DB, webhookURL string) *lifecycle.Runner {
	current := func(_ context.Context, chave string) (string, bool, error) {
		c, err := store.GetCollection(chave)
		if err != nil || c == nil {
			return "", false, err
		}
		return c.Situacao, true, nil
	}
	apply := func(_ context.Context, t lifecycle.Transition) error {
		var err error
		if t.To == "CONCLUIDO" {
			_, err = store.CompleteCollection(t.ID)
		} else {
			_, err = store.TransitionCollection(t.ID, t.To)
		}
		return err
	}
	runner := lifecycle.NewRunner(Name, collectionLifecycle, sched, current, apply)
	runner.OnChanged = func(ctx context.Context, t lifecycle.Transition) error {
		if t.Meta["source"] == "scenario" && t.To == "CONCLUIDO" {
			deliverCompletionWebhook(ctx, db, webhookURL, t.ID)
		}
		return scheduleScenarioStep(ctx, runner, t.ID, t.To)
	}
	return runner
}

// startScenario starts a new collection down the simulation's scenario
// path, if one applies to collections.
func startScenario(ctx context.Context, runner *lifecycle.Runner, chave string) error {
	return scheduleScenarioStep(ctx, runner, chave, "PENDENTE")
}

// scheduleScenarioStep schedules the scenario step leaving from, if any.
// A step without "after" runs on the next scheduler tick.
func scheduleScenarioStep(ctx context.Context, runner *lifecycle.Runner, chave, from string) error {
	sc, ok := scenario.Active(ctx, Name, "coleta")
	if !ok {
		return nil
	}
	step, ok := sc.Next(from)
	if !ok {
		return nil
	}
	t := lifecycle.Transition{ID: chave, From: from, To: step.To, Meta: map[string]string{"source": "scenario"}}
	if step.After != nil {
		t.After = time.Duration(*step.After)
	}
	return runner.Schedule(ctx, t)
}

// deliverCompletionWebhook POSTs BRy's completion webhook to webhookURL
// (no-op when unset). Matches golgimed's webhookPayload shape
// (bry_adapter.go): only the collection key is read from the body,
// everything else is re-verified.
func deliverCompletionWebhook(ctx context.Context, db *sql.DB, webhookURL, chave string) {
	if webhookURL == "" {
		return
	}
	if err := webhooks.Deliver(context.WithoutCancel(ctx), db, webhooks.DeliverInput{
		Provider:     Name,
		ResourceType: "coleta",
		ResourceID:   chave,
		URL:          webhookURL,
		Payload:      map[string]any{"payload": map[string]any{"chave": chave}},
	}); err != nil {
		// The collection key is client-supplied, so it stays out of the log
		// line; the timeline's WebhookFailed event carries it.
		slog.Error("bry-scad: deliver completion webhook", "error", err)
	}
}

// writeTransitionError maps a lifecycle error onto SCAD's error envelope.
// The 409 for a collection that already left PENDENTE is a Mimic
// assumption: BRy's documentation doesn't specify the response for acting
// on a finished collection.
func writeTransitionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, lifecycle.ErrNotFound):
		writeError(w, http.StatusNotFound, "coleta_nao_encontrada", "Coleta não encontrada.")
	case errors.Is(err, lifecycle.ErrInvalidTransition):
		writeError(w, http.StatusConflict, "coleta_situacao_invalida", "A coleta não está pendente.")
	default:
		writeError(w, http.StatusInternalServerError, "erro_interno", err.Error())
	}
}
