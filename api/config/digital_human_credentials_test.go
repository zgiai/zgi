package config

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/zgiai/zgi/api/internal/infra/credentialcrypto"
)

const (
	testDigitalHumanLegacyKey = "12345678901234567890123456789012"
	testDigitalHumanActiveKey = "abcdefghijklmnopqrstuvwxzy123456"
)

func digitalHumanEnvSource(values map[string]string) *envSource {
	return &envSource{lookupEnv: func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}}
}

func emptyDigitalHumanEnvSource() *envSource {
	return digitalHumanEnvSource(nil)
}

func digitalHumanCipherFromConfig(t *testing.T, cfg *Config) credentialcrypto.CredentialCipher {
	t.Helper()
	cipher, err := credentialcrypto.NewCredentialKeyring(
		cfg.DigitalHumanCredentials.ActiveKeyID,
		cfg.DigitalHumanCredentials.Keys,
	)
	if err != nil {
		t.Fatalf("NewCredentialKeyring() error = %v", err)
	}
	return cipher
}

func TestLoadDigitalHumanCredentialsRetainsLegacyAPIKeyEncryptionKey(t *testing.T) {
	cfg := &Config{Encryption: EncryptionConfig{APIKeyEncryptionKey: testDigitalHumanLegacyKey}}
	if err := loadDigitalHumanCredentialsConfig(cfg, emptyDigitalHumanEnvSource()); err != nil {
		t.Fatalf("loadDigitalHumanCredentialsConfig() error = %v", err)
	}
	if cfg.DigitalHumanCredentials.ActiveKeyID != "legacy" {
		t.Fatalf("active key id = %q, want legacy", cfg.DigitalHumanCredentials.ActiveKeyID)
	}
	if got := cfg.DigitalHumanCredentials.Keys["legacy"]; got != testDigitalHumanLegacyKey {
		t.Fatalf("legacy key = %q, want API_KEY_ENCRYPTION_KEY", got)
	}

	// The previous container wrote envelopes under the literal "legacy" key id.
	// They must stay readable after loading the migrated configuration.
	oldCipher, err := credentialcrypto.NewCredentialKeyring("legacy", map[string]string{"legacy": testDigitalHumanLegacyKey})
	if err != nil {
		t.Fatalf("legacy keyring error = %v", err)
	}
	aad := credentialcrypto.CredentialAAD{
		OrganizationID:    uuid.New(),
		ConnectionID:      uuid.New(),
		IntegrationID:     "dingtalk",
		CredentialVersion: 1,
	}
	envelope, err := oldCipher.EncryptCredentials(map[string]string{"app_secret": "old-secret"}, aad)
	if err != nil {
		t.Fatalf("EncryptCredentials() error = %v", err)
	}
	credentials, err := digitalHumanCipherFromConfig(t, cfg).DecryptCredentials(envelope, aad)
	if err != nil {
		t.Fatalf("DecryptCredentials() error = %v", err)
	}
	if credentials["app_secret"] != "old-secret" {
		t.Fatalf("decrypted app_secret = %q", credentials["app_secret"])
	}
}

func TestLoadDigitalHumanCredentialsAddsLegacyKeyToConfiguredKeyring(t *testing.T) {
	cfg := &Config{Encryption: EncryptionConfig{APIKeyEncryptionKey: testDigitalHumanLegacyKey}}
	source := digitalHumanEnvSource(map[string]string{
		envDigitalHumanCredentialKeysJSON:    `{"primary":"` + testDigitalHumanActiveKey + `"}`,
		envDigitalHumanCredentialActiveKeyID: "primary",
	})
	if err := loadDigitalHumanCredentialsConfig(cfg, source); err != nil {
		t.Fatalf("loadDigitalHumanCredentialsConfig() error = %v", err)
	}
	if cfg.DigitalHumanCredentials.ActiveKeyID != "primary" {
		t.Fatalf("active key id = %q, want primary", cfg.DigitalHumanCredentials.ActiveKeyID)
	}
	if got := cfg.DigitalHumanCredentials.Keys["legacy"]; got != testDigitalHumanLegacyKey {
		t.Fatalf("legacy key = %q, want API_KEY_ENCRYPTION_KEY", got)
	}
	if got := cfg.DigitalHumanCredentials.Keys["primary"]; got != testDigitalHumanActiveKey {
		t.Fatalf("primary key = %q", got)
	}

	cipher := digitalHumanCipherFromConfig(t, cfg)
	aad := credentialcrypto.CredentialAAD{
		OrganizationID:    uuid.New(),
		ConnectionID:      uuid.New(),
		IntegrationID:     "feishu",
		CredentialVersion: 2,
	}
	envelope, err := cipher.EncryptCredentials(map[string]string{"app_secret": "rotated"}, aad)
	if err != nil {
		t.Fatalf("EncryptCredentials() error = %v", err)
	}
	if !strings.HasPrefix(envelope, "v2.primary.") {
		t.Fatalf("new envelope = %q, want active key id prefix", envelope)
	}
}

