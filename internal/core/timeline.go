package core

import (
	"net/http"
	"strings"
	"time"

	"github.com/golgimed/mimic/internal/shared/correlation"
	"github.com/golgimed/mimic/internal/shared/timeline"
)

// withTimeline gives each request a correlation ID (the client's
// X-Request-ID if usable, else a fresh UUID), echoes it as
// X-Mimic-Request-ID, puts it and rec into the request context, and
// records the request and its response on the simulation timeline.
func withTimeline(rec *timeline.Recorder, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := correlation.FromClient(r.Header.Get(correlation.ClientHeader))
		w.Header().Set(correlation.Header, requestID)
		ctx := correlation.WithRequestID(timeline.WithRecorder(r.Context(), rec), requestID)
		r = r.WithContext(ctx)
		provider, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
		start := time.Now()

		timeline.Record(ctx, timeline.Event{
			Type:     timeline.RequestReceived,
			Provider: provider,
			Data:     map[string]any{"method": r.Method, "path": r.URL.Path},
		})
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		timeline.Record(ctx, timeline.Event{
			Type:     timeline.ResponseReturned,
			Provider: provider,
			Data: map[string]any{
				"method":     r.Method,
				"path":       r.URL.Path,
				"status":     sw.status,
				"durationMs": time.Since(start).Milliseconds(),
			},
		})
	})
}

// statusWriter captures the response status code.
type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.status, w.wroteHeader = code, true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	w.wroteHeader = true
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
