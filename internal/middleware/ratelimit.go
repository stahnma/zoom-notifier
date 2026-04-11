package middleware

import (
	"net/http"

	"github.com/didip/tollbooth/v7"
	"github.com/didip/tollbooth/v7/limiter"
)

const (
	// webhookRateLimit is the maximum requests per second per IP for the webhook endpoint.
	webhookRateLimit = 10.0
)

// NewWebhookRateLimiter creates a rate limiter for the webhook endpoint.
// Limits to 10 requests/second per IP address.
func NewWebhookRateLimiter() *limiter.Limiter {
	lmt := tollbooth.NewLimiter(webhookRateLimit, nil)
	lmt.SetMessage("rate limit exceeded")
	lmt.SetMessageContentType("text/plain")
	return lmt
}

// RateLimit wraps an http.Handler with per-IP rate limiting.
func RateLimit(lmt *limiter.Limiter, next http.Handler) http.Handler {
	return tollbooth.LimitHandler(lmt, next)
}
