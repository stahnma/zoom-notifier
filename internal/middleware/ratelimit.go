package middleware

import (
	"net/http"

	"github.com/didip/tollbooth/v7"
	"github.com/didip/tollbooth/v7/limiter"
)

const (
	// webhookRateLimit is the sustained requests per second per IP.
	webhookRateLimit = 10.0
	// webhookBurstSize is the maximum burst size per IP. Accommodates
	// legitimate bursts like a 30-person meeting ending (31 webhooks:
	// one participant_left per person plus meeting.ended).
	webhookBurstSize = 50
)

// NewWebhookRateLimiter creates a rate limiter for the webhook endpoint.
// Allows bursts of up to 50 requests, refilling at 10 requests/second per IP.
func NewWebhookRateLimiter() *limiter.Limiter {
	lmt := tollbooth.NewLimiter(webhookRateLimit, nil)
	lmt.SetBurst(webhookBurstSize)
	lmt.SetMessage("rate limit exceeded")
	lmt.SetMessageContentType("text/plain")
	return lmt
}

// RateLimit wraps an http.Handler with per-IP rate limiting.
func RateLimit(lmt *limiter.Limiter, next http.Handler) http.Handler {
	return tollbooth.LimitHandler(lmt, next)
}
