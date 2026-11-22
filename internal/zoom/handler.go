package zoom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stahnma/zoom-notifier/internal/metrics"
	"github.com/stahnma/zoom-notifier/internal/store"
)

// Dispatcher is called when a notification-worthy event occurs.
// It receives the tenant ID, the event name, and the full payload.
type Dispatcher func(tenantID string, event string, payload WebhookPayload)

type Handler struct {
	store store.Store
	// fallbackSecret (the global zoom.webhook_secret) verifies webhooks for
	// accounts that have no per-account webhook secret configured.
	fallbackSecret string
	dispatch       Dispatcher
}

func NewHandler(s store.Store, fallbackSecret string, dispatch Dispatcher) *Handler {
	return &Handler{store: s, fallbackSecret: fallbackSecret, dispatch: dispatch}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxWebhookBodyBytes))
	if err != nil {
		log.WithError(err).Warn("failed to read webhook body")
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}

	// Decoding before verification is safe: nothing in the payload is acted
	// on until the signature checks out. The account ID only selects which
	// secret to verify with, and a forged one selects a secret the sender
	// does not have.
	var payload WebhookPayload
	decodeErr := json.Unmarshal(body, &payload)

	candidates, err := h.candidateSecrets(r.Context(), payload, decodeErr == nil)
	if err != nil {
		log.WithError(err).Error("failed to load zoom webhook secrets")
		w.WriteHeader(http.StatusInternalServerError) // Zoom retries on non-2xx
		return
	}

	// Every request, including endpoint.url_validation, must carry a valid
	// Zoom signature. An unsigned CRC handshake would be a signing oracle:
	// sending plainToken "v0:{ts}:{body}" returns a valid signature for body.
	secret, err := verifyWithAny(candidates, r.Header, body, time.Now())
	if err != nil {
		metrics.WebhooksRejected.Inc()
		log.WithError(err).WithFields(log.Fields{
			"remote_addr": r.RemoteAddr,
			"account_id":  payload.Payload.AccountID,
			"event":       payload.Event,
		}).Warn("rejected zoom webhook")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if decodeErr != nil {
		log.WithError(decodeErr).Error("failed to decode webhook payload")
		w.WriteHeader(http.StatusOK) // don't leak errors to Zoom
		return
	}

	// Answer the CRC with the secret that verified the request, i.e. the
	// secret of the Zoom app that sent it.
	if payload.Event == EventURLValidation {
		resp, err := ValidateCRC(payload, secret)
		if err != nil {
			log.WithError(err).Error("CRC validation failed")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.WithError(err).Warn("failed to write CRC response")
		}
		return
	}

	metrics.WebhooksReceived.WithLabelValues(payload.Event).Inc()
	log.WithField("event", payload.Event).Debug("received zoom webhook")
	h.ProcessWebhook(r.Context(), payload)
	w.WriteHeader(http.StatusOK)
}

// candidateSecrets returns the webhook secrets a request may be signed with.
//
//   - Events carry payload.account_id: use that account's webhook secret, or
//     the global fallback if the account has none. The fallback is NOT also
//     accepted for accounts with their own secret, so whoever holds one Zoom
//     app's secret cannot forge events for another account.
//   - endpoint.url_validation carries no account ID, so every configured
//     secret is a candidate; the one that verifies identifies the Zoom app.
//   - An undecodable body can only be checked against the fallback.
func (h *Handler) candidateSecrets(ctx context.Context, payload WebhookPayload, decoded bool) ([]string, error) {
	var secrets []string
	add := func(s string) {
		if s == "" {
			return
		}
		for _, existing := range secrets {
			if existing == s {
				return
			}
		}
		secrets = append(secrets, s)
	}

	switch {
	case !decoded:
		add(h.fallbackSecret)

	case payload.Event == EventURLValidation:
		accounts, err := h.store.ListZoomAccounts(ctx)
		if err != nil {
			return nil, fmt.Errorf("list zoom accounts: %w", err)
		}
		for _, a := range accounts {
			add(a.WebhookSecret)
		}
		add(h.fallbackSecret)

	default:
		account, err := h.store.GetZoomAccount(ctx, payload.Payload.AccountID)
		if err != nil {
			return nil, fmt.Errorf("get zoom account: %w", err)
		}
		if account != nil && account.WebhookSecret != "" {
			add(account.WebhookSecret)
		} else {
			add(h.fallbackSecret)
		}
	}
	return secrets, nil
}

