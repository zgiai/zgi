package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLoadExternalIntegrationsConfigUsesUnifiedDefaults(t *testing.T) {
	cfg := &Config{
		Encryption: EncryptionConfig{APIKeyEncryptionKey: "01234567890123456789012345678901"},
	}
	if err := loadExternalIntegrationsConfig(cfg, externalIntegrationsEnvSource(nil)); err != nil {
		t.Fatalf("loadExternalIntegrationsConfig() error = %v", err)
	}

	got := cfg.ExternalIntegrations
	if got.Enabled || got.OrgDailyLimit != 1000 || got.TimeoutSeconds != 20 {
		t.Fatalf("unified defaults = enabled %v, daily %d, timeout %d", got.Enabled, got.OrgDailyLimit, got.TimeoutSeconds)
	}
	if got.CredentialActiveKeyID != "legacy" || got.CredentialKeys["legacy"] != cfg.Encryption.APIKeyEncryptionKey {
		t.Fatalf("legacy key fallback was not configured: active=%q keys=%v", got.CredentialActiveKeyID, len(got.CredentialKeys))
	}
	if got.Health.FailureThreshold != 3 {
		t.Fatalf("unexpected health defaults: %+v", got.Health)
	}
	if got.OAuth.RefreshWindowSeconds != 600 {
		t.Fatalf("unexpected OAuth defaults: %+v", got.OAuth)
	}
	if got.OAuth.FlowTTLSeconds != 600 ||
		got.OAuth.CallbackURL != "http://127.0.0.1:2679/console/api/integrations/oauth/callback" ||
		got.OAuth.ResultURL != "http://localhost:3000/console/integrations/oauth/result" {
		t.Fatalf("unexpected OAuth flow defaults: %+v", got.OAuth)
	}
}

func TestLegacyWebSearchEnableDoesNotControlExternalIntegrations(t *testing.T) {
	cfg := &Config{}
	source := externalIntegrationsEnvSource(map[string]string{
		"WEB_SEARCH_ENABLED": "true",
	})
	if err := loadExternalIntegrationsConfig(cfg, source); err != nil {
		t.Fatalf("loadExternalIntegrationsConfig() error = %v", err)
	}
	if cfg.ExternalIntegrations.Enabled {
		t.Fatal("legacy WEB_SEARCH_ENABLED unexpectedly enabled the shared runtime")
	}
}

func TestLoadExternalIntegrationsConfigReadsKeyringAndHealthPolicy(t *testing.T) {
	key := "abcdefghijklmnopqrstuvwxyz123456"
	source := externalIntegrationsEnvSource(map[string]string{
		envExternalIntegrationsEnabled:          "true",
		envIntegrationOrgDailyLimit:             "800",
		envIntegrationTimeoutSeconds:            "40",
		envIntegrationCredentialActiveKeyID:     "v2",
		envIntegrationCredentialKeysJSON:        `{"v1":"01234567890123456789012345678901","v2":"` + key + `"}`,
		envIntegrationHealthFailureThreshold:    "4",
		envIntegrationOAuthRefreshWindowSeconds: "900",
		envIntegrationOAuthFlowTTLSeconds:       "720",
		envIntegrationOAuthCallbackURL:          "https://api.example.com/console/api/integrations/oauth/callback",
		envIntegrationOAuthResultURL:            "https://app.example.com/console/integrations/oauth/result",
		envIntegrationOAuthClientsJSON:          `{"gmail":{"client_id":"client-id","client_secret":"client-secret","config":{"tenant":"example"}}}`,
	})
	cfg := &Config{}
	if err := loadExternalIntegrationsConfig(cfg, source); err != nil {
		t.Fatalf("loadExternalIntegrationsConfig() error = %v", err)
	}
	got := cfg.ExternalIntegrations
	if got.CredentialActiveKeyID != "v2" || got.CredentialKeys["v2"] != key {
		t.Fatalf("unexpected keyring config: active=%q keys=%d", got.CredentialActiveKeyID, len(got.CredentialKeys))
	}
	if got.Health.FailureThreshold != 4 || got.OAuth.RefreshWindowSeconds != 900 {
		t.Fatalf("unexpected health/OAuth config: health=%+v oauth=%+v", got.Health, got.OAuth)
	}
	if got.OAuth.FlowTTLSeconds != 720 || got.OAuth.Clients["gmail"].ClientID != "client-id" {
		t.Fatalf("unexpected OAuth flow/client config: %+v", got.OAuth)
	}
	if err := validateExternalIntegrationsConfig(got); err != nil {
		t.Fatalf("validateExternalIntegrationsConfig() error = %v", err)
	}
}

