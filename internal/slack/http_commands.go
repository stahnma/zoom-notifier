package slack

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"

	log "github.com/sirupsen/logrus"
	"strconv"
	"time"
)

// HTTPCommandHandler receives Slack slash commands via HTTP POST
// and verifies them using the Slack signing secret.
type HTTPCommandHandler struct {
	handler       *CommandHandler
	signingSecret string
}

// NewHTTPCommandHandler creates a new HTTP handler for Slack slash commands.
func NewHTTPCommandHandler(handler *CommandHandler, signingSecret string) *HTTPCommandHandler {
	return &HTTPCommandHandler{
		handler:       handler,
		signingSecret: signingSecret,
	}
}

// ServeHTTP handles incoming Slack slash command requests.
func (h *HTTPCommandHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer func() { _ = r.Body.Close() }()

	// Verify the Slack signing secret
	if !h.verifySignature(r.Header, body) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	// Parse URL-encoded form body
	values, err := url.ParseQuery(string(body))
	if err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}

	cmd := SlashCommand{
		TeamID:    values.Get("team_id"),
		UserID:    values.Get("user_id"),
		ChannelID: values.Get("channel_id"),
		Text:      values.Get("text"),
		TriggerID: values.Get("trigger_id"),
	}

	log.WithFields(log.Fields{
		"team_id":    cmd.TeamID,
		"user_id":    cmd.UserID,
		"text":       cmd.Text,
		"trigger_id": cmd.TriggerID != "",
	}).Debug("received slash command")

	resp, err := h.handler.Handle(r.Context(), cmd)
	if err != nil {
		log.WithError(err).Error("slash command handler failed")
		// Slack requires 200 — return ephemeral error message
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"response_type": "ephemeral",
			"text":          "Something went wrong. Please try again.",
		})
		return
	}

	// If text is empty (e.g. modal was opened), return empty 200
	if resp.Text == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	log.WithFields(log.Fields{
		"response_type": resp.ResponseType,
		"text_length":   len(resp.Text),
	}).Debug("sending slash command response")

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"response_type": resp.ResponseType,
		"text":          resp.Text,
	})
}

// verifySignature checks the Slack request signature using HMAC-SHA256.
func (h *HTTPCommandHandler) verifySignature(headers http.Header, body []byte) bool {
	return verifySlackSignature(h.signingSecret, headers, body)
}

// verifySlackSignature is a package-level function that verifies a Slack request
// signature using HMAC-SHA256. Both the command handler and interaction handler use this.
func verifySlackSignature(secret string, headers http.Header, body []byte) bool {
	timestamp := headers.Get("X-Slack-Request-Timestamp")
	signature := headers.Get("X-Slack-Signature")

	if timestamp == "" || signature == "" {
		return false
	}

	// Reject requests older than 5 minutes (replay protection)
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if math.Abs(float64(time.Now().Unix()-ts)) > 300 {
		return false
	}

	// Compute basestring: v0:{timestamp}:{body}
	baseString := fmt.Sprintf("v0:%s:%s", timestamp, string(body))

	// HMAC-SHA256 with signing secret
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(baseString))
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))

	// Constant-time comparison
	return hmac.Equal([]byte(expected), []byte(signature))
}
