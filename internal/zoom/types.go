package zoom

import "time"

// Zoom webhook event types.
const (
	EventURLValidation     = "endpoint.url_validation"
	EventMeetingStarted    = "meeting.started"
	EventMeetingEnded      = "meeting.ended"
	EventParticipantJoined = "meeting.participant_joined"
	EventParticipantLeft   = "meeting.participant_left"
)

type WebhookPayload struct {
	Payload struct {
		PlainToken string `json:"plainToken"`
		AccountID  string `json:"account_id"`
		Object     struct {
			UUID        string    `json:"uuid"`
			ID          string    `json:"id"`
			Type        int       `json:"type"`
			Topic       string    `json:"topic"`
			HostID      string    `json:"host_id"`
			Duration    int       `json:"duration"`
			StartTime   time.Time `json:"start_time"`
			Timezone    string    `json:"timezone"`
			Participant struct {
				UserID            string    `json:"user_id"`
				UserName          string    `json:"user_name"`
				Email             string    `json:"email"`
				JoinTime          time.Time `json:"join_time"`
				LeaveTime         time.Time `json:"leave_time"`
				LeaveReason       string    `json:"leave_reason"`
				ID                string    `json:"id"`
				ParticipantUserID string    `json:"participant_user_id"`
				ParticipantUUID   string    `json:"participant_uuid"`
				RegistrantID      string    `json:"registrant_id"`
			} `json:"participant"`
		} `json:"object"`
	} `json:"payload"`
	EventTs int64  `json:"event_ts"`
	Event   string `json:"event"`
}

type CRCResponse struct {
	PlainToken     string `json:"plainToken"`
	EncryptedToken string `json:"encryptedToken"`
}
