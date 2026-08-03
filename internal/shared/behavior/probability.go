// Package behavior implements probabilistic and distribution-based
// primitives for simulating realistic API behavior on top of the existing
// fault_config mechanism.
package behavior

import "math/rand"

// ShouldApply reports whether a probabilistic fault should fire this time.
// p == nil or *p >= 1 always fires (preserves unconditional fault behavior).
// *p <= 0 never fires.
func ShouldApply(p *float64) bool {
	if p == nil || *p >= 1 {
		return true
	}
	if *p <= 0 {
		return false
	}
	return rand.Float64() < *p
}
