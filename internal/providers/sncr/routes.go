package sncr

import (
	"net/http"

	"github.com/golgimed/mimic/internal/shared/admin"
)

const Name = "sncr"

// withFault registers handler on mux under "METHOD path" while keying fault
// injection on the bare path only (no method prefix) — matching how faults
// are configured via PUT /admin/faults (routePattern is a path, not a
// method+path pair).
func withFault(mux *http.ServeMux, store *admin.Store, method, path string, handler http.HandlerFunc) {
	wrapped := admin.RequestFaultHook(store, Name, path)(handler)
	mux.Handle(method+" "+path, wrapped)
}

func registerRoutes(mux *http.ServeMux, store *Store, faultStore *admin.Store) {
	withFault(mux, faultStore, "GET", "/sncr/api/v1/auth/login", loginHandler(store))
	withFault(mux, faultStore, "GET", "/sncr/api/v1/auth/token", tokenHandler(store))
	withFault(mux, faultStore, "POST", "/sncr/api/v1/numeracoes/notificacao-receita", notificationNumberingHandler(store))
	withFault(mux, faultStore, "POST", "/sncr/api/v1/numeracoes/receita-especial-retencao", especialRetencaoHandler(store))
}
