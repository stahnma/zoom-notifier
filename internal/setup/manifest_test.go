package setup

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGenerateSlackManifest(t *testing.T) {
	serverURL := "https://example.com"
	manifest := GenerateSlackManifest(serverURL)

	// Must be valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(manifest), &parsed); err != nil {
		t.Fatalf("GenerateSlackManifest returned invalid JSON: %v", err)
	}

	// Must contain required top-level keys
	for _, key := range []string{"display_information", "features", "oauth_config", "settings"} {
		if _, ok := parsed[key]; !ok {
			t.Errorf("manifest missing required key %q", key)
		}
	}

	// Must contain server URL in redirect_urls
	if !strings.Contains(manifest, serverURL+"/slack/callback") {
		t.Error("manifest does not contain server URL in redirect_urls")
	}

	// Must contain server URL in slash command url
	if !strings.Contains(manifest, serverURL+"/slack/commands") {
		t.Error("manifest does not contain server URL in slash command url")
	}

	// Must contain interactivity URL
	if !strings.Contains(manifest, serverURL+"/slack/interactions") {
		t.Error("manifest does not contain interactivity URL")
	}

	// Verify interactivity is enabled
	settings := parsed["settings"].(map[string]interface{})
	interactivity := settings["interactivity"].(map[string]interface{})
	if interactivity["is_enabled"] != true {
		t.Error("interactivity should be enabled")
	}

	// Verify display_information values
	di := parsed["display_information"].(map[string]interface{})
	if di["name"] != "zoom-notifier" {
		t.Errorf("expected display name %q, got %q", "zoom-notifier", di["name"])
	}

	// Verify bot scopes
	oauth := parsed["oauth_config"].(map[string]interface{})
	scopes := oauth["scopes"].(map[string]interface{})
	botScopes := scopes["bot"].([]interface{})
	expectedScopes := map[string]bool{"commands": true, "chat:write": true, "chat:write.public": true, "channels:read": true, "groups:read": true}
	for _, s := range botScopes {
		delete(expectedScopes, s.(string))
	}
	if len(expectedScopes) > 0 {
		t.Errorf("missing bot scopes: %v", expectedScopes)
	}
}

func TestSlackManifestURL(t *testing.T) {
	serverURL := "https://example.com"
	u := SlackManifestURL(serverURL)

	if u == "" {
		t.Fatal("SlackManifestURL returned empty string")
	}

	if !strings.HasPrefix(u, "https://api.slack.com/apps") {
		t.Errorf("URL does not start with api.slack.com base: %s", u)
	}

	if !strings.Contains(u, "manifest_json=") {
		t.Error("URL does not contain manifest_json parameter")
	}

	// The URL-encoded manifest should contain the server URL (encoded)
	if !strings.Contains(u, "example.com") {
		t.Error("URL does not contain the server URL")
	}
}
