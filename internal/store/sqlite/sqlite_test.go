package sqlite

import (
	"testing"
)

func TestNewInMemory(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()
}

func TestRunMigrations(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()

	if err := s.Migrate(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Verify tables exist by querying them
	tables := []string{"tenants", "tenant_admins", "subscriptions", "meeting_filters",
		"active_meetings", "participants", "irc_configs", "zoom_credentials"}
	for _, table := range tables {
		_, err := s.db.Exec("SELECT 1 FROM " + table + " LIMIT 1")
		if err != nil {
			t.Errorf("table %s not found: %v", table, err)
		}
	}
}
