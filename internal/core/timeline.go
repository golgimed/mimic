package core

import (
	"net/http"
	"strings"
	"time"

	"github.com/golgimed/mimic/internal/shared/timeline"
)

// withTimeline puts rec into each request's context and records the
// request and its response on the simulation timeline.
func withTimeline(rec *timeline.Recorder, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := timeline.WithRecorder(r.Context(), rec)
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
