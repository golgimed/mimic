// Package scenario describes provider behavior declaratively: which state
// path a resource follows, when, which webhooks that emits, and which
// faults apply. A scenario is selected per simulation (see
// internal/shared/sim); providers consult the active one where they would
// otherwise follow their default lifecycle. Scenarios configure provider
// behavior — they never replace provider routes, contracts or state models.
//
// Providers ship their scenarios as embedded YAML (scenarios.yaml next to
// the provider) and register them at construction.
package scenario

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/golgimed/mimic/internal/shared/sim"
)

// Duration is a time.Duration written as a Go duration string ("2s").
type Duration time.Duration

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	var s string
	if err := n.Decode(&s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(v)
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return []byte(`"` + time.Duration(d).String() + `"`), nil
}

// Step is one transition along the scenario's path. After is the delay
// since the previous state; nil means "the provider's default delay".
// Meta carries provider-specific details (e.g. a failure reason).
type Step struct {
	To    string            `yaml:"to" json:"to"`
	After *Duration         `yaml:"after,omitempty" json:"after,omitempty"`
	Meta  map[string]string `yaml:"meta,omitempty" json:"meta,omitempty"`
}

// Fault is a fault active for every request in a simulation running the
// scenario, in the same terms as PUT /admin/faults.
type Fault struct {
	Kind        string   `yaml:"kind" json:"kind"`
	Value       *string  `yaml:"value,omitempty" json:"value,omitempty"`
	Route       *string  `yaml:"route,omitempty" json:"route,omitempty"`
	Probability *float64 `yaml:"probability,omitempty" json:"probability,omitempty"`
	Times       *int64   `yaml:"times,omitempty" json:"times,omitempty"`
}

// Scenario is one named provider behavior.
type Scenario struct {
	Name        string `yaml:"name" json:"name"`
	Provider    string `yaml:"-" json:"provider"`
	Resource    string `yaml:"resource,omitempty" json:"resource,omitempty"`
	Description string `yaml:"description" json:"description"`
	// Initial is the state the resource is created in; Steps the path it
	// then follows. Both empty for fault-only scenarios.
	Initial string `yaml:"initial,omitempty" json:"initial,omitempty"`
	Steps   []Step `yaml:"transitions,omitempty" json:"transitions,omitempty"`
	// Webhooks lists the webhook events the lifecycle emits (descriptive).
	Webhooks []string `yaml:"webhooks,omitempty" json:"webhooks,omitempty"`
	Faults   []Fault  `yaml:"faults,omitempty" json:"faults,omitempty"`
	// Requires documents configuration the scenario depends on.
	Requires []string `yaml:"requires,omitempty" json:"requires,omitempty"`
}

// Next returns the step leading out of current along the scenario's path.
func (s *Scenario) Next(current string) (Step, bool) {
	state := s.Initial
	for _, step := range s.Steps {
		if state == current {
			return step, true
		}
		state = step.To
	}
	return Step{}, false
}

// Lifecycle returns the full state path, e.g. [PENDING SIGNED].
func (s *Scenario) Lifecycle() []string {
	if s.Initial == "" {
		return nil
	}
	path := []string{s.Initial}
	for _, step := range s.Steps {
		path = append(path, step.To)
	}
	return path
}

// Parse decodes a provider's scenarios.yaml (a list of scenarios).
// validEdge, if non-nil, checks each step against the provider's state
// machine.
func Parse(provider string, raw []byte, validEdge func(resource, from, to string) bool) ([]Scenario, error) {
	var list []Scenario
	if err := yaml.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("%s scenarios: %w", provider, err)
	}
	for i := range list {
		list[i].Provider = provider
		if err := list[i].validate(i, validEdge); err != nil {
			return nil, err
		}
	}
	return list, nil
}

// validate checks one parsed scenario (index i, for messages).
func (s *Scenario) validate(i int, validEdge func(resource, from, to string) bool) error {
	if s.Name == "" || s.Description == "" {
		return fmt.Errorf("%s scenario #%d: name and description are required", s.Provider, i+1)
	}
	if (s.Initial == "") != (len(s.Steps) == 0) {
		return fmt.Errorf("%s scenario %q: initial and transitions go together", s.Provider, s.Name)
	}
	if len(s.Steps) > 0 && s.Resource == "" {
		return fmt.Errorf("%s scenario %q: transitions need a resource", s.Provider, s.Name)
	}
	if validEdge == nil {
		return nil
	}
	from := s.Initial
	for _, step := range s.Steps {
		if !validEdge(s.Resource, from, step.To) {
			return fmt.Errorf("%s scenario %q: %s -> %s is not an allowed %s transition", s.Provider, s.Name, from, step.To, s.Resource)
		}
		from = step.To
	}
	return nil
}

// MustParse is Parse for embedded, compile-time-known YAML.
func MustParse(provider string, raw []byte, validEdge func(resource, from, to string) bool) []Scenario {
	list, err := Parse(provider, raw, validEdge)
	if err != nil {
		panic(err)
	}
	return list
}

var (
	mu      sync.RWMutex
	catalog = map[string]Scenario{}
)

// Register adds scenarios to the catalog. Names are global (a simulation
// selects one by name); re-registering a provider's own scenario replaces
// it, so building a provider twice (tests) is harmless.
func Register(list ...Scenario) error {
	mu.Lock()
	defer mu.Unlock()
	for _, s := range list {
		if existing, ok := catalog[s.Name]; ok && existing.Provider != s.Provider {
			return fmt.Errorf("scenario %q is defined by both %s and %s", s.Name, existing.Provider, s.Provider)
		}
		catalog[s.Name] = s
	}
	return nil
}

// Lookup returns the named scenario.
func Lookup(name string) (Scenario, bool) {
	mu.RLock()
	defer mu.RUnlock()
	s, ok := catalog[name]
	return s, ok
}

// All returns every registered scenario, sorted by provider then name.
func All() []Scenario {
	mu.RLock()
	out := make([]Scenario, 0, len(catalog))
	for _, s := range catalog {
		out = append(out, s)
	}
	mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Active returns the scenario ctx's simulation runs, if it applies to
// provider's resource.
func Active(ctx context.Context, provider, resource string) (*Scenario, bool) {
	s := sim.FromContext(ctx)
	if s == nil || s.Scenario == "" {
		return nil, false
	}
	sc, ok := Lookup(s.Scenario)
	if !ok || sc.Provider != provider || sc.Resource != resource || len(sc.Steps) == 0 {
		return nil, false
	}
	return &sc, true
}
