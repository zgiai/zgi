package integrations

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestCredentialCipherRoundTripUsesBoundAAD(t *testing.T) {
	cipher, err := NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("NewCredentialCipher() error = %v", err)
	}
	aad := CredentialAAD{
		OrganizationID:    uuid.New(),
		ConnectionID:      uuid.New(),
		IntegrationID:     IntegrationWebSearch,
		CredentialVersion: 1,
	}
	envelope, err := cipher.EncryptCredentials(map[string]string{"api_key": "private-exa-key-value"}, aad)
	if err != nil {
		t.Fatalf("EncryptCredentials() error = %v", err)
	}
	if !strings.HasPrefix(envelope, credentialEnvelopePrefix) {
		t.Fatalf("envelope = %q, want version prefix", envelope)
	}
	if strings.Contains(envelope, "private-exa-key-value") {
		t.Fatal("credential envelope contains plaintext")
	}
	credentials, err := cipher.DecryptCredentials(envelope, aad)
	if err != nil {
		t.Fatalf("DecryptCredentials() error = %v", err)
	}
	if got := credentials["api_key"]; got != "private-exa-key-value" {
		t.Fatalf("decrypted api_key = %q", got)
	}
	destroyCredentialMap(credentials)

	bindings := []CredentialAAD{
		{OrganizationID: uuid.New(), ConnectionID: aad.ConnectionID, IntegrationID: aad.IntegrationID, CredentialVersion: aad.CredentialVersion},
		{OrganizationID: aad.OrganizationID, ConnectionID: uuid.New(), IntegrationID: aad.IntegrationID, CredentialVersion: aad.CredentialVersion},
		{OrganizationID: aad.OrganizationID, ConnectionID: aad.ConnectionID, IntegrationID: "other-integration", CredentialVersion: aad.CredentialVersion},
		{OrganizationID: aad.OrganizationID, ConnectionID: aad.ConnectionID, IntegrationID: aad.IntegrationID, CredentialVersion: 2},
	}
	for index, binding := range bindings {
		if _, err := cipher.DecryptCredentials(envelope, binding); err == nil {
			t.Fatalf("DecryptCredentials() binding %d error = nil, want AAD rejection", index)
		}
	}
}

func TestCredentialKeyringRotatesWritersAndKeepsOldEnvelopeReaders(t *testing.T) {
	oldKey := "12345678901234567890123456789012"
	newKey := "abcdefghijklmnopqrstuvwxzy123456"
	aad := CredentialAAD{OrganizationID: uuid.New(), ConnectionID: uuid.New(), IntegrationID: "github", CredentialVersion: 3}
	oldRing, err := NewCredentialKeyring("old-2026", map[string]string{"old-2026": oldKey})
	if err != nil {
		t.Fatal(err)
	}
	oldEnvelope, err := oldRing.EncryptCredentials(map[string]string{"access_token": "old-token"}, aad)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(oldEnvelope, "v2.old-2026.") {
		t.Fatalf("old envelope = %q", oldEnvelope)
	}

	rotated, err := NewCredentialKeyring("new-2026", map[string]string{"old-2026": oldKey, "new-2026": newKey})
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := rotated.DecryptCredentials(oldEnvelope, aad)
	if err != nil || credentials["access_token"] != "old-token" {
		t.Fatalf("decrypt old envelope after rotation = %#v, %v", credentials, err)
	}
	destroyCredentialMap(credentials)
	newEnvelope, err := rotated.EncryptCredentials(map[string]string{"access_token": "new-token"}, aad)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(newEnvelope, "v2.new-2026.") {
		t.Fatalf("new envelope = %q", newEnvelope)
	}
	withoutOldKey, err := NewCredentialKeyring("new-2026", map[string]string{"new-2026": newKey})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := withoutOldKey.DecryptCredentials(oldEnvelope, aad); err == nil {
		t.Fatal("old envelope decrypted after its key was removed")
	}
}

