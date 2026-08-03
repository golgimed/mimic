package openapi

import (
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/golgimed/mimic/internal/registry"
	"github.com/golgimed/mimic/internal/shared/admin"
)

// SpecMeta is the per-spec metadata surfaced on the dashboard: enough to
// identify which version of a spec is currently loaded without inventing a
// mechanism beyond registry.Provider's existing ListItems/GetItemDetail.
type SpecMeta struct {
	Prefix   string
	Path     string
	Title    string
	Version  string
	Checksum string
}

// New builds a registry.Provider from a merged route table. From core's
// point of view this is indistinguishable from any hand-written provider —
// core never imports this package, only internal/providers/providers.go
// does.
//
// Unlike the hand-written providers, spec-derived routes don't require an
// X-API-TOKEN header: an arbitrary OpenAPI spec makes no promise about
// auth, so this adapter doesn't invent one. Fault injection still applies,
// since that's the simulator's own feature, not the provider's contract.
func New(name string, routes []Route, faultStore *admin.Store, specs []SpecMeta, db *sql.DB) *registry.Provider {
	loadedAt := time.Now().UTC().Format(time.RFC3339)
	store := NewStore(db)

	routeCount := make(map[string]int, len(specs))
	for _, r := range routes {
		for _, s := range specs {
			if r.Source == s.Path {
				routeCount[s.Prefix]++
				break
			}
		}
	}

	byPrefix := make(map[string]SpecMeta, len(specs))
	for _, s := range specs {
		byPrefix[s.Prefix] = s
	}

	return &registry.Provider{
		Name: name,
		Register: func(mux *http.ServeMux) {
			for _, route := range routes {
				wrapped := admin.RequestFaultHook(faultStore, name, route.Path)(route.handler(store))
				mux.Handle(route.Method+" "+route.Path, wrapped)
			}
		},
		ListItems: func() []registry.DashboardItem {
			items := make([]registry.DashboardItem, 0, len(specs))
			for _, s := range specs {
				version := s.Version
				if version == "" {
					version = "unversioned"
				}
				checksum := s.Checksum
				if len(checksum) > 8 {
					checksum = checksum[:8]
				}
				items = append(items, registry.DashboardItem{
					Provider:  name,
					Type:      "spec",
					ID:        s.Prefix,
					Status:    fmt.Sprintf("v%s · %s", version, checksum),
					CreatedAt: loadedAt,
				})
			}
			sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
			return items
		},
		GetItemDetail: func(id string) (*registry.ItemDetail, bool) {
			s, ok := byPrefix[id]
			if !ok {
				return nil, false
			}
			return &registry.ItemDetail{
				Provider: name,
				Payload: map[string]any{
					"prefix":     s.Prefix,
					"path":       s.Path,
					"title":      s.Title,
					"version":    s.Version,
					"checksum":   s.Checksum,
					"routeCount": routeCount[s.Prefix],
					"loadedAt":   loadedAt,
				},
			}, true
		},
	}
}
