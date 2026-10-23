// Package behavior implements probabilistic and distribution-based
// primitives for simulating realistic API behavior on top of the existing
// fault_config mechanism.
package behavior

import "math/rand/v2"

// ShouldApply reports whether a probabilistic fault should fire this time,
// rolling on r — the caller's simulation-seeded stream (see
// internal/shared/sim), never global randomness. p == nil or *p >= 1
// always fires (preserves unconditional fault behavior); *p <= 0 never
// fires.
func ShouldApply(r *rand.Rand, p *float64) bool {
	if p == nil || *p >= 1 {
		return true
	}
	if *p <= 0 {
		return false
	}
	return r.Float64() < *p //NOSONAR -- fault-injection probability roll, not security-sensitive
}
