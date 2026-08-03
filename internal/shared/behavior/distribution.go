package behavior

import (
	"encoding/json"
	"math/rand"
	"time"
)

type DistributionKind string

const (
	DistFixed   DistributionKind = "fixed"
	DistUniform DistributionKind = "uniform"
	DistNormal  DistributionKind = "normal"
)

// Distribution is the JSON shape stored in fault_config.delay_distribution.
type Distribution struct {
	Kind   DistributionKind `json:"kind"`
	MS     float64          `json:"ms,omitempty"`
	MinMS  float64          `json:"minMs,omitempty"`
	MaxMS  float64          `json:"maxMs,omitempty"`
	MeanMS float64          `json:"meanMs,omitempty"`
	StdDev float64          `json:"stdDevMs,omitempty"`
}

// Sample returns a duration for one call. Negative results (possible with
// normal sampling) are clamped to 0 — there's no such thing as negative
// latency.
func (d Distribution) Sample() time.Duration {
	var ms float64
	switch d.Kind {
	case DistUniform:
		ms = d.MinMS + rand.Float64()*(d.MaxMS-d.MinMS)
	case DistNormal:
		ms = rand.NormFloat64()*d.StdDev + d.MeanMS
	default: // DistFixed or unrecognized
		ms = d.MS
	}
	if ms < 0 {
		ms = 0
	}
	return time.Duration(ms * float64(time.Millisecond))
}

func ParseDistribution(raw string) (*Distribution, error) {
	var d Distribution
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return nil, err
	}
	return &d, nil
}
