package clock

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeWall is a controllable wall clock for exercising rates.
type fakeWall struct{ t time.Time }

func (f *fakeWall) now() time.Time { return f.t }

func withWall(c *Clock, f *fakeWall) *Clock {
	c.now = f.now
	c.wallBase = f.t
	c.simBase = f.t
	return c
}

func TestAcceleratedRunsFaster(t *testing.T) {
	wall := &fakeWall{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	c, err := New(Accelerated, 60)
	if err != nil {
		t.Fatal(err)
	}
	withWall(c, wall)
	wall.t = wall.t.Add(time.Second)
	if got := c.Now().Sub(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); got != time.Minute {
		t.Fatalf("1 real second = %v simulated, want 1m", got)
	}
}

func TestVirtualOnlyMovesWhenSet(t *testing.T) {
	wall := &fakeWall{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	c, _ := New(Virtual, 1)
	withWall(c, wall)
	start := c.Now()
	wall.t = wall.t.Add(time.Hour)
	if !c.Now().Equal(start) {
		t.Fatal("virtual time moved with the wall clock")
	}
	c.set(start.Add(5 * time.Minute))
	if got := c.Now().Sub(start); got != 5*time.Minute {
		t.Fatalf("after set: %v, want 5m", got)
	}
	c.set(start) // never backwards
	if got := c.Now().Sub(start); got != 5*time.Minute {
		t.Fatalf("clock moved backwards to %v", got)
	}
}

func TestSleepIsSkippedOnVirtualClock(t *testing.T) {
	c, _ := New(Virtual, 1)
	begin := time.Now()
	c.Sleep(context.Background(), time.Hour)
	if time.Since(begin) > time.Second {
		t.Fatal("virtual Sleep blocked")
	}
}

func TestAdvanceRunsRunnerInOrderAndRejectsRealtime(t *testing.T) {
	t.Cleanup(func() { Set(mustNew(Realtime, 1)); SetRunner(nil) })

	Set(mustNew(Realtime, 1))
	if _, err := Advance(context.Background(), time.Minute); !errors.Is(err, ErrRealtime) {
		t.Fatalf("realtime Advance err = %v, want ErrRealtime", err)
	}

	c, _ := New(Virtual, 1)
	Set(c)
	start := c.Now()
	var seen []time.Duration
	SetRunner(func(_ context.Context, target time.Time, step func(time.Time)) error {
		for _, due := range []time.Duration{2 * time.Second, 4 * time.Second} {
			step(start.Add(due))
			seen = append(seen, Now().Sub(start))
		}
		return nil
	})
	now, err := Advance(context.Background(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || seen[0] != 2*time.Second || seen[1] != 4*time.Second {
		t.Fatalf("work observed times %v, want [2s 4s]", seen)
	}
	if got := now.Sub(start); got != time.Minute {
		t.Fatalf("advanced to %v, want 1m", got)
	}
}

func TestParseMode(t *testing.T) {
	if m, err := ParseMode(""); err != nil || m != Realtime {
		t.Fatalf("default = %v, %v", m, err)
	}
	if _, err := ParseMode("sideways"); err == nil {
		t.Fatal("expected error")
	}
}
