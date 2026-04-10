package zoom

import (
	"encoding/json"
	"os"
	"testing"
)

func TestValidateCRC(t *testing.T) {
	data, err := os.ReadFile("../../examples/zoom/endpoint_url_validation.json")
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}
	var payload WebhookPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if payload.Event != "endpoint.url_validation" {
		t.Fatalf("expected endpoint.url_validation event, got %s", payload.Event)
	}

	resp, err := ValidateCRC(payload, "test-secret")
	if err != nil {
		t.Fatalf("validate crc: %v", err)
	}
	if resp.PlainToken != payload.Payload.PlainToken {
		t.Errorf("plain token mismatch")
	}
	if resp.EncryptedToken == "" {
		t.Error("encrypted token should not be empty")
	}
}

func TestValidateCRC_NotValidationEvent(t *testing.T) {
	payload := WebhookPayload{}
	payload.Event = "meeting.participant_joined"

	_, err := ValidateCRC(payload, "secret")
	if err == nil {
		t.Error("expected error for non-validation event")
	}
}

func TestValidateCRC_DeterministicOutput(t *testing.T) {
	payload := WebhookPayload{}
	payload.Event = "endpoint.url_validation"
	payload.Payload.PlainToken = "known-token"

	resp1, _ := ValidateCRC(payload, "same-secret")
	resp2, _ := ValidateCRC(payload, "same-secret")
	if resp1.EncryptedToken != resp2.EncryptedToken {
		t.Error("same input should produce same output")
	}

	resp3, _ := ValidateCRC(payload, "different-secret")
	if resp1.EncryptedToken == resp3.EncryptedToken {
		t.Error("different secrets should produce different output")
	}
}

func TestParseParticipantJoined(t *testing.T) {
	data, err := os.ReadFile("../../examples/zoom/participant_joined.json")
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}
	var payload WebhookPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if payload.Event != "meeting.participant_joined" {
		t.Errorf("expected meeting.participant_joined, got %s", payload.Event)
	}
	if payload.Payload.AccountID != "uUpLA0YDRhWZvYIu_JxPpg" {
		t.Errorf("unexpected account_id: %s", payload.Payload.AccountID)
	}
	if payload.Payload.Object.Participant.UserName != "Jonny Bananas" {
		t.Errorf("unexpected user_name: %s", payload.Payload.Object.Participant.UserName)
	}
	if payload.Payload.Object.Topic != "Somebody's Personal Meeting Room" {
		t.Errorf("unexpected topic: %s", payload.Payload.Object.Topic)
	}
}

func TestParseParticipantLeft(t *testing.T) {
	data, err := os.ReadFile("../../examples/zoom/participant_left.json")
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}
	var payload WebhookPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if payload.Event != "meeting.participant_left" {
		t.Errorf("expected meeting.participant_left, got %s", payload.Event)
	}
}
