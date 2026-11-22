package zoom

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Zoom webhook signature headers.
// See https://developers.zoom.us/docs/api/webhooks/#verify-webhook-events
const (
	HeaderSignature = "x-zm-signature"
	HeaderTimestamp = "x-zm-request-timestamp"

	// MaxSignatureSkew bounds how far a request timestamp may drift from the
	// server clock, limiting the window in which a captured request can be replayed.
	MaxSignatureSkew = 5 * time.Minute

	// MaxWebhookBodyBytes caps the webhook body read before verification.
	MaxWebhookBodyBytes = 1 << 20
)

var (
	ErrMissingSignature = errors.New("missing zoom signature headers")
	ErrStaleTimestamp   = errors.New("zoom request timestamp outside allowed skew")
	ErrBadSignature     = errors.New("zoom signature mismatch")
)

// Sign computes the x-zm-signature value for a raw request body:
// "v0=" + hex(HMAC-SHA256(secret, "v0:{timestamp}:{body}")).
func Sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + timestamp + ":"))
	mac.Write(body)
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature checks that body was signed by Zoom with secret and that the
// request timestamp is within MaxSignatureSkew of now. It must be called on the
// exact raw bytes received, before any JSON decoding.
func VerifySignature(secret string, header http.Header, body []byte, now time.Time) error {
	if secret == "" {
		return errors.New("zoom webhook secret not configured")
	}
	sig := header.Get(HeaderSignature)
	ts := header.Get(HeaderTimestamp)
	if sig == "" || ts == "" {
		return ErrMissingSignature
	}

	tsSec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return fmt.Errorf("parse %s: %w", HeaderTimestamp, err)
	}
	skew := now.Sub(time.Unix(tsSec, 0))
	if skew < 0 {
		skew = -skew
	}
	if skew > MaxSignatureSkew {
		return ErrStaleTimestamp
	}

	if !hmac.Equal([]byte(Sign(secret, ts, body)), []byte(sig)) {
		return ErrBadSignature
	}
	return nil
}
