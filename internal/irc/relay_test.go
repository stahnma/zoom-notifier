package irc

import (
	"context"
	"testing"

	"github.com/stahnma/mandatoryFun/zoom-notifier/internal/store"
)

func TestRelay_ValidateConfig_MissingServer(t *testing.T) {
	r := &Relay{}
	cfg := &store.IRCConfig{
		TenantID: "T1",
		Server:   "",
		Nick:     "bot",
		Password: "pass",
	}
	err := r.Send(context.Background(), cfg, "#channel", "test")
	if err == nil {
		t.Error("expected error for missing server")
	}
}

func TestRelay_ValidateConfig_MissingNick(t *testing.T) {
	r := &Relay{}
	cfg := &store.IRCConfig{
		TenantID: "T1",
		Server:   "irc.example.com:6697",
		Nick:     "",
		Password: "pass",
	}
	err := r.Send(context.Background(), cfg, "#channel", "test")
	if err == nil {
		t.Error("expected error for missing nick")
	}
}

func TestRelay_ValidateConfig_MissingChannel(t *testing.T) {
	r := &Relay{}
	cfg := &store.IRCConfig{
		TenantID: "T1",
		Server:   "irc.example.com:6697",
		Nick:     "bot",
		Password: "pass",
	}
	err := r.Send(context.Background(), cfg, "", "test")
	if err == nil {
		t.Error("expected error for missing channel")
	}
}

func TestRelay_ValidateConfig_EmptyMessage(t *testing.T) {
	r := &Relay{}
	cfg := &store.IRCConfig{
		TenantID: "T1",
		Server:   "irc.example.com:6697",
		Nick:     "bot",
		Password: "pass",
	}
	err := r.Send(context.Background(), cfg, "#channel", "")
	if err == nil {
		t.Error("expected error for empty message")
	}
}
