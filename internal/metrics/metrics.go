package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const namespace = "zoomnotifier"

var (
	// HTTP request metrics (instrumented by middleware)

	HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "http_requests_total",
		Help:      "Total number of HTTP requests.",
	}, []string{"method", "route", "status"})

	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "http_request_duration_seconds",
		Help:      "Duration of HTTP requests in seconds.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "route"})

	// Zoom webhook metrics

	WebhooksReceived = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "zoom_webhooks_received_total",
		Help:      "Total number of Zoom webhook events received.",
	}, []string{"event"})

	WebhooksNoTenant = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "zoom_webhooks_no_tenant_total",
		Help:      "Zoom webhooks with no matching tenant.",
	})

	// Notification metrics

	NotificationsSent = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "notifications_sent_total",
		Help:      "Total notifications sent.",
	}, []string{"backend", "status"})

	// Zoom API client metrics

	ZoomAPIRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "zoom_api_requests_total",
		Help:      "Total Zoom API requests.",
	}, []string{"endpoint", "status"})

	ZoomTokenRefreshes = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "zoom_api_token_refreshes_total",
		Help:      "Total Zoom OAuth token refresh attempts.",
	}, []string{"status"})
)