func TestLoadDigitalHumanCredentialsNeverInheritsConnectorKeyring(t *testing.T) {
	connectorKey := strings.Repeat("c", 32)
	cfg := &Config{
		Connectors: ConnectorsConfig{
			CredentialActiveKeyID: "legacy",
			CredentialKeys:        map[string]string{"legacy": connectorKey},
		},
	}
	if err := loadDigitalHumanCredentialsConfig(cfg, emptyDigitalHumanEnvSource()); err != nil {
		t.Fatalf("loadDigitalHumanCredentialsConfig() error = %v", err)
	}
	if cfg.DigitalHumanCredentials.ActiveKeyID != "" || len(cfg.DigitalHumanCredentials.Keys) != 0 {
		t.Fatalf("DigitalHuman keyring inherited Connector keys: %#v", cfg.DigitalHumanCredentials)
	}

	// Even when both keyrings are present, the DigitalHuman "legacy" key must
	// stay bound to API_KEY_ENCRYPTION_KEY rather than the Connector entry.
	cfg = &Config{
		Connectors: ConnectorsConfig{
			CredentialActiveKeyID: "legacy",
			CredentialKeys:        map[string]string{"legacy": connectorKey},
		},
		Encryption: EncryptionConfig{APIKeyEncryptionKey: testDigitalHumanLegacyKey},
	}
	if err := loadDigitalHumanCredentialsConfig(cfg, emptyDigitalHumanEnvSource()); err != nil {
		t.Fatalf("loadDigitalHumanCredentialsConfig() error = %v", err)
	}
	if got := cfg.DigitalHumanCredentials.Keys["legacy"]; got != testDigitalHumanLegacyKey {
		t.Fatalf("DigitalHuman legacy key = %q, want API_KEY_ENCRYPTION_KEY, not the Connector key", got)
	}
}

func TestLoadDigitalHumanCredentialsFallsBackToDefaultKey(t *testing.T) {
	cfg := &Config{}
	source := digitalHumanEnvSource(map[string]string{
		envDigitalHumanCredentialKeysJSON: `{"default":"` + testDigitalHumanActiveKey + `"}`,
	})
	if err := loadDigitalHumanCredentialsConfig(cfg, source); err != nil {
		t.Fatalf("loadDigitalHumanCredentialsConfig() error = %v", err)
	}
	if cfg.DigitalHumanCredentials.ActiveKeyID != "default" {
		t.Fatalf("active key id = %q, want default", cfg.DigitalHumanCredentials.ActiveKeyID)
	}
}

func TestLoadDigitalHumanCredentialsRejectsInvalidKeyring(t *testing.T) {
	cases := []struct {
		name   string
		cfg    *Config
		values map[string]string
	}{
		{
			name:   "invalid JSON",
			values: map[string]string{envDigitalHumanCredentialKeysJSON: `["not","object"]`},
		},
		{
			name:   "null JSON",
			values: map[string]string{envDigitalHumanCredentialKeysJSON: `null`},
		},
		{
			name:   "short key",
			values: map[string]string{envDigitalHumanCredentialKeysJSON: `{"primary":"short"}`, envDigitalHumanCredentialActiveKeyID: "primary"},
		},
		{
			name:   "unknown active key",
			values: map[string]string{envDigitalHumanCredentialKeysJSON: `{"primary":"` + testDigitalHumanActiveKey + `"}`, envDigitalHumanCredentialActiveKeyID: "missing"},
		},
		{
			name:   "active without keys",
			values: map[string]string{envDigitalHumanCredentialActiveKeyID: "primary"},
		},
		{
			name:   "conflicting legacy key",
			cfg:    &Config{Encryption: EncryptionConfig{APIKeyEncryptionKey: testDigitalHumanLegacyKey}},
			values: map[string]string{envDigitalHumanCredentialKeysJSON: `{"legacy":"` + testDigitalHumanActiveKey + `"}`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.cfg
			if cfg == nil {
				cfg = &Config{}
			}
			if err := loadDigitalHumanCredentialsConfig(cfg, digitalHumanEnvSource(tc.values)); err == nil {
				t.Fatalf("loadDigitalHumanCredentialsConfig() error = nil")
			}
		})
	}
}
