package integraicp

import (
	"database/sql"
	"net/http"

	"github.com/golgimed/mimic/internal/registry"
	"github.com/golgimed/mimic/internal/shared/admin"
)

func New(db *sql.DB, faultStore *admin.Store) *registry.Provider {
	store := NewStore(db)

	return &registry.Provider{
		Name: Name,
		Register: func(mux *http.ServeMux) {
			registerRoutes(mux, store, faultStore)
		},
		ListItems: func() []registry.DashboardItem {
			credentials, err := store.ListCredentials()
			if err != nil {
				return nil
			}
			items := make([]registry.DashboardItem, 0, len(credentials))
			for _, c := range credentials {
				items = append(items, registry.DashboardItem{
					Provider:  Name,
					Type:      "signature",
					ID:        c.ID,
					Status:    "AUTHENTICATED",
					CreatedAt: c.CreatedAt,
				})
			}
			return items
		},
		GetItemDetail: func(id string) (*registry.ItemDetail, bool) {
			credential, found, err := store.GetCredential(id)
			if err != nil || !found {
				return nil, false
			}
			return &registry.ItemDetail{
				Provider:          Name,
				Payload:           credential,
				WebhookDeliveries: []registry.WebhookDeliveryView{},
			}, true
		},
	}
}