func TestCredentialKeyringSupportsDottedKeyIDs(t *testing.T) {
	const keyID = "prod.2026.07"
	keyring, err := NewCredentialKeyring(keyID, map[string]string{
		keyID: "12345678901234567890123456789012",
	})
	if err != nil {
		t.Fatal(err)
	}
	aad := CredentialAAD{OrganizationID: uuid.New(), ConnectionID: uuid.New(), IntegrationID: "github", CredentialVersion: 2}
	envelope, err := keyring.EncryptCredentials(map[string]string{"token": "dotted-key-id-token"}, aad)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(envelope, "v2."+keyID+".") {
		t.Fatalf("envelope = %q", envelope)
	}
	credentials, err := keyring.DecryptCredentials(envelope, aad)
	if err != nil {
		t.Fatalf("DecryptCredentials() error = %v", err)
	}
	if credentials["token"] != "dotted-key-id-token" {
		t.Fatalf("credentials = %#v", credentials)
	}
	destroyCredentialMap(credentials)
}

func TestCredentialKeyringRejectsNormalizedDuplicateKeyIDs(t *testing.T) {
	_, err := NewCredentialKeyring("prod", map[string]string{
		"prod":   "12345678901234567890123456789012",
		" prod ": "abcdefghijklmnopqrstuvwxzy123456",
	})
	if err == nil {
		t.Fatal("NewCredentialKeyring() accepted duplicate normalized key ids")
	}
}

func TestCredentialKeyringReadsLegacyV1EnvelopeDuringMigration(t *testing.T) {
	masterKey := "12345678901234567890123456789012"
	aad := CredentialAAD{OrganizationID: uuid.New(), ConnectionID: uuid.New(), IntegrationID: IntegrationWebSearch, CredentialVersion: 1}
	aead, err := newCredentialAEAD(masterKey)
	if err != nil {
		t.Fatal(err)
	}
	plaintext, _ := json.Marshal(map[string]string{"api_key": "legacy-key"})
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	sealed := aead.Seal(nil, nonce, plaintext, credentialAADBytes(aad))
	legacyEnvelope := legacyCredentialEnvelopePrefix + base64.RawURLEncoding.EncodeToString(append(nonce, sealed...))

	keyring, err := NewCredentialKeyring("active", map[string]string{"active": "abcdefghijklmnopqrstuvwxzy123456", "legacy": masterKey})
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := keyring.DecryptCredentials(legacyEnvelope, aad)
	if err != nil || credentials["api_key"] != "legacy-key" {
		t.Fatalf("decrypt legacy envelope = %#v, %v", credentials, err)
	}
	destroyCredentialMap(credentials)
}

func TestCredentialCipherRejectsInvalidKeyPayloadAndTampering(t *testing.T) {
	for _, key := range []string{"", "too-short", "123456789012345678901234567890123"} {
		if _, err := NewCredentialCipher(key); err == nil {
			t.Fatalf("NewCredentialCipher(%d bytes) error = nil", len(key))
		}
	}
	cipher, err := NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	aad := CredentialAAD{OrganizationID: uuid.New(), ConnectionID: uuid.New(), IntegrationID: IntegrationWebSearch, CredentialVersion: 1}
	if _, err := cipher.EncryptCredentials(nil, aad); err == nil {
		t.Fatal("EncryptCredentials(nil) error = nil")
	}
	envelope, err := cipher.EncryptCredentials(map[string]string{"api_key": "secret-value"}, aad)
	if err != nil {
		t.Fatal(err)
	}
	tampered := envelope[:len(envelope)-1] + "A"
	if tampered == envelope {
		tampered = envelope[:len(envelope)-1] + "B"
	}
	if _, err := cipher.DecryptCredentials(tampered, aad); err == nil {
		t.Fatal("DecryptCredentials(tampered) error = nil")
	}
}

func TestIntegrationConnectionJSONNeverExposesEncryptedEnvelope(t *testing.T) {
	envelope := "v1.super-secret-envelope"
	connection := IntegrationConnection{
		ID:                   uuid.New(),
		OrganizationID:       uuid.New(),
		IntegrationID:        IntegrationWebSearch,
		DriverID:             DriverExa,
		CredentialSource:     ConnectionCredentialSourceOrganization,
		AuthType:             ConnectionAuthTypeAPIKey,
		EncryptedCredentials: &envelope,
	}
	for _, value := range []any{connection, newConnectionView(&connection)} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		if strings.Contains(string(encoded), envelope) || strings.Contains(string(encoded), "encrypted_credentials") {
			t.Fatalf("safe JSON exposed credential envelope: %s", encoded)
		}
	}
}
