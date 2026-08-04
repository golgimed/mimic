package integraicp

import (
	"net/http"

	"github.com/golgimed/mimic/internal/shared/admin"
)

const Name = "integraicp"

// withFault registers handler on mux under "METHOD path" while keying fault
// injection on the bare path only (no method prefix) — matching how faults
// are configured via PUT /admin/faults (routePattern is a path, not a
// method+path pair).
func withFault(mux *http.ServeMux, store *admin.Store, method, path string, handler http.HandlerFunc) {
	wrapped := admin.RequestFaultHook(store, Name, path)(handler)
	mux.Handle(method+" "+path, wrapped)
}

func registerRoutes(mux *http.ServeMux, store *Store, faultStore *admin.Store) {
	withFault(mux, faultStore, "GET", "/integraicp/c/{channelId}/icp/v3/authentications", authenticationsHandler(store))
	withFault(mux, faultStore, "GET", "/integraicp/c/{channelId}/icp/v3/credentials/{credentialId}", credentialsHandler(store))
	withFault(mux, faultStore, "POST", "/integraicp/c/{channelId}/icp/v3/signatures", signaturesHandler(store))
}
