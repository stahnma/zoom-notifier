package slack

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	log "github.com/sirupsen/logrus"
	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store"
)

// InteractionHandler handles Slack interactive component payloads (e.g. modal submissions).
type InteractionHandler struct {
	store         store.Store
	signingSecret string
	handlers      map[string]ViewSubmissionHandler
}

// ViewSubmissionHandler processes a view_submission interaction payload.
type ViewSubmissionHandler func(payload InteractionPayload) error

// InteractionPayload represents the JSON payload Slack sends for interactions.
type InteractionPayload struct {
	Type string `json:"type"`
	User struct {
		ID     string `json:"id"`
		TeamID string `json:"team_id"`
	} `json:"user"`
	Team struct {
		ID string `json:"id"`
	} `json:"team"`
	View struct {
		CallbackID      string     `json:"callback_id"`
		State           *ViewState `json:"state"`
		PrivateMetadata string     `json:"private_metadata"`
	} `json:"view"`
}

// ViewState represents the state of a modal view's input elements.
type ViewState struct {
	Values map[string]map[string]ViewStateValue `json:"values"`
}

// ViewStateValue represents the value of a single input element in a modal.
type ViewStateValue struct {
	Type                 string          `json:"type"`
	Value                *string         `json:"value"`
	Selected             *SelectedOption `json:"selected_option"`
	SelectedConversation *string         `json:"selected_conversation"`
}

// SelectedOption represents a selected option in a static select or radio button.
type SelectedOption struct {
	Value string `json:"value"`
}

// NewInteractionHandler creates a new handler for Slack interaction payloads.
func NewInteractionHandler(s store.Store, signingSecret string) *InteractionHandler {
	return &InteractionHandler{
		store:         s,
		signingSecret: signingSecret,
		handlers:      make(map[string]ViewSubmissionHandler),
	}
}

// RegisterHandler registers a ViewSubmissionHandler for a given callback_id.
func (h *InteractionHandler) RegisterHandler(callbackID string, fn ViewSubmissionHandler) {
	h.handlers[callbackID] = fn
}

// ServeHTTP handles incoming Slack interaction requests.
func (h *InteractionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if !verifySlackSignature(h.signingSecret, r.Header, body) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	// Slack sends: payload=<url-encoded-json>
	formValues, err := url.ParseQuery(string(body))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	payloadStr := formValues.Get("payload")

	var payload InteractionPayload
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		log.WithError(err).Error("failed to parse interaction payload")
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}

	log.WithFields(log.Fields{
		"type":        payload.Type,
		"callback_id": payload.View.CallbackID,
		"user_id":     payload.User.ID,
	}).Debug("received interaction payload")

	if payload.Type == "view_submission" {
		callbackID := payload.View.CallbackID
		if fn, ok := h.handlers[callbackID]; ok {
			if err := fn(payload); err != nil {
				log.WithError(err).WithField("callback_id", callbackID).Error("interaction handler failed")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		} else {
			log.WithField("callback_id", callbackID).Warn("no handler for callback_id")
		}
	}

	w.WriteHeader(http.StatusOK)
}
