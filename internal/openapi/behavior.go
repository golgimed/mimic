package openapi

import (
	"encoding/json"
	"fmt"

	"github.com/golgimed/mimic/internal/shared/admin"
	"github.com/golgimed/mimic/internal/shared/behavior"
)

// behaviorSpec is the x-mimic-behavior wire shape, deliberately mirroring
// admin's createFaultWire field-for-field: it exists to pre-populate a
// fault_config row at boot (see toCreateFaultInput), so every existing
// consumption path (ConsumeMatchingFault, RequestFaultHook, GET
// /admin/faults) works on spec-declared behavior unmodified.
type behaviorSpec struct {
	FaultKind         string                    `yaml:"faultKind"`
	FaultValue        *string                   `yaml:"faultValue"`
	Probability       *float64                  `yaml:"probability"`
	DelayDistribution *behaviorDistributionSpec `yaml:"delayDistribution"`
	Times             *int64                    `yaml:"times"`
}

type behaviorDistributionSpec struct {
	Kind   string  `yaml:"kind"`
	MS     float64 `yaml:"ms"`
	MinMS  float64 `yaml:"minMs"`
	MaxMS  float64 `yaml:"maxMs"`
	MeanMS float64 `yaml:"meanMs"`
	StdDev float64 `yaml:"stdDevMs"`
}

// validBehaviorFaultKinds excludes the webhook_* kinds: a route's
// x-mimic-behavior seeds a fault keyed by the route's own path, never by the
// synthetic "webhook" route pattern webhook-time faults use.
var validBehaviorFaultKinds = map[admin.FaultKind]bool{
	admin.FaultDelayMS:        true,
	admin.FaultHTTPStatus:     true,
	admin.FaultTimeout:        true,
	admin.FaultInvalidPayload: true,
	admin.FaultRateLimited:    true,
}

var validBehaviorDistributionKinds = map[behavior.DistributionKind]bool{
	behavior.DistFixed:   true,
	behavior.DistUniform: true,
	behavior.DistNormal:  true,
}

// ToCreateFaultInput validates and converts a spec-declared behavior into
// the input for admin.Store.CreateFault, scoped to provider+routePattern.
func (b *behaviorSpec) ToCreateFaultInput(provider, routePattern string) (admin.CreateFaultInput, error) {
	kind := admin.FaultKind(b.FaultKind)
	if !validBehaviorFaultKinds[kind] {
		return admin.CreateFaultInput{}, fmt.Errorf(
			"faultKind must be one of delay_ms, http_status, timeout, invalid_payload, rate_limited, got %q", b.FaultKind)
	}
	if b.Probability != nil && (*b.Probability < 0 || *b.Probability > 1) {
		return admin.CreateFaultInput{}, fmt.Errorf("probability must be between 0 and 1")
	}

	var delayDistribution *string
	if b.DelayDistribution != nil {
		dk := behavior.DistributionKind(b.DelayDistribution.Kind)
		if !validBehaviorDistributionKinds[dk] {
			return admin.CreateFaultInput{}, fmt.Errorf(
				"delayDistribution.kind must be one of fixed, uniform, normal, got %q", b.DelayDistribution.Kind)
		}
		raw, err := json.Marshal(behavior.Distribution{
			Kind:   dk,
			MS:     b.DelayDistribution.MS,
			MinMS:  b.DelayDistribution.MinMS,
			MaxMS:  b.DelayDistribution.MaxMS,
			MeanMS: b.DelayDistribution.MeanMS,
			StdDev: b.DelayDistribution.StdDev,
		})
		if err != nil {
			return admin.CreateFaultInput{}, err
		}
		s := string(raw)
		delayDistribution = &s
	}

	return admin.CreateFaultInput{
		Provider:          provider,
		RoutePattern:      &routePattern,
		FaultKind:         kind,
		FaultValue:        b.FaultValue,
		Times:             b.Times,
		Probability:       b.Probability,
		DelayDistribution: delayDistribution,
	}, nil
}
