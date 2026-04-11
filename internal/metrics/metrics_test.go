package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsRegistered(t *testing.T) {
	// Verify all metrics are registered with prometheus default registry.
	// promauto registers automatically, so this confirms no panics on init
	// and that the collectors are queryable.
	metrics := []prometheus.Collector{
		HTTPRequestsTotal,
		HTTPRequestDuration,
		WebhooksReceived,
		WebhooksNoTenant,
		NotificationsSent,
		ZoomAPIRequests,
		ZoomTokenRefreshes,
	}

	for _, m := range metrics {
		if m == nil {
			t.Error("expected metric to be non-nil")
		}
	}
}

func TestHTTPRequestsTotal_Increment(t *testing.T) {
	HTTPRequestsTotal.WithLabelValues("GET", "/healthz", "200").Inc()
	// No panic = success. The counter was incremented without error.
}

func TestWebhooksReceived_Increment(t *testing.T) {
	WebhooksReceived.WithLabelValues("meeting.participant_joined").Inc()
}

func TestNotificationsSent_Increment(t *testing.T) {
	NotificationsSent.WithLabelValues("slack", "success").Inc()
}
