package faults

import (
	"sync"
	"time"
)

type rateLimitState struct {
	mu          sync.Mutex
	windowStart time.Time
	count       int
}

// CheckRateLimit reports whether a call at time now should be allowed, given
// limit requests per window, for the fixed key. limiters holds the live
// counters and is owned by the caller (typically one *sync.Map per running
// app/store instance) - state is in-memory only (not persisted), since a
// rate-limit window is ephemeral simulation state, not declarative fault
// config, and resets on restart.
func CheckRateLimit(limiters *sync.Map, key string, limit int, window time.Duration, now time.Time) bool {
	v, _ := limiters.LoadOrStore(key, &rateLimitState{windowStart: now})
	s := v.(*rateLimitState)

	s.mu.Lock()
	defer s.mu.Unlock()

	if now.Sub(s.windowStart) >= window {
		s.windowStart = now
		s.count = 0
	}
	s.count++
	return s.count <= limit
}
