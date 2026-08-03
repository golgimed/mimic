package admin

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golgimed/mimic/internal/shared/behavior"
	"github.com/golgimed/mimic/internal/shared/faults"
)

// RequestFaultHook returns middleware that checks for a configured fault
// matching this provider + routePattern before the real handler runs. Only
// request-time fault kinds are handled here (delay_ms, http_status, timeout,
// invalid_payload, rate_limited) - webhook_dropped/webhook_invalid are
// applied at webhook delivery time.
func RequestFaultHook(store *Store, provider, routePattern string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fault, err := store.ConsumeMatchingFault(provider, routePattern)
			if err != nil || fault == nil {
				next.ServeHTTP(w, r)
				return
			}

			switch fault.FaultKind {
			case FaultDelayMS:
				var d time.Duration
				if fault.DelayDistribution != nil {
					if dist, err := behavior.ParseDistribution(*fault.DelayDistribution); err == nil {
						d = dist.Sample()
					}
				} else if fault.FaultValue != nil {
					ms, _ := strconv.ParseInt(*fault.FaultValue, 10, 64)
					d = time.Duration(ms) * time.Millisecond
				}
				faults.SimulateLatency(r.Context(), &d)
				next.ServeHTTP(w, r)

			case FaultRateLimited:
				limit, window, ok := parseRateLimitValue(fault.FaultValue)
				if !ok || faults.CheckRateLimit(&store.rateLimiters, provider+"|"+routePattern, limit, window, time.Now()) {
					next.ServeHTTP(w, r)
					return
				}
				w.Header().Set("Retry-After", strconv.Itoa(int(window.Seconds())))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{
						"code":    http.StatusTooManyRequests,
						"message": "Simulated rate limit via fault injection",
					},
				})

			case FaultHTTPStatus:
				status := 500
				if fault.FaultValue != nil {
					if v, err := strconv.Atoi(*fault.FaultValue); err == nil {
						status = v
					}
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{
						"code":    status,
						"message": "Simulated " + strconv.Itoa(status) + " via fault injection",
					},
				})

			case FaultInvalidPayload:
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"truncated": tr`))

			case FaultTimeout:
				<-r.Context().Done()

			default:
				next.ServeHTTP(w, r)
			}
		})
	}
}

// parseRateLimitValue parses a FaultValue of the shape "<limit>/<window>"
// (e.g. "5/1s") into a request limit and a window duration.
func parseRateLimitValue(value *string) (limit int, window time.Duration, ok bool) {
	if value == nil {
		return 0, 0, false
	}
	parts := strings.SplitN(*value, "/", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	limit, err := strconv.Atoi(parts[0])
	if err != nil || limit <= 0 {
		return 0, 0, false
	}
	window, err = time.ParseDuration(parts[1])
	if err != nil || window <= 0 {
		return 0, 0, false
	}
	return limit, window, true
}
