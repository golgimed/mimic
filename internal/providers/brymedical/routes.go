package brymedical

import (
	"net/http"
	"strings"

	"github.com/golgimed/mimic/internal/shared/admin"
)

const Name = "bry-medical"

func withMiddleware(mux *http.ServeMux, faultStore *admin.Store, method, path string, handler http.HandlerFunc) {
	wrapped := requireBearer(admin.RequestFaultHook(faultStore, Name, path)(handler))
	mux.Handle(method+" "+path, wrapped)
}

// requireBearer mirrors bryscad's presence-only Bearer check: the simulator
// validates the header is present, not that it matches a real BRy app token.
func requireBearer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")) == "" {
			writeHubError(w, http.StatusUnauthorized, "Authorization Bearer token ausente ou inválido.", "")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireKMSType enforces the HUB Signer's documented kms_type header,
// which golgimed's adapter always sets to BRYKMS — the only credential
// variant the adapter (and therefore the Mimic) implements.
func requireKMSType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("kms_type") != "BRYKMS" {
			writeHubError(w, http.StatusBadRequest, "kms_type header ausente ou não suportado (esperado BRYKMS).", "")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func registerRoutes(mux *http.ServeMux, store *Store, faultStore *admin.Store) {
	withMiddleware(mux, faultStore, "POST", "/bry-kms/chaves/{uuidChave}/autorizacoes", preAuthorizeHandler(store))

	signPath := "/bry-medical/fw/v1/pdf/kms/lote/assinaturas"
	wrapped := requireBearer(requireKMSType(admin.RequestFaultHook(faultStore, Name, signPath)(signHandler(store))))
	mux.Handle("POST "+signPath, wrapped)
}
