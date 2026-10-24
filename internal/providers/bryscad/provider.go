package bryscad

import (
	"database/sql"
	_ "embed"
	"net/http"

	"github.com/golgimed/mimic/internal/registry"
	"github.com/golgimed/mimic/internal/shared/admin"
	"github.com/golgimed/mimic/internal/shared/scenario"
	"github.com/golgimed/mimic/internal/shared/scheduler"
)

//go:embed scenarios.yaml
var scenariosYAML []byte

// Scenarios are bry-scad's built-in scenarios (scenarios.yaml).
var Scenarios = scenario.MustParse(Name, scenariosYAML, func(resource, from, to string) bool {
	return resource == "coleta" && collectionLifecycle.Can(from, to)
})

func New(db *sql.DB, faultStore *admin.Store, sched *scheduler.Scheduler, webhookURL string) *registry.Provider {
	if err := scenario.Register(Scenarios...); err != nil {
		panic(err)
	}
	store := NewStore(db)
	// The runner registers its job handler, so build it once per provider.
	runner := newCollectionRunner(store, sched, db, webhookURL)
	return &registry.Provider{
		Name:     Name,
		Register: func(mux *http.ServeMux) { registerRoutes(mux, store, runner, faultStore, db, webhookURL) },
		ListItems: func() []registry.DashboardItem {
			collections, err := store.ListCollections()
			if err != nil {
				return nil
			}
			items := make([]registry.DashboardItem, 0, len(collections))
			for _, c := range collections {
				items = append(items, registry.DashboardItem{Provider: Name, Type: "coleta", ID: c.Chave, Status: c.Situacao, CreatedAt: c.CreatedAt})
			}
			return items
		},
		GetItemDetail: func(id string) (*registry.ItemDetail, bool) {
			collection, err := store.GetCollection(id)
			if err != nil || collection == nil {
				return nil, false
			}
			return &registry.ItemDetail{Provider: Name, Payload: collectionResponse(collection), WebhookDeliveries: []registry.WebhookDeliveryView{}}, true
		},
	}
}
