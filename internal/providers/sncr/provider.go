package sncr

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
			tokens, err := store.ListAccessTokens()
			if err != nil {
				return nil
			}
			items := make([]registry.DashboardItem, 0, len(tokens))
			for _, t := range tokens {
				items = append(items, registry.DashboardItem{
					Provider:  Name,
					Type:      "access_token",
					ID:        t.AccessToken,
					Status:    "ISSUED",
					CreatedAt: t.CreatedAt,
				})
			}
			return items
		},
		GetItemDetail: func(id string) (*registry.ItemDetail, bool) {
			record, err := store.GetAccessTokenRecord(id)
			if err != nil || record == nil {
				return nil, false
			}
			return &registry.ItemDetail{
				Provider:          Name,
				Payload:           record,
				WebhookDeliveries: []registry.WebhookDeliveryView{},
			}, true
		},
	}
}
