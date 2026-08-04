package admin

import (
	"fmt"

	"github.com/golgimed/mimic/internal/shared/behavior"
)

var validFaultKinds = map[FaultKind]bool{
	FaultDelayMS:        true,
	FaultHTTPStatus:     true,
	FaultTimeout:        true,
	FaultInvalidPayload: true,
	FaultWebhookDropped: true,
	FaultWebhookInvalid: true,
	FaultRateLimited:    true,
}

var validDistributionKinds = map[behavior.DistributionKind]bool{
	behavior.DistFixed:   true,
	behavior.DistUniform: true,
	behavior.DistNormal:  true,
}

// createFaultWire is the raw JSON shape accepted by PUT /admin/faults.
type createFaultWire struct {
	Provider          string   `json:"provider"`
	RoutePattern      *string  `json:"routePattern"`
	FaultKind         string   `json:"faultKind"`
	FaultValue        *string  `json:"faultValue"`
	Times             *int64   `json:"times"`
	Probability       *float64 `json:"probability"`
	DelayDistribution *string  `json:"delayDistribution"`
}

type validationIssue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (w createFaultWire) toInput() (CreateFaultInput, []validationIssue) {
	var issues []validationIssue

	if w.Provider == "" {
		issues = append(issues, validationIssue{Path: "provider", Message: "provider is required"})
	}
	if !validFaultKinds[FaultKind(w.FaultKind)] {
		issues = append(issues, validationIssue{
			Path:    "faultKind",
			Message: fmt.Sprintf("faultKind must be one of delay_ms, http_status, timeout, invalid_payload, webhook_dropped, webhook_invalid, rate_limited, got %q", w.FaultKind),
		})
	}
	if w.Times != nil && *w.Times <= 0 {
		issues = append(issues, validationIssue{Path: "times", Message: "times must be a positive integer"})
	}
	if w.Probability != nil && (*w.Probability < 0 || *w.Probability > 1) {
		issues = append(issues, validationIssue{Path: "probability", Message: "probability must be between 0 and 1"})
	}
	if w.DelayDistribution != nil {
		dist, err := behavior.ParseDistribution(*w.DelayDistribution)
		if err != nil {
			issues = append(issues, validationIssue{Path: "delayDistribution", Message: "delayDistribution must be valid JSON"})
		} else if !validDistributionKinds[dist.Kind] {
			issues = append(issues, validationIssue{
				Path:    "delayDistribution.kind",
				Message: fmt.Sprintf("delayDistribution.kind must be one of fixed, uniform, normal, got %q", dist.Kind),
			})
		}
	}

	return CreateFaultInput{
		Provider:          w.Provider,
		RoutePattern:      w.RoutePattern,
		FaultKind:         FaultKind(w.FaultKind),
		FaultValue:        w.FaultValue,
		Times:             w.Times,
		Probability:       w.Probability,
		DelayDistribution: w.DelayDistribution,
	}, issues
}
