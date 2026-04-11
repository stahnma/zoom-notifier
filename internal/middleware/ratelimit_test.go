package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/didip/tollbooth/v7"
)

func TestRateLimit_AllowsNormalTraffic(t *testing.T) {
	lmt := tollbooth.NewLimiter(100, nil) // generous limit
	handler := RateLimit(lmt, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodPost, "/webhook/zoom", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got '%s'", w.Body.String())
	}
}

func TestRateLimit_BlocksExcessiveTraffic(t *testing.T) {
	lmt := tollbooth.NewLimiter(1, nil) // 1 req/s
	lmt.SetMessage("rate limit exceeded")
	lmt.SetMessageContentType("text/plain")

	handler := RateLimit(lmt, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request should pass
	req1 := httptest.NewRequest(http.MethodPost, "/webhook/zoom", nil)
	req1.RemoteAddr = "10.0.0.1:12345"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Errorf("first request: expected 200, got %d", w1.Code)
	}

	// Rapid second request from same IP should be rate limited
	req2 := httptest.NewRequest(http.MethodPost, "/webhook/zoom", nil)
	req2.RemoteAddr = "10.0.0.1:12345"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("second request: expected 429, got %d", w2.Code)
	}
}

func TestRateLimit_DifferentIPsNotAffected(t *testing.T) {
	lmt := tollbooth.NewLimiter(1, nil) // 1 req/s

	handler := RateLimit(lmt, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Request from IP A
	req1 := httptest.NewRequest(http.MethodPost, "/webhook/zoom", nil)
	req1.RemoteAddr = "10.0.0.1:12345"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Errorf("IP A: expected 200, got %d", w1.Code)
	}

	// Request from IP B should not be affected by IP A's limit
	req2 := httptest.NewRequest(http.MethodPost, "/webhook/zoom", nil)
	req2.RemoteAddr = "10.0.0.2:12345"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("IP B: expected 200, got %d", w2.Code)
	}
}

func TestRateLimit_BurstAllowed(t *testing.T) {
	lmt := tollbooth.NewLimiter(1, nil) // 1 req/s sustained
	lmt.SetBurst(10)                    // but allow bursts of 10

	handler := RateLimit(lmt, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Send 10 rapid requests — all should pass (within burst)
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodPost, "/webhook/zoom", nil)
		req.RemoteAddr = "10.0.0.50:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	// 11th request should be rate limited (burst exhausted)
	req := httptest.NewRequest(http.MethodPost, "/webhook/zoom", nil)
	req.RemoteAddr = "10.0.0.50:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("request after burst: expected 429, got %d", w.Code)
	}
}

func TestNewWebhookRateLimiter(t *testing.T) {
	lmt := NewWebhookRateLimiter()
	if lmt == nil {
		t.Fatal("expected non-nil limiter")
	}
}
