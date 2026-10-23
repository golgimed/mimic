// Package clock is the simulation's time source. Simulation behavior —
// scheduled jobs, state-transition delays, token expiry, rate-limit
// windows, provider timestamps, simulated latency — reads time from here
// instead of the system clock, so time can run:
//
//   - realtime (default): simulation time is wall-clock time;
//   - accelerated: simulation time runs Speed times faster than wall-clock;
//   - virtual: simulation time only moves when advanced explicitly
//     (POST /admin/clock/advance), which then runs every job that falls due.
package clock

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Mode is how simulation time relates to wall-clock time.
type Mode string

const (
	Realtime    Mode = "realtime"
	Accelerated Mode = "accelerated"
	Virtual     Mode = "virtual"
)

// ParseMode parses MIMIC_CLOCK.
func ParseMode(s string) (Mode, error) {
	switch m := Mode(strings.ToLower(strings.TrimSpace(s))); m {
	case "":
		return Realtime, nil
	case Realtime, Accelerated, Virtual:
		return m, nil
	default:
		return "", fmt.Errorf("clock mode must be realtime, accelerated or virtual, got %q", s)
	}
}

// ErrRealtime is returned when advancing a realtime clock.
var ErrRealtime = errors.New("the realtime clock cannot be advanced; set MIMIC_CLOCK=virtual or accelerated")

// Clock is a simulation clock.
type Clock struct {
	mu    sync.Mutex
	mode  Mode
	speed float64
	// Simulation time is simBase + (wall - wallBase) * rate, where rate is
	// 1 (realtime), speed (accelerated) or 0 (virtual).
	wallBase time.Time
	simBase  time.Time
	now      func() time.Time
}

// New returns a clock in mode; speed is used by Accelerated (must be > 0).
func New(mode Mode, speed float64) (*Clock, error) {
	if mode == Accelerated && speed <= 0 {
		return nil, fmt.Errorf("accelerated clock speed must be > 0, got %v", speed)
	}
	if mode != Accelerated {
		speed = 1
	}
	now := time.Now().UTC()
	return &Clock{mode: mode, speed: speed, wallBase: now, simBase: now, now: time.Now}, nil
}

func (c *Clock) rate() float64 {
	switch c.mode {
	case Virtual:
		return 0
	case Accelerated:
		return c.speed
	default:
		return 1
	}
}

// Now returns the current simulation time (UTC).
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.nowLocked()
}

func (c *Clock) nowLocked() time.Time {
	if c.mode == Realtime {
		return c.now().UTC()
	}
	elapsed := c.now().Sub(c.wallBase)
	return c.simBase.Add(time.Duration(float64(elapsed) * c.rate())).UTC()
}

// Mode returns the clock's mode; Speed its acceleration factor.
func (c *Clock) Mode() Mode     { return c.mode }
func (c *Clock) Speed() float64 { return c.speed }

// set moves simulation time to t (never backwards).
func (c *Clock) set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !t.After(c.nowLocked()) {
		return
	}
	c.wallBase = c.now()
	c.simBase = t.UTC()
}

// Sleep waits d of simulation time: d on a realtime clock, d/speed on an
// accelerated one, and not at all on a virtual one (simulated latency
// would otherwise never elapse). Respects ctx cancellation.
func (c *Clock) Sleep(ctx context.Context, d time.Duration) {
	if d <= 0 || c.mode == Virtual {
		return
	}
	timer := time.NewTimer(time.Duration(float64(d) / c.rate()))
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}

// Runner executes simulated work that falls due while the clock is
// advanced: given a target time, it repeatedly runs the earliest due work
// and moves the clock to that work's due time first (via Step), so work
// observes the time it was due at. The job scheduler registers itself.
type Runner func(ctx context.Context, target time.Time, step func(time.Time)) error

var (
	defaultMu sync.RWMutex
	current   = mustNew(Realtime, 1)
	runner    Runner
)

func mustNew(mode Mode, speed float64) *Clock {
	c, err := New(mode, speed)
	if err != nil {
		panic(err)
	}
	return c
}

// Set installs c as the process-wide simulation clock (at startup, or per
// test).
func Set(c *Clock) {
	defaultMu.Lock()
	current = c
	defaultMu.Unlock()
}

// Get returns the process-wide simulation clock.
func Get() *Clock {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return current
}

// Now returns the process-wide simulation time.
func Now() time.Time { return Get().Now() }

// Sleep sleeps d of process-wide simulation time.
func Sleep(ctx context.Context, d time.Duration) { Get().Sleep(ctx, d) }

// SetRunner registers the work runner Advance drives.
func SetRunner(r Runner) {
	defaultMu.Lock()
	runner = r
	defaultMu.Unlock()
}

// Advance moves the process-wide clock forward by d, running (in due-time
// order) every piece of work that falls due on the way. Not allowed on a
// realtime clock.
func Advance(ctx context.Context, d time.Duration) (time.Time, error) {
	c := Get()
	if c.mode == Realtime {
		return time.Time{}, ErrRealtime
	}
	if d < 0 {
		return time.Time{}, errors.New("duration must not be negative")
	}
	target := c.Now().Add(d)
	defaultMu.RLock()
	r := runner
	defaultMu.RUnlock()
	if r != nil {
		if err := r(ctx, target, c.set); err != nil {
			return c.Now(), err
		}
	}
	c.set(target)
	return c.Now(), nil
}
