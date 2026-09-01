package brymedical

import (
	"database/sql"
	"net/http"

	"github.com/golgimed/mimic/internal/registry"
	"github.com/golgimed/mimic/internal/shared/admin"
)

func New(db *sql.DB, faultStore *admin.Store) *registry.Provider {
	store := NewStore(db)
	return &registry.Provider{
		Name:     Name,
		Register: func(mux *http.ServeMux) { registerRoutes(mux, store, faultStore) },
	}
}
