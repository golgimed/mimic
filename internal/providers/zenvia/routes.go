package zenvia

import (
	"net/http"

	"github.com/golgimed/mimic/internal/shared/admin"
	"github.com/golgimed/mimic/internal/shared/auth"
)

const Name = "zenvia"

// withMiddleware registers handler on mux under "METHOD path", requiring the
// X-API-TOKEN header and applying fault injection (keyed on the bare path,
// matching how faults are configured via PUT /admin/faults).
func withMiddleware(mux *http.ServeMux, faultStore *admin.Store, method, path string, handler http.HandlerFunc) {
	wrapped := auth.RequireAPIToken("X-API-TOKEN")(
		admin.RequestFaultHook(faultStore, Name, path)(handler),
	)
	mux.Handle(method+" "+path, wrapped)
}

func registerRoutes(mux *http.ServeMux, store *Store, faultStore *admin.Store, scheduleAdvance func(string) error) {
	withMiddleware(mux, faultStore, "POST", "/zenvia/channels/sms/messages", createMessageHandler("sms", store, scheduleAdvance))
	withMiddleware(mux, faultStore, "POST", "/zenvia/channels/whatsapp/messages", createMessageHandler("whatsapp", store, scheduleAdvance))
	withMiddleware(mux, faultStore, "POST", "/zenvia/channels/email/messages", createMessageHandler("email", store, scheduleAdvance))

	withMiddleware(mux, faultStore, "POST", "/zenvia/subscriptions", createSubscriptionHandler(store))
	withMiddleware(mux, faultStore, "GET", "/zenvia/subscriptions", listSubscriptionsHandler(store))
	withMiddleware(mux, faultStore, "GET", "/zenvia/subscriptions/{subscriptionId}", getSubscriptionHandler(store))
	withMiddleware(mux, faultStore, "DELETE", "/zenvia/subscriptions/{subscriptionId}", deleteSubscriptionHandler(store))
}
