// Package crm simulates the CFM physician-lookup endpoint that golgimed's
// platform-services CRM adapter (HTTPCRMClient.QueryLicense) calls. Dev-only.
package crm

import (
	"net/http"

	"github.com/golgimed/mimic/internal/registry"
	"github.com/golgimed/mimic/internal/shared/admin"
	"github.com/golgimed/mimic/internal/shared/httpx"
)

const Name = "crm"

// DefaultAllowedLicenses is the dev seed persona (Ana Costa, db/seeds/dev/008).
var DefaultAllowedLicenses = []string{"CRM-SP 98765"}

// New serves GET /crm/busca-medico-by-name/{license}: 200 {} when license is
// in allowed, 404 otherwise. The real client only checks for HTTP 200.
func New(faultStore *admin.Store, allowed []string) *registry.Provider {
	set := make(map[string]struct{}, len(allowed))
	for _, l := range allowed {
		set[l] = struct{}{}
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		if _, ok := set[r.PathValue("license")]; !ok {
			httpx.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "license not found"})
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{})
	}

	const path = "/crm/busca-medico-by-name/{license}"
	return &registry.Provider{
		Name: Name,
		Register: func(mux *http.ServeMux) {
			mux.Handle("GET "+path, admin.RequestFaultHook(faultStore, Name, path)(http.HandlerFunc(handler)))
		},
	}
}
