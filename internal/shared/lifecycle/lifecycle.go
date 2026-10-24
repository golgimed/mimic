// Package lifecycle makes provider resource state transitions explicit: a
// Machine lists which state changes a resource type allows, and a Runner
// executes transitions — immediately, or later through the shared job
// scheduler. Providers keep owning their states and side effects (they
// supply Current and Apply); this package only checks and sequences them.
// It is deliberately not a workflow engine.
package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/golgimed/mimic/internal/shared/clock"
	"github.com/golgimed/mimic/internal/shared/scheduler"
	"github.com/golgimed/mimic/internal/shared/sim"
	"github.com/golgimed/mimic/internal/shared/timeline"
)

// ErrInvalidTransition is returned when a transition isn't allowed from the
// resource's current state.
var ErrInvalidTransition = errors.New("invalid state transition")

// ErrNotFound is returned when the resource doesn't exist.
var ErrNotFound = errors.New("resource not found")

// Machine is the set of allowed state changes for one resource type. The
// first target listed for a state is its default next state (see Next).
type Machine struct {
	Resource string
	edges    map[string][]string
}

// NewMachine builds a Machine from state -> allowed target states.
func NewMachine(resource string, edges map[string][]string) *Machine {
	return &Machine{Resource: resource, edges: edges}
}

// Can reports whether from -> to is allowed.
func (m *Machine) Can(from, to string) bool {
	for _, t := range m.edges[from] {
		if t == to {
			return true
		}
	}
	return false
}

// Next returns from's default next state, if any.
func (m *Machine) Next(from string) (string, bool) {
	if targets := m.edges[from]; len(targets) > 0 {
		return targets[0], true
	}
	return "", false
}

// Transition is one requested state change of one resource.
type Transition struct {
	ID    string        `json:"id"`
	From  string        `json:"from"`
	To    string        `json:"to"`
	After time.Duration `json:"after,omitempty"`
	// Meta carries provider-specific context for Apply (e.g. which
	// lifecycle path the resource follows). Optional.
	Meta map[string]string `json:"meta,omitempty"`
}

// CurrentFunc returns a resource's current state; ok is false if it
// doesn't exist.
type CurrentFunc func(ctx context.Context, id string) (state string, ok bool, err error)

// ApplyFunc persists t.To for the resource.
type ApplyFunc func(ctx context.Context, t Transition) error

// Runner executes transitions for one Machine.
type Runner struct {
	Machine  *Machine
	provider string
	jobKind  string
	sched    *scheduler.Scheduler
	current  CurrentFunc
	apply    ApplyFunc

	// OnChanged, if set, runs after a transition is persisted and
	// recorded: the provider's side effects (webhooks, scheduling the
	// following hop, ...).
	OnChanged ApplyFunc
}

// NewRunner builds a Runner and, if sched is non-nil, registers the
// "lifecycle:<provider>:<resource>" job handler used by Schedule.
func NewRunner(provider string, m *Machine, sched *scheduler.Scheduler, current CurrentFunc, apply ApplyFunc) *Runner {
	r := &Runner{
		Machine:  m,
		provider: provider,
		jobKind:  "lifecycle:" + provider + ":" + m.Resource,
		sched:    sched,
		current:  current,
		apply:    apply,
	}
	if sched != nil {
		sched.RegisterJobHandler(r.jobKind, r.handleJob)
	}
	return r
}

// Apply executes t now. t.From is checked against the resource's current
// state (ErrInvalidTransition if it moved on), then the edge is checked
// against the Machine. An empty t.From means "from whatever state it is in".
func (r *Runner) Apply(ctx context.Context, t Transition) error {
	cur, ok, err := r.current(ctx, t.ID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	if t.From == "" {
		t.From = cur
	}
	if cur != t.From || !r.Machine.Can(t.From, t.To) {
		return fmt.Errorf("%w: %s %s %s -> %s (current %s)", ErrInvalidTransition, r.Machine.Resource, t.ID, t.From, t.To, cur)
	}
	if err := r.apply(ctx, t); err != nil {
		return err
	}
	timeline.Record(ctx, timeline.Event{
		Type: timeline.StateChanged, Provider: r.provider, ResourceType: r.Machine.Resource, ResourceID: t.ID,
		Data: map[string]any{"from": t.From, "to": t.To},
	})
	slog.Debug("lifecycle: state changed", "provider", r.provider, "resource", r.Machine.Resource, "id", t.ID, "from", t.From, "to", t.To)
	if r.OnChanged != nil {
		return r.OnChanged(ctx, t)
	}
	return nil
}

// Schedule enqueues t on the job scheduler to run after t.After, scaled by
// the simulation profile's DelayScale.
func (r *Runner) Schedule(ctx context.Context, t Transition) error {
	if r.sched == nil {
		return errors.New("lifecycle: runner has no scheduler")
	}
	if !r.Machine.Can(t.From, t.To) {
		return fmt.Errorf("%w: %s %s %s -> %s", ErrInvalidTransition, r.Machine.Resource, t.ID, t.From, t.To)
	}
	return r.sched.ScheduleJob(ctx, r.jobKind, t, clock.Now().Add(sim.ScaleDelay(ctx, t.After)))
}

// handleJob runs a scheduled transition. A resource that has since been
// deleted or moved to another state makes the transition stale: it is
// skipped, not failed, since that is a normal race (e.g. a cancel landing
// before a scheduled delivery).
func (r *Runner) handleJob(ctx context.Context, raw json.RawMessage) error {
	var t Transition
	if err := json.Unmarshal(raw, &t); err != nil {
		return err
	}
	err := r.Apply(ctx, t)
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalidTransition) {
		slog.Debug("lifecycle: skipping stale scheduled transition", "provider", r.provider, "id", t.ID, "error", err)
		return nil
	}
	return err
}