// verifyWithAny returns the first secret that verifies the request.
func verifyWithAny(secrets []string, header http.Header, body []byte, now time.Time) (string, error) {
	if len(secrets) == 0 {
		return "", errors.New("no zoom webhook secret configured for this account")
	}
	var lastErr error
	for _, secret := range secrets {
		err := VerifySignature(secret, header, body, now)
		if err == nil {
			return secret, nil
		}
		// Header and timestamp problems are the same for every secret.
		if errors.Is(err, ErrMissingSignature) || errors.Is(err, ErrStaleTimestamp) {
			return "", err
		}
		lastErr = err
	}
	return "", lastErr
}

// ProcessWebhook records meeting state for the payload's Zoom account and
// dispatches notifications to every tenant linked to that account. State is
// written once per webhook (it is shared by the account's tenants); only
// dispatch fans out per tenant.
func (h *Handler) ProcessWebhook(ctx context.Context, payload WebhookPayload) {
	accountID := payload.Payload.AccountID
	tenants, err := h.store.GetTenantByZoomAccount(ctx, accountID)
	if err != nil {
		log.WithError(err).WithField("account_id", accountID).Error("failed to lookup tenant")
		return
	}
	if len(tenants) == 0 {
		metrics.WebhooksNoTenant.Inc()
		log.WithField("account_id", accountID).Warn("no tenant found for zoom account")
		return
	}

	obj := payload.Payload.Object

	log.WithFields(log.Fields{
		"event":      payload.Event,
		"account_id": accountID,
		"meeting_id": obj.ID,
		"topic":      obj.Topic,
		"tenants":    len(tenants),
	}).Debug("processing webhook event")

	if !h.recordState(ctx, accountID, payload) {
		return
	}

	if h.dispatch == nil {
		return
	}
	for _, tenant := range tenants {
		h.dispatch(tenant.ID, payload.Event, payload)
	}
}

// recordState applies the event to the account's meeting state and reports
// whether the event should be dispatched as a notification.
func (h *Handler) recordState(ctx context.Context, accountID string, payload WebhookPayload) bool {
	obj := payload.Payload.Object
	fields := log.Fields{"meeting_id": obj.ID, "topic": obj.Topic, "account_id": accountID}

	switch payload.Event {
	case EventMeetingStarted:
		log.WithFields(fields).Debug("meeting started")
		if err := h.store.UpsertMeeting(ctx, meetingFromPayload(accountID, payload)); err != nil {
			log.WithError(err).Error("failed to upsert meeting on start")
		}
		return false

	case EventMeetingEnded:
		log.WithFields(fields).Debug("meeting ended")
		if err := h.store.DeleteParticipantsForMeeting(ctx, accountID, obj.ID); err != nil {
			log.WithError(err).Error("failed to delete participants on meeting end")
		}
		if err := h.store.DeleteMeeting(ctx, accountID, obj.ID); err != nil {
			log.WithError(err).Error("failed to delete meeting on end")
		}
		return false

	case EventParticipantJoined:
		log.WithFields(fields).WithField("user", obj.Participant.UserName).Debug("participant joined")
		// Ensure meeting exists (we may have missed meeting.started).
		if err := h.store.UpsertMeeting(ctx, meetingFromPayload(accountID, payload)); err != nil {
			log.WithError(err).Error("failed to upsert meeting on join")
		}
		p := &store.Participant{
			ZoomAccountID: accountID,
			MeetingID:     obj.ID,
			UserName:      obj.Participant.UserName,
			Email:         obj.Participant.Email,
			JoinTime:      obj.Participant.JoinTime,
		}
		if err := h.store.AddParticipant(ctx, p); err != nil {
			log.WithError(err).Error("failed to add participant")
		}
		return true

	case EventParticipantLeft:
		log.WithFields(fields).WithField("user", obj.Participant.UserName).Debug("participant left")
		leaveTime := time.Now()
		if !obj.Participant.LeaveTime.IsZero() {
			leaveTime = obj.Participant.LeaveTime
		}
		if err := h.store.SetParticipantLeft(ctx, accountID, obj.ID, obj.Participant.UserName, &leaveTime); err != nil {
			log.WithError(err).Error("failed to set participant left")
		}
		return true

	default:
		log.WithField("event", payload.Event).Debug("ignoring unhandled event")
		return false
	}
}

func meetingFromPayload(accountID string, payload WebhookPayload) *store.ActiveMeeting {
	obj := payload.Payload.Object
	return &store.ActiveMeeting{
		ZoomAccountID: accountID,
		MeetingID:     obj.ID,
		Topic:         obj.Topic,
		HostID:        obj.HostID,
		StartTime:     obj.StartTime,
	}
}
