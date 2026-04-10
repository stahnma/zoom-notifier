package zoom

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

func ValidateCRC(payload WebhookPayload, secret string) (*CRCResponse, error) {
	if payload.Event != "endpoint.url_validation" {
		return nil, fmt.Errorf("not a CRC validation event: %s", payload.Event)
	}
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(payload.Payload.PlainToken))
	return &CRCResponse{
		PlainToken:     payload.Payload.PlainToken,
		EncryptedToken: hex.EncodeToString(h.Sum(nil)),
	}, nil
}
