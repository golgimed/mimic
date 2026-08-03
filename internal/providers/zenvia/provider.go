package zenvia

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/golgimed/mimic/internal/registry"
	"github.com/golgimed/mimic/internal/shared/admin"
	"github.com/golgimed/mimic/internal/shared/scheduler"
	"github.com/golgimed/mimic/internal/shared/webhooks"
)

func New(db *sql.DB, faultStore *admin.Store, sched *scheduler.Scheduler, statusDelay time.Duration) *registry.Provider {
	store := NewStore(db)
	scheduleAdvance := RegisterScheduler(sched, db, store, faultStore, statusDelay)

	return &registry.Provider{
		Name: Name,
		Register: func(mux *http.ServeMux) {
			registerRoutes(mux, store, faultStore, scheduleAdvance)
		},
		ListItems: func() []registry.DashboardItem {
			messages, err := store.ListMessages()
			if err != nil {
				return nil
			}
			items := make([]registry.DashboardItem, 0, len(messages))
			for _, m := range messages {
				items = append(items, registry.DashboardItem{
					Provider: Name, Type: m.Channel, ID: m.ID, Status: m.Status, CreatedAt: m.CreatedAt,
				})
			}
			return items
		},
		GetItemDetail: func(id string) (*registry.ItemDetail, bool) {
			message, err := store.GetMessage(id)
			if err != nil || message == nil {
				return nil, false
			}
			deliveries, err := webhooks.ListDeliveries(db, Name, id)
			if err != nil {
				return nil, false
			}
			return &registry.ItemDetail{
				Provider:          Name,
				Payload:           message,
				WebhookDeliveries: deliveries,
			}, true
		},
	}
}
