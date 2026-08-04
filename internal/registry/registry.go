// Package registry holds the Provider abstraction and the in-memory registry
// of enabled providers. It is deliberately dependency-free (no core/admin
// imports) so both the core HTTP wiring and the admin dashboard aggregation
// can depend on it without an import cycle.
package registry

import (
	"net/http"
	"sync"
)

// DashboardItem is a row in the admin dashboard's aggregated item list.
type DashboardItem struct {
	Provider  string `json:"provider"`
	Type      string `json:"type"`
	ID        string `json:"id"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

// WebhookDeliveryView is the dashboard-facing shape of a webhook delivery record.
type WebhookDeliveryView struct {
	URL          string `json:"url"`
	Payload      any    `json:"payload"`
	Status       string `json:"status"`
	ResponseCode *int64 `json:"responseCode"`
	CreatedAt    string `json:"createdAt"`
}

// ItemDetail is the dashboard's detail view for a single provider item.
type ItemDetail struct {
	Provider          string                `json:"provider"`
	Payload           any                   `json:"payload"`
	WebhookDeliveries []WebhookDeliveryView `json:"webhookDeliveries"`
}

// Provider owns its own routes, handlers and state. Name also doubles as the
// route prefix (e.g. "zenvia" -> routes registered under /zenvia/...).
// ListItems/GetItemDetail are optional (nil if a provider has nothing to
// show on the dashboard).
type Provider struct {
	Name          string
	Register      func(mux *http.ServeMux)
	ListItems     func() []DashboardItem
	GetItemDetail func(id string) (*ItemDetail, bool)
}

// Registry is an in-memory, insertion-ordered map of registered providers.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]*Provider
	order     []string
}

func New() *Registry {
	return &Registry{providers: make(map[string]*Provider)}
}

func (r *Registry) Register(p *Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.providers[p.Name]; !exists {
		r.order = append(r.order, p.Name)
	}
	r.providers[p.Name] = p
}

func (r *Registry) Get(name string) (*Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

func (r *Registry) All() []*Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Provider, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.providers[name])
	}
	return out
}

// Enabled returns providers filtered by enabledNames. A nil/empty slice
// enables every registered provider.
func (r *Registry) Enabled(enabledNames []string) []*Provider {
	if len(enabledNames) == 0 {
		return r.All()
	}

	enabled := make(map[string]bool, len(enabledNames))
	for _, name := range enabledNames {
		enabled[name] = true
	}

	var out []*Provider
	for _, p := range r.All() {
		if enabled[p.Name] {
			out = append(out, p)
		}
	}
	return out
}
