package bryscad

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/golgimed/mimic/internal/shared/admin"
	"github.com/golgimed/mimic/internal/shared/httpx"
)

const Name = "bry-scad"

func withMiddleware(mux *http.ServeMux, faultStore *admin.Store, method, path string, handler http.HandlerFunc) {
	wrapped := requireBearer(admin.RequestFaultHook(faultStore, Name, path)(handler))
	mux.Handle(method+" "+path, wrapped)
}

// requireBearer preserves SCAD's documented Bearer-authentication shape. The
// simulator deliberately validates presence only, just like the other
// providers' local authentication shims.
func requireBearer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")) == "" {
			httpx.WriteJSON(w, http.StatusUnauthorized, map[string]any{
				"sucesso":   false,
				"chaveI18n": "nao_autorizado",
				"mensagem":  "Token Bearer ausente ou inválido.",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func registerRoutes(mux *http.ServeMux, store *Store, faultStore *admin.Store, db *sql.DB, webhookURL string) {
	// Webhooks (v1 and v2).
	withMiddleware(mux, faultStore, "GET", "/bry-scad/webhook/method", webhookMethodsHandler())
	withMiddleware(mux, faultStore, "GET", "/bry-scad/webhook/user-data", collectionHandler())
	withMiddleware(mux, faultStore, "POST", "/bry-scad/webhook/user-data", successHandler())
	withMiddleware(mux, faultStore, "PUT", "/bry-scad/webhook/user-data", successHandler())
	withMiddleware(mux, faultStore, "DELETE", "/bry-scad/webhook/user-data", successHandler())
	withMiddleware(mux, faultStore, "GET", "/bry-scad/webhook/v2/user-data", collectionHandler())
	withMiddleware(mux, faultStore, "POST", "/bry-scad/webhook/v2/user-data", successHandler())
	withMiddleware(mux, faultStore, "PUT", "/bry-scad/webhook/v2/user-data", successHandler())
	withMiddleware(mux, faultStore, "DELETE", "/bry-scad/webhook/v2/user-data", successHandler())

	withMiddleware(mux, faultStore, "GET", "/bry-scad/coletas/cadastrar/checar-permissao", permissionHandler())
	withMiddleware(mux, faultStore, "GET", "/bry-scad/tipos-assinatura", signatureTypesHandler())
	withMiddleware(mux, faultStore, "POST", "/bry-scad/coletas/cadastrar", createCollectionHandler(store))

	// Groups and tags.
	withMiddleware(mux, faultStore, "POST", "/bry-scad/grupos", successHandler())
	withMiddleware(mux, faultStore, "PUT", "/bry-scad/grupos", successHandler())
	withMiddleware(mux, faultStore, "GET", "/bry-scad/grupos", collectionHandler())
	withMiddleware(mux, faultStore, "GET", "/bry-scad/grupos/{id}", objectHandler())
	withMiddleware(mux, faultStore, "DELETE", "/bry-scad/grupos/{id}", successHandler())
	withMiddleware(mux, faultStore, "POST", "/bry-scad/tags", successHandler())
	withMiddleware(mux, faultStore, "PUT", "/bry-scad/tags", successHandler())
	withMiddleware(mux, faultStore, "GET", "/bry-scad/tags", collectionHandler())
	withMiddleware(mux, faultStore, "GET", "/bry-scad/tags/{id}", objectHandler())
	withMiddleware(mux, faultStore, "DELETE", "/bry-scad/tags/{id}", successHandler())

	// Signature-image and location configuration.
	withMiddleware(mux, faultStore, "POST", "/bry-scad/imagens-assinatura", successHandler())
	withMiddleware(mux, faultStore, "GET", "/bry-scad/imagens-assinatura", collectionHandler())
	withMiddleware(mux, faultStore, "POST", "/bry-scad/imagens-assinatura/{id}/usuario", successHandler())
	withMiddleware(mux, faultStore, "GET", "/bry-scad/imagens-assinatura/imagem-padrao", objectHandler())
	withMiddleware(mux, faultStore, "DELETE", "/bry-scad/imagens-assinatura/imagem-padrao", successHandler())
	withMiddleware(mux, faultStore, "GET", "/bry-scad/imagens-assinatura/{chave}", downloadHandler())
	withMiddleware(mux, faultStore, "DELETE", "/bry-scad/imagens-assinatura/{id}", successHandler())
	withMiddleware(mux, faultStore, "POST", "/bry-scad/coletas/{chave}/definir-locais-assinaturas", successHandler())

	// Signing process.
	withMiddleware(mux, faultStore, "OPTIONS", "/bry-scad/assinatura", locksHandler())
	withMiddleware(mux, faultStore, "GET", "/bry-scad/assinatura", startSigningHandler())
	withMiddleware(mux, faultStore, "POST", "/bry-scad/assinatura", finishSigningHandler())
	withMiddleware(mux, faultStore, "DELETE", "/bry-scad/assinatura", successHandler())
	withMiddleware(mux, faultStore, "POST", "/bry-scad/coletas/{chave}/finalizar-processo", successHandler())

	withMiddleware(mux, faultStore, "GET", "/bry-scad/coletas", listCollectionsHandler(store))
	withMiddleware(mux, faultStore, "GET", "/bry-scad/coletas/pendencias", collectionHandler())
	withMiddleware(mux, faultStore, "GET", "/bry-scad/coletas/{chave}", getCollectionHandler(store))
	withMiddleware(mux, faultStore, "GET", "/bry-scad/coletas/{chave}/historico", historyListHandler(store))
	withMiddleware(mux, faultStore, "GET", "/bry-scad/coletas/{chave}/participantes", participantsListHandler(store))
	withMiddleware(mux, faultStore, "GET", "/bry-scad/coletas/{chave}/grupos/{id}", collectionHandler())
	withMiddleware(mux, faultStore, "POST", "/bry-scad/coletas/{chave}/estender-data-limite", successHandler())
	withMiddleware(mux, faultStore, "POST", "/bry-scad/coletas/{chave}/reenviar-email", successHandler())
	withMiddleware(mux, faultStore, "POST", "/bry-scad/coletas/{chave}/cancelar", transitionCollectionHandler(store, "CANCELADO"))
	withMiddleware(mux, faultStore, "POST", "/bry-scad/coletas/{chave}/rejeitar", transitionCollectionHandler(store, "REJEITADA"))
	// Mimic-only test helper — see completeCollectionHandler doc comment.
	withMiddleware(mux, faultStore, "POST", "/bry-scad/coletas/{chave}/concluir", completeCollectionHandler(store, db, webhookURL))
	withMiddleware(mux, faultStore, "GET", "/bry-scad/coletas/{chave}/documentos", collectionHandler())
	withMiddleware(mux, faultStore, "GET", "/bry-scad/documento/{chave}", downloadHandler())
	withMiddleware(mux, faultStore, "GET", "/bry-scad/coletas/{chave}/documentos-assinados", signedDocumentsListHandler(store))
	withMiddleware(mux, faultStore, "GET", "/bry-scad/documento/assinado/{chave}", downloadHandler())
	withMiddleware(mux, faultStore, "GET", "/bry-scad/documentos/{chave}/protocolos", downloadHandler())
	withMiddleware(mux, faultStore, "GET", "/bry-scad/documentos/{chave}/assinaturas", downloadHandler())
	withMiddleware(mux, faultStore, "GET", "/bry-scad/documentos/{chave}/relatorio-assinaturas", downloadHandler())
	withMiddleware(mux, faultStore, "GET", "/bry-scad/documentos/{chave}/relatorio-assinaturas/anexo", downloadHandler())
	withMiddleware(mux, faultStore, "GET", "/bry-scad/relatorio/participantesPendentes", collectionHandler())
}
