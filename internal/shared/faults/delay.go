// Package faults provides small helpers for simulating processing latency.
package faults

import (
	"context"
	"time"
)

// defaultDelay is the fallback latency (DEFAULT_DELAY_MS) applied when a
// call to SimulateLatency doesn't pass an explicit override.
var defaultDelay time.Duration

// SetDefaultDelay configures the fallback latency used by SimulateLatency
// when no explicit delay is given. Called once at startup from config.
func SetDefaultDelay(d time.Duration) {
	defaultDelay = d
}

// SimulateLatency sleeps for d if positive, otherwise falls back to the
// configured default delay. Respects context cancellation.
func SimulateLatency(ctx context.Context, d *time.Duration) {
	delay := defaultDelay
	if d != nil {
		delay = *d
	}
	if delay <= 0 {
		return
	}
	select {
	case <-time.After(delay):
	case <-ctx.Done():
	}
}
