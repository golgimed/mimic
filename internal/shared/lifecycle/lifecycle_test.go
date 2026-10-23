package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golgimed/mimic/internal/shared/scheduler"
	"github.com/golgimed/mimic/internal/shared/storage"
)

var docLifecycle = NewMachine("doc", map[string][]string{
	"PENDING": {"SIGNED", "REJECTED"},
})

// memState is a toy provider: an in-memory id -> state map.
type memState map[string]string

func (m memState) current(_ context.Context, id string) (string, bool, error) {
	s, ok := m[id]
	return s, ok, nil
}

func (m memState) apply(_ context.Context, t Transition) error {
	m[t.ID] = t.To
	return nil
}

func TestMachine(t *testing.T) {
	if !docLifecycle.Can("PENDING", "REJECTED") || docLifecycle.Can("SIGNED", "PENDING") {
		t.Fatal("Can reports wrong edges")
	}
	if next, ok := docLifecycle.Next("PENDING"); !ok || next != "SIGNED" {
		t.Fatalf("Next(PENDING) = %q, %v; want SIGNED (first listed)", next, ok)
	}
	if _, ok := docLifecycle.Next("SIGNED"); ok {
		t.Fatal("terminal state has a next state")
	}
}

func TestApply(t *testing.T) {
	state := memState{"d1": "PENDING"}
	r := NewRunner("test", docLifecycle, nil, state.current, state.apply)
	ctx := context.Background()

	if err := r.Apply(ctx, Transition{ID: "d1", To: "SIGNED"}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if state["d1"] != "SIGNED" {
		t.Fatalf("state = %q, want SIGNED", state["d1"])
	}
	if err := r.Apply(ctx, Transition{ID: "d1", To: "REJECTED"}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("SIGNED -> REJECTED err = %v, want ErrInvalidTransition", err)
	}
	if err := r.Apply(ctx, Transition{ID: "nope", To: "SIGNED"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id err = %v, want ErrNotFound", err)
	}
}

func TestScheduledTransitionRunsOnTickAndSkipsStale(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := storage.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	sched := scheduler.New(db)
	state := memState{"d1": "PENDING", "d2": "PENDING", "d3": "PENDING"}
	r := NewRunner("test", docLifecycle, sched, state.current, state.apply)
	ctx := context.Background()

	for _, tr := range []Transition{
		{ID: "d1", From: "PENDING", To: "SIGNED"},
		{ID: "d2", From: "PENDING", To: "SIGNED", After: time.Hour},
		{ID: "d3", From: "PENDING", To: "REJECTED"},
	} {
		if err := r.Schedule(tr); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Schedule(Transition{ID: "d1", From: "SIGNED", To: "PENDING"}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("scheduling a disallowed edge err = %v", err)
	}

	// d3 was moved by something else before its job ran: stale, skipped.
	state["d3"] = "SIGNED"
	if err := sched.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if state["d1"] != "SIGNED" || state["d2"] != "PENDING" || state["d3"] != "SIGNED" {
		t.Fatalf("state = %v, want d1 SIGNED, d2 not yet due, d3 untouched", state)
	}

	var failed int
	if err := db.QueryRow("SELECT COUNT(*) FROM jobs WHERE status = 'failed'").Scan(&failed); err != nil || failed != 0 {
		t.Fatalf("failed jobs = %d (err %v), want stale transitions marked done", failed, err)
	}
}
