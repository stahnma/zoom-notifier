package zoom

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store"
)

// Dispatcher is called when a notification-worthy event occurs.
// It receives the tenant ID, the event name, and the full payload.
type Dispatcher func(tenantID string, event string, payload WebhookPayload)

type Handler struct {
	store         store.Store
	webhookSecret string
	dispatch      Dispatcher
}

func NewHandler(s store.Store, webhookSecret string, dispatch Dispatcher) *Handler {
	return &Handler{store: s, webhookSecret: webhookSecret, dispatch: dispatch}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var payload WebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.WithError(err).Error("failed to decode webhook payload")
		w.WriteHeader(http.StatusOK) // don't leak errors to Zoom
		return
	}

	// Handle CRC validation (no tenant resolution needed)
	if payload.Event == "endpoint.url_validation" {
		resp, err := ValidateCRC(payload, h.webhookSecret)
		if err != nil {
			log.WithError(err).Error("CRC validation failed")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	h.ProcessWebhook(r.Context(), payload)
	w.WriteHeader(http.StatusOK)
}

// ProcessWebhook handles tenant resolution and event processing for a parsed payload.
// It is used by both the direct HTTP handler and the API server's strict handler.
func (h *Handler) ProcessWebhook(ctx context.Context, payload WebhookPayload) {
	accountID := payload.Payload.AccountID
	tenants, err := h.store.GetTenantByZoomAccount(ctx, accountID)
	if err != nil {
		log.WithError(err).WithField("account_id", accountID).Error("failed to lookup tenant")
		return
	}
	if len(tenants) == 0 {
		log.WithField("account_id", accountID).Warn("no tenant found for zoom account")
		return
	}

	obj := payload.Payload.Object

	for _, tenant := range tenants {
		switch payload.Event {
		case "meeting.started":
			meeting := &store.ActiveMeeting{
				MeetingID: obj.ID,
				TenantID:  tenant.ID,
				Topic:     obj.Topic,
				HostID:    obj.HostID,
				StartTime: obj.StartTime,
			}
			if err := h.store.UpsertMeeting(ctx, meeting); err != nil {
				log.WithError(err).Error("failed to upsert meeting on start")
			}

		case "meeting.ended":
			if err := h.store.DeleteParticipantsForMeeting(ctx, obj.ID); err != nil {
				log.WithError(err).Error("failed to delete participants on meeting end")
			}
			if err := h.store.DeleteMeeting(ctx, obj.ID); err != nil {
				log.WithError(err).Error("failed to delete meeting on end")
			}

		case "meeting.participant_joined":
			// Ensure meeting exists
			meeting := &store.ActiveMeeting{
				MeetingID: obj.ID,
				TenantID:  tenant.ID,
				Topic:     obj.Topic,
				HostID:    obj.HostID,
				StartTime: obj.StartTime,
			}
			if err := h.store.UpsertMeeting(ctx, meeting); err != nil {
				log.WithError(err).Error("failed to upsert meeting on join")
			}

			p := &store.Participant{
				MeetingID: obj.ID,
				UserName:  obj.Participant.UserName,
				Email:     obj.Participant.Email,
				JoinTime:  obj.Participant.JoinTime,
			}
			if err := h.store.AddParticipant(ctx, p); err != nil {
				log.WithError(err).Error("failed to add participant")
			}

			if h.dispatch != nil {
				h.dispatch(tenant.ID, payload.Event, payload)
			}

		case "meeting.participant_left":
			leaveTime := time.Now()
			if !obj.Participant.LeaveTime.IsZero() {
				leaveTime = obj.Participant.LeaveTime
			}
			if err := h.store.SetParticipantLeft(ctx, obj.ID, obj.Participant.UserName, &leaveTime); err != nil {
				log.WithError(err).Error("failed to set participant left")
			}

			if h.dispatch != nil {
				h.dispatch(tenant.ID, payload.Event, payload)
			}

		default:
			log.WithField("event", payload.Event).Debug("ignoring unhandled event")
		}
	}
}
