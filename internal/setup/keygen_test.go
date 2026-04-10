package setup

import (
	"testing"
)

func TestGenerateAPIKey_NoError(t *testing.T) {
	key, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if key == "" {
		t.Fatal("expected non-empty key")
	}
}

func TestGenerateAPIKey_Length(t *testing.T) {
	key, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(key) != 64 {
		t.Fatalf("expected key length 64, got %d", len(key))
	}
}

func TestGenerateAPIKey_Uniqueness(t *testing.T) {
	key1, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	key2, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key1 == key2 {
		t.Fatal("expected two generated keys to be different")
	}
}
