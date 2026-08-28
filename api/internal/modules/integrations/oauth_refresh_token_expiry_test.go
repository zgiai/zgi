package integrations

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestConnectionViewExposesRefreshTokenExpiryWithoutCredentialMaterial(t *testing.T) {
	refreshExpiry := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Microsecond)
	envelope := "v2.encrypted-secret-material"
	connection := &IntegrationConnection{
		EncryptedCredentials:  &envelope,
		RefreshTokenExpiresAt: &refreshExpiry,
	}
	view := newConnectionView(connection)
	refreshExpiry = refreshExpiry.Add(time.Hour)

	if view.RefreshTokenExpiresAt == nil ||
		view.RefreshTokenExpiresAt.Equal(refreshExpiry) {
		t.Fatalf("connection view did not clone refresh token expiry: %#v", view.RefreshTokenExpiresAt)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"refresh_token_expires_at"`) {
		t.Fatalf("connection view omitted refresh token expiry: %s", encoded)
	}
	if strings.Contains(string(encoded), envelope) {
		t.Fatalf("connection view exposed encrypted credential material: %s", encoded)
	}
}