func TestExternalIntegrationsConfigRejectsInvalidKeyring(t *testing.T) {
	cfg := enabledExternalIntegrationsConfig()
	cfg.CredentialActiveKeyID = "missing"
	if err := validateExternalIntegrationsConfig(cfg); err == nil || !strings.Contains(err.Error(), envIntegrationCredentialActiveKeyID) {
		t.Fatalf("missing active key validation error = %v", err)
	}

	cfg = enabledExternalIntegrationsConfig()
	cfg.CredentialKeys["active"] = "short"
	if err := validateExternalIntegrationsConfig(cfg); err == nil || !strings.Contains(err.Error(), "32 bytes") {
		t.Fatalf("short key validation error = %v", err)
	}
}

func TestExternalIntegrationsConfigAcceptsXPublicOAuthClient(t *testing.T) {
	for _, key := range []string{"x", "x:x", "x/x"} {
		t.Run(key, func(t *testing.T) {
			cfg := enabledExternalIntegrationsConfig()
			cfg.OAuth.Clients = map[string]ExternalIntegrationOAuthClientConfig{
				key: {ClientID: "public-client-id"},
			}
			if err := validateExternalIntegrationsConfig(cfg); err != nil {
				t.Fatalf("validateExternalIntegrationsConfig() X public client error = %v", err)
			}
		})
	}
}

func TestLoadExternalIntegrationsConfigReadsXPublicOAuthClient(t *testing.T) {
	source := externalIntegrationsEnvSource(map[string]string{
		envExternalIntegrationsEnabled:          "true",
		envIntegrationCredentialActiveKeyID:     "active",
		envIntegrationCredentialKeysJSON:        `{"active":"01234567890123456789012345678901"}`,
		envIntegrationOAuthCallbackURL:          "https://api.example.com/console/api/integrations/oauth/callback",
		envIntegrationOAuthResultURL:            "https://app.example.com/console/integrations/oauth/result",
		envIntegrationOAuthClientsJSON:          `{"x":{"client_id":"public-client-id"}}`,
		envIntegrationOrgDailyLimit:             "1000",
		envIntegrationTimeoutSeconds:            "20",
		envIntegrationHealthFailureThreshold:    "3",
		envIntegrationOAuthRefreshWindowSeconds: "600",
		envIntegrationOAuthFlowTTLSeconds:       "600",
	})
	cfg := &Config{}
	if err := loadExternalIntegrationsConfig(cfg, source); err != nil {
		t.Fatalf("loadExternalIntegrationsConfig() error = %v", err)
	}
	client := cfg.ExternalIntegrations.OAuth.Clients["x"]
	if client.ClientID != "public-client-id" || client.ClientSecret != "" {
		t.Fatalf("loaded X public OAuth client = %#v", client)
	}
	if err := validateExternalIntegrationsConfig(cfg.ExternalIntegrations); err != nil {
		t.Fatalf("validateExternalIntegrationsConfig() error = %v", err)
	}
}

func TestExternalIntegrationsConfigRequiresSecretsForConfidentialOAuthProviders(t *testing.T) {
	for _, provider := range []string{"gmail", "feishu", "unknown"} {
		t.Run(provider, func(t *testing.T) {
			cfg := enabledExternalIntegrationsConfig()
			cfg.OAuth.Clients = map[string]ExternalIntegrationOAuthClientConfig{
				provider: {ClientID: "client-id"},
			}
			err := validateExternalIntegrationsConfig(cfg)
			if err == nil || !strings.Contains(err.Error(), "missing client_secret") {
				t.Fatalf("validateExternalIntegrationsConfig() error = %v", err)
			}
		})
	}
}

func TestExternalIntegrationCredentialKeysAreNotSerialized(t *testing.T) {
	cfg := enabledExternalIntegrationsConfig()
	cfg.OAuth.Clients = map[string]ExternalIntegrationOAuthClientConfig{
		"gmail": {ClientID: "client-id", ClientSecret: "client-secret"},
	}
	encoded, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(encoded), cfg.CredentialKeys["active"]) || strings.Contains(string(encoded), "credential_keys") {
		t.Fatalf("serialized integration config leaked key material: %s", encoded)
	}
	if strings.Contains(string(encoded), "client-id") || strings.Contains(string(encoded), "client-secret") || strings.Contains(string(encoded), "clients") {
		t.Fatalf("serialized integration config leaked OAuth client material: %s", encoded)
	}
}

func enabledExternalIntegrationsConfig() ExternalIntegrationsConfig {
	return ExternalIntegrationsConfig{
		Enabled:               true,
		OrgDailyLimit:         1000,
		TimeoutSeconds:        20,
		CredentialActiveKeyID: "active",
		CredentialKeys: map[string]string{
			"active": "01234567890123456789012345678901",
		},
		Health: ExternalIntegrationHealthConfig{
			FailureThreshold: 3,
		},
		OAuth: ExternalIntegrationOAuthConfig{
			RefreshWindowSeconds: 600,
			FlowTTLSeconds:       600,
			CallbackURL:          "https://api.example.com/console/api/integrations/oauth/callback",
			ResultURL:            "https://app.example.com/console/integrations/oauth/result",
		},
	}
}

func externalIntegrationsEnvSource(values map[string]string) *envSource {
	return &envSource{lookupEnv: func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}}
}
