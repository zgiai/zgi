package container

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/zgiai/zgi/api/config"
	credentialcrypto "github.com/zgiai/zgi/api/internal/infra/credentialcrypto"
)

const (
	testDigitalHumanLegacyMasterKey    = "12345678901234567890123456789012"
	testDigitalHumanConnectorMasterKey = "cccccccccccccccccccccccccccccccc"
)

func digitalHumanCredentialTestAAD() credentialcrypto.CredentialAAD {
	return credentialcrypto.CredentialAAD{
		OrganizationID:    uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		ConnectionID:      uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		IntegrationID:     "dingtalk",
		CredentialVersion: 1,
	}
}

// TestDigitalHumanCredentialCipherUsesLegacyKeyringNotConnectorKeys pins the
// purpose separation: historical DigitalHuman channel envelopes stay readable
// through the dedicated keyring, and the Connector c1 key can never substitute
// for the legacy DigitalHuman key.
func TestDigitalHumanCredentialCipherUsesLegacyKeyringNotConnectorKeys(t *testing.T) {
	container := &ServiceContainer{config: &config.Config{
		Connectors: config.ConnectorsConfig{
			CredentialActiveKeyID: "connector-primary",
			CredentialKeys:        map[string]string{"connector-primary": testDigitalHumanConnectorMasterKey},
		},
		DigitalHumanCredentials: config.DigitalHumanCredentialsConfig{
			ActiveKeyID: "legacy",
			Keys:        map[string]string{"legacy": testDigitalHumanLegacyMasterKey},
		},
	}}

	cipher := container.GetDigitalHumanCredentialCipher()
	if cipher == nil {
		t.Fatal("DigitalHuman credential cipher is nil")
	}
	aad := digitalHumanCredentialTestAAD()

	// An envelope written by the pre-Connector container under the literal
	// "legacy" key id must remain readable after the Connector keyring exists.
	historical, err := credentialcrypto.NewCredentialKeyring(
		"legacy", map[string]string{"legacy": testDigitalHumanLegacyMasterKey},
	)
	if err != nil {
		t.Fatalf("historical keyring error = %v", err)
	}
	envelope, err := historical.EncryptCredentials(map[string]string{"app_secret": "legacy-secret"}, aad)
	if err != nil {
		t.Fatalf("historical EncryptCredentials() error = %v", err)
	}
	credentials, err := cipher.DecryptCredentials(envelope, aad)
	if err != nil {
		t.Fatalf("DecryptCredentials() error = %v", err)
	}
	if credentials["app_secret"] != "legacy-secret" {
		t.Fatalf("decrypted app_secret = %q", credentials["app_secret"])
	}

	// New DigitalHuman envelopes stay bound to the DigitalHuman active key.
	fresh, err := cipher.EncryptCredentials(map[string]string{"app_secret": "fresh-secret"}, aad)
	if err != nil {
		t.Fatalf("EncryptCredentials() error = %v", err)
	}
	if !strings.HasPrefix(fresh, "v2.legacy.") {
		t.Fatalf("new envelope = %q, want v2.legacy prefix", fresh)
	}

	// The Connector c1 keyring must not open DigitalHuman envelopes, and the
	// DigitalHuman cipher must not open Connector envelopes.
	connectorCipher, err := credentialcrypto.NewCredentialKeyring(
		"connector-primary", map[string]string{"connector-primary": testDigitalHumanConnectorMasterKey},
	)
	if err != nil {
		t.Fatalf("Connector keyring error = %v", err)
	}
	if _, err := connectorCipher.DecryptCredentials(envelope, aad); err == nil {
		t.Fatal("Connector keyring decrypted a DigitalHuman envelope")
	}
	connectorEnvelope, err := connectorCipher.EncryptCredentials(map[string]string{"app_secret": "connector-secret"}, aad)
	if err != nil {
		t.Fatalf("Connector EncryptCredentials() error = %v", err)
	}
	if _, err := cipher.DecryptCredentials(connectorEnvelope, aad); err == nil {
		t.Fatal("DigitalHuman cipher decrypted a Connector envelope")
	}
}

// TestDigitalHumanCredentialCipherFailsClosedWithoutDigitalHumanKeys ensures a
// deployment that only configured Connector c1 key material cannot silently
// start encrypting DigitalHuman channel credentials under the wrong keyring.
func TestDigitalHumanCredentialCipherFailsClosedWithoutDigitalHumanKeys(t *testing.T) {
	connectorsOnly := &ServiceContainer{config: &config.Config{
		Connectors: config.ConnectorsConfig{
			CredentialActiveKeyID: "connector-primary",
			CredentialKeys:        map[string]string{"connector-primary": testDigitalHumanConnectorMasterKey},
		},
	}}
	unconfigured := &ServiceContainer{}
	aad := digitalHumanCredentialTestAAD()

	for name, container := range map[string]*ServiceContainer{
		"connector-only": connectorsOnly,
		"nil-config":     unconfigured,
	} {
		t.Run(name, func(t *testing.T) {
			cipher := container.GetDigitalHumanCredentialCipher()
			if _, err := cipher.EncryptCredentials(map[string]string{"app_secret": "secret"}, aad); !errors.Is(err, credentialcrypto.ErrCredentialCipherUnavailable) {
				t.Fatalf("EncryptCredentials() error = %v, want unavailable", err)
			}
			if _, err := cipher.DecryptCredentials("v2.legacy.anything", aad); !errors.Is(err, credentialcrypto.ErrCredentialCipherUnavailable) {
				t.Fatalf("DecryptCredentials() error = %v, want unavailable", err)
			}
			if container.digitalHumanCredentialCipher != nil {
				t.Fatal("unavailable cipher was cached on the container")
			}
		})
	}
}
