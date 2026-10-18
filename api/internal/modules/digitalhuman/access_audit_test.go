package digitalhuman

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAccessAuditDetailsExcludeNamesCredentialsAndExternalUsers(t *testing.T) {
	name := "customer@example.test secret key label"
	status := APIKeyStatusRevoked
	expiresAt := time.Now().UTC().Add(time.Hour)
	details := apiKeyUpdateAuditDetails(
		&APIKey{Status: APIKeyStatusActive, Name: "old private label"},
		&APIKey{Status: APIKeyStatusRevoked, Name: name},
		APIKeyUpdateRequest{Name: &name, Status: &status, ExpiresAt: &expiresAt},
	)
	encoded, err := json.Marshal(details)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, forbidden := range []string{name, "old private label", "customer@example.test", "api_key", "user"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("audit details leaked %q: %s", forbidden, text)
		}
	}
	for _, required := range []string{"name_changed", "status", "expires_at_changed"} {
		if !strings.Contains(text, required) {
			t.Fatalf("audit details are missing %q: %s", required, text)
		}
	}
}

func TestAccessSettingsAuditDetailsContainOnlyRequestedTransitions(t *testing.T) {
	webAppEnabled := true
	details := accessSettingsAuditDetails(
		AccessSettings{WebAppEnabled: false, APIEnabled: true},
		AccessSettings{WebAppEnabled: true, APIEnabled: true},
		AccessSettingsUpdate{WebAppEnabled: &webAppEnabled},
	)
	if _, exists := details["api_enabled"]; exists {
		t.Fatalf("unrequested API transition was audited: %#v", details)
	}
	transition, ok := details["webapp_enabled"].(map[string]bool)
	if !ok || transition["from"] || !transition["to"] {
		t.Fatalf("webapp transition = %#v", details["webapp_enabled"])
	}
}
