package zoom

import (
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestSign_KnownVector(t *testing.T) {
	// Computed independently: HMAC-SHA256("test-secret", `v0:1700000000:{"event":"meeting.started"}`)
	got := Sign("test-secret", "1700000000", []byte(`{"event":"meeting.started"}`))
	want := "v0=efdf2392b434b2831ee596df1ff41b306dfaddd8a5308e75eba1a4401cfd6026"
	if got != want {
		t.Errorf("Sign() = %s, want %s", got, want)
	}
}

func TestVerifySignature(t *testing.T) {
	now := time.Unix(1700000000, 0)
	body := []byte(`{"event":"meeting.started"}`)
	ts := strconv.FormatInt(now.Unix(), 10)

	headers := func(ts, sig string) http.Header {
		h := http.Header{}
		if ts != "" {
			h.Set(HeaderTimestamp, ts)
		}
		if sig != "" {
			h.Set(HeaderSignature, sig)
		}
		return h
	}
	staleTS := strconv.FormatInt(now.Add(-MaxSignatureSkew-time.Second).Unix(), 10)
	futureTS := strconv.FormatInt(now.Add(MaxSignatureSkew+time.Second).Unix(), 10)

	tests := []struct {
		name    string
		secret  string
		header  http.Header
		body    []byte
		wantErr error // nil = success; errAny = any error
	}{
		{"valid", "s3cret", headers(ts, Sign("s3cret", ts, body)), body, nil},
		{"missing signature", "s3cret", headers(ts, ""), body, ErrMissingSignature},
		{"missing timestamp", "s3cret", headers("", Sign("s3cret", ts, body)), body, ErrMissingSignature},
		{"wrong secret", "s3cret", headers(ts, Sign("other", ts, body)), body, ErrBadSignature},
		{"tampered body", "s3cret", headers(ts, Sign("s3cret", ts, body)), []byte(`{"event":"meeting.ended"}`), ErrBadSignature},
		{"signature for different timestamp", "s3cret", headers(ts, Sign("s3cret", "1700000001", body)), body, ErrBadSignature},
		{"stale timestamp", "s3cret", headers(staleTS, Sign("s3cret", staleTS, body)), body, ErrStaleTimestamp},
		{"future timestamp", "s3cret", headers(futureTS, Sign("s3cret", futureTS, body)), body, ErrStaleTimestamp},
		{"non-numeric timestamp", "s3cret", headers("abc", Sign("s3cret", "abc", body)), body, errAny},
		{"empty secret", "", headers(ts, Sign("", ts, body)), body, errAny},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifySignature(tt.secret, tt.header, tt.body, now)
			switch {
			case tt.wantErr == nil && err != nil:
				t.Fatalf("expected success, got %v", err)
			case tt.wantErr == errAny && err == nil:
				t.Fatal("expected an error, got nil")
			case tt.wantErr != nil && tt.wantErr != errAny && !errors.Is(err, tt.wantErr):
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

var errAny = errors.New("any error")
