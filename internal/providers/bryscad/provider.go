package bryscad

import (
	"database/sql"
	"net/http"

	"github.com/golgimed/mimic/internal/registry"
	"github.com/golgimed/mimic/internal/shared/admin"
)

func New(db *sql.DB, faultStore *admin.Store, webhookURL string) *registry.Provider {
	store := NewStore(db)
	return &registry.Provider{
		Name:     Name,
		Register: func(mux *http.ServeMux) { registerRoutes(mux, store, faultStore, db, webhookURL) },
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
