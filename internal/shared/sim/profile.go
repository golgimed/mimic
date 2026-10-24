package sim

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/golgimed/mimic/internal/shared/behavior"
)

// Profile is a named preset for simulation characteristics — timing,
// latency, failure rate — around providers' own behavior. It never changes
// what a provider does (contract, states, scenario path), only how fast and
// how reliably it does it.
//
// Precedence, highest first: an explicit fault (PUT /admin/faults or a
// scenario's faults) matching a request replaces the profile's latency and
// failure roll for that request; a scenario step's "after" and a provider's
// configured default delay set the base transition delay, which the
// profile then scales.
type Profile struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// DelayScale multiplies every lifecycle transition delay (0 = due
	// immediately).
	DelayScale float64 `json:"delayScale"`
	// Latency, if set, is added to every simulated request that no
	// explicit fault matched.
	Latency *behavior.Distribution `json:"latency,omitempty"`
	// FailureRate is the chance a simulated request (no explicit fault
	// matched) fails with FailureStatus.
	FailureRate   float64 `json:"failureRate,omitempty"`
	FailureStatus int     `json:"failureStatus,omitempty"`
}

// Profiles is the built-in profile set.
var Profiles = map[string]Profile{
	"fast": {
		Name:        "fast",
		Description: "No added latency and lifecycle transitions due immediately (run on the next scheduler tick or clock advance).",
		DelayScale:  0,
	},
	"realistic": {
		Name:        "realistic",
		Description: "Provider-default transition delays and ~150ms (normal, sd 50ms) added request latency.",
		DelayScale:  1,
		Latency:     &behavior.Distribution{Kind: behavior.DistNormal, MeanMS: 150, StdDev: 50},
	},
	"chaos": {
		Name:          "chaos",
		Description:   "Transition delays doubled, 50-1500ms (uniform) added latency, and 10% of requests fail with 503.",
		DelayScale:    2,
		Latency:       &behavior.Distribution{Kind: behavior.DistUniform, MinMS: 50, MaxMS: 1500},
		FailureRate:   0.1,
		FailureStatus: 503,
	},
}

// ProfileNames lists the built-in profiles, sorted.
func ProfileNames() []string {
	names := make([]string, 0, len(Profiles))
	for n := range Profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ValidateProfile checks a profile name ("" means none).
func ValidateProfile(name string) error {
	if name == "" {
		return nil
	}
	if _, ok := Profiles[name]; !ok {
		return fmt.Errorf("unknown profile %q (want one of %v)", name, ProfileNames())
	}
	return nil
}

var defaultProfile string

// SetDefaultProfile sets the profile (MIMIC_PROFILE) simulations get when
// they don't name one. "" means no profile: providers' own timing, no added
// latency or failures.
func SetDefaultProfile(name string) error {
	if err := ValidateProfile(name); err != nil {
		return err
	}
	defaultProfile = name
	return nil
}

// ActiveProfile returns ctx's simulation profile, if any.
func ActiveProfile(ctx context.Context) (Profile, bool) {
	s := FromContext(ctx)
	if s == nil || s.Profile == "" {
		return Profile{}, false
	}
	p, ok := Profiles[s.Profile]
	return p, ok
}

// ScaleDelay applies ctx's profile to a lifecycle transition delay.
func ScaleDelay(ctx context.Context, d time.Duration) time.Duration {
	if p, ok := ActiveProfile(ctx); ok {
		return time.Duration(float64(d) * p.DelayScale)
	}
	return d
}
