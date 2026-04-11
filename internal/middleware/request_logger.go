package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	log "github.com/sirupsen/logrus"
	"github.com/stahnma/zoom-notifier/internal/metrics"
)

// statusRecorder wraps http.ResponseWriter to capture the status code.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// RequestLogger returns middleware that logs each HTTP request with
// method, path, status code, and duration. It also records Prometheus
// metrics using the Chi route pattern (not the raw URL path).
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		duration := time.Since(start)

		// Use Chi route pattern for metrics labels to avoid unbounded cardinality.
		// Falls back to raw path if no route context (e.g. in tests without Chi).
		route := r.URL.Path
		if rctx := chi.RouteContext(r.Context()); rctx != nil && rctx.RoutePattern() != "" {
			route = rctx.RoutePattern()
		}

		statusStr := fmt.Sprintf("%d", rec.status)
		metrics.HTTPRequestsTotal.WithLabelValues(r.Method, route, statusStr).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(r.Method, route).Observe(duration.Seconds())

		log.WithFields(log.Fields{
			"method":   r.Method,
			"path":     r.URL.Path,
			"status":   rec.status,
			"duration": duration.String(),
		}).Info("http request")
	})
}
