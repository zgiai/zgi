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
		got.OAuth.CallbackURL != "http://127.0.0.1:2670/console/api/integrations/oauth/callback" ||
		got.OAuth.ResultURL != "http://localhost:3000/console/integrations/oauth/result" {
		t.Fatalf("unexpected OAuth flow defaults: %+v", got.OAuth)
	}
	if got.WeCom.InstallCallbackURL != "http://127.0.0.1:2670/console/api/integrations/wecom/install/callback" ||
		got.WeCom.SuiteEventsURL != "http://127.0.0.1:2670/console/api/integrations/wecom/suite/events" ||
		got.WeCom.ResultURL != "http://localhost:3000/console/integrations?provider=wecom" ||
		got.WeCom.InstallAuthMode != WeComInstallAuthModeFormal ||
		got.WeCom.CustomerContactEnabled || got.WeCom.CustomerMassMessageEnabled {
		t.Fatalf("unexpected WeCom URL defaults: %+v", got.WeCom)
	}
}

func TestLoadConsoleAndOAuthDefaultsFollowServerListener(t *testing.T) {
	cfg := &Config{
		Server:     ServerConfig{Port: 4317},
		Encryption: EncryptionConfig{APIKeyEncryptionKey: "01234567890123456789012345678901"},
	}
	source := externalIntegrationsEnvSource(nil)
	loadConsoleConfig(cfg, source)
	if cfg.Console.APIURL != "http://127.0.0.1:4317" {
		t.Fatalf("Console API URL = %q, want server listener", cfg.Console.APIURL)
	}
	if err := loadExternalIntegrationsConfig(cfg, source); err != nil {
		t.Fatalf("loadExternalIntegrationsConfig() error = %v", err)
	}
	if got := cfg.ExternalIntegrations.OAuth.CallbackURL; got != "http://127.0.0.1:4317/console/api/integrations/oauth/callback" {
		t.Fatalf("OAuth callback URL = %q, want server listener", got)
	}
}

func TestLoadConsoleAndOAuthHonorExplicitGatewayURL(t *testing.T) {
	cfg := &Config{
		Server:     ServerConfig{Port: 2670},
		Encryption: EncryptionConfig{APIKeyEncryptionKey: "01234567890123456789012345678901"},
	}
	source := externalIntegrationsEnvSource(map[string]string{
		envConsoleAPIURL: "http://localhost:2679",
	})
	loadConsoleConfig(cfg, source)
	if err := loadExternalIntegrationsConfig(cfg, source); err != nil {
		t.Fatalf("loadExternalIntegrationsConfig() error = %v", err)
	}
	if got := cfg.ExternalIntegrations.OAuth.CallbackURL; got != "http://localhost:2679/console/api/integrations/oauth/callback" {
		t.Fatalf("OAuth callback URL = %q, want explicit gateway URL", got)
	}
}

func TestProductionOAuthDeploymentRejectsLoopbackURLs(t *testing.T) {
	cfg := &Config{
		Server:               ServerConfig{Mode: "release", Environment: "production"},
		ExternalIntegrations: enabledExternalIntegrationsConfig(),
	}
	cfg.ExternalIntegrations.OAuth.CallbackURL = "http://127.0.0.1:2670/console/api/integrations/oauth/callback"
	cfg.ExternalIntegrations.OAuth.ResultURL = "https://app.example.com/console/integrations/oauth/result"
	if err := validateExternalIntegrationOAuthDeployment(cfg); err == nil ||
		!strings.Contains(err.Error(), envIntegrationOAuthCallbackURL) {
		t.Fatalf("production callback validation error = %v", err)
	}

	cfg.ExternalIntegrations.OAuth.CallbackURL = "https://api.example.com/console/api/integrations/oauth/callback"
	cfg.ExternalIntegrations.OAuth.ResultURL = "http://localhost:3000/console/integrations/oauth/result"
	if err := validateExternalIntegrationOAuthDeployment(cfg); err == nil ||
		!strings.Contains(err.Error(), envIntegrationOAuthResultURL) {
		t.Fatalf("production result validation error = %v", err)
	}

	cfg.ExternalIntegrations.OAuth.ResultURL = "https://app.example.com/console/integrations/oauth/result"
	if err := validateExternalIntegrationOAuthDeployment(cfg); err != nil {
		t.Fatalf("production HTTPS OAuth URLs rejected: %v", err)
	}
}

func TestDevelopmentOAuthDeploymentAllowsLoopbackURLs(t *testing.T) {
	cfg := &Config{
		Server:               ServerConfig{Mode: "debug", Environment: "local"},
		ExternalIntegrations: enabledExternalIntegrationsConfig(),
	}
	cfg.ExternalIntegrations.OAuth.CallbackURL = "http://127.0.0.1:2670/console/api/integrations/oauth/callback"
	cfg.ExternalIntegrations.OAuth.ResultURL = "http://localhost:3000/console/integrations/oauth/result"
	if err := validateExternalIntegrationOAuthDeployment(cfg); err != nil {
		t.Fatalf("development loopback OAuth URLs rejected: %v", err)
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

func TestExternalIntegrationsConfigLoadsAndValidatesWeComSuite(t *testing.T) {
	source := externalIntegrationsEnvSource(map[string]string{
		envIntegrationWeComSuiteID:                    "ww0123456789abcdef",
		envIntegrationWeComProviderCorpID:             "wwfedcba9876543210",
		envIntegrationWeComProviderSecret:             " provider-secret ",
		envIntegrationWeComCustomerContactEnabled:     "true",
		envIntegrationWeComCustomerMassMessageEnabled: "TRUE",
		envIntegrationWeComSuiteSecret:                "suite-secret",
		envIntegrationWeComCallbackToken:              "callback-token",
		envIntegrationWeComEncodingAESKey:             "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG",
		envIntegrationWeComInstallAuthMode:            " Test ",
		envIntegrationWeComInstallCallbackURL:         "https://cloud.vibeclass.cn/console/api/integrations/wecom/install/callback",
		envIntegrationWeComSuiteEventsURL:             "https://cloud.vibeclass.cn/console/api/integrations/wecom/suite/events",
		envIntegrationWeComResultURL:                  "https://cloud.vibeclass.cn/console/integrations?provider=wecom",
	})
	cfg := &Config{}
	if err := loadExternalIntegrationsConfig(cfg, source); err != nil {
		t.Fatalf("loadExternalIntegrationsConfig() error = %v", err)
	}
	if !cfg.ExternalIntegrations.WeCom.Configured() {
		t.Fatalf("WeCom Suite config was not loaded: %+v", cfg.ExternalIntegrations.WeCom)
	}
	if cfg.ExternalIntegrations.WeCom.InstallAuthMode != WeComInstallAuthModeTest {
		t.Fatalf("WeCom install auth mode = %q", cfg.ExternalIntegrations.WeCom.InstallAuthMode)
	}
	if cfg.ExternalIntegrations.WeCom.ProviderSecret != "provider-secret" {
		t.Fatalf("WeCom provider secret was not normalized")
	}
	if !cfg.ExternalIntegrations.WeCom.CustomerContactEnabled || !cfg.ExternalIntegrations.WeCom.CustomerMassMessageEnabled {
		t.Fatalf("WeCom customer capabilities were not loaded: %+v", cfg.ExternalIntegrations.WeCom)
	}
	validWeCom := enabledExternalIntegrationsConfig()
	validWeCom.WeCom = cfg.ExternalIntegrations.WeCom
	if err := validateExternalIntegrationsConfig(validWeCom); err != nil {
		t.Fatalf("valid WeCom Suite config rejected: %v", err)
	}

	partial := enabledExternalIntegrationsConfig()
	partial.WeCom.SuiteID = "ww0123456789abcdef"
	if err := validateExternalIntegrationsConfig(partial); err == nil || !strings.Contains(err.Error(), "configured together") {
		t.Fatalf("partial WeCom Suite validation error = %v", err)
	}

	missingProviderCorpID := validWeCom
	missingProviderCorpID.WeCom.ProviderCorpID = ""
	if err := validateExternalIntegrationsConfig(missingProviderCorpID); err == nil || !strings.Contains(err.Error(), envIntegrationWeComProviderCorpID) {
		t.Fatalf("missing WeCom provider CorpID validation error = %v", err)
	}

	invalidProviderCorpID := validWeCom
	invalidProviderCorpID.WeCom.ProviderCorpID = "provider-corp"
	if err := validateExternalIntegrationsConfig(invalidProviderCorpID); err == nil || !strings.Contains(err.Error(), envIntegrationWeComProviderCorpID) {
		t.Fatalf("invalid WeCom provider CorpID validation error = %v", err)
	}

	missingProviderSecret := validWeCom
	missingProviderSecret.WeCom.ProviderSecret = ""
	if err := validateExternalIntegrationsConfig(missingProviderSecret); err == nil || !strings.Contains(err.Error(), envIntegrationWeComProviderSecret) {
		t.Fatalf("missing WeCom provider secret validation error = %v", err)
	}

	tooLongProviderSecret := validWeCom
	tooLongProviderSecret.WeCom.ProviderSecret = strings.Repeat("s", 513)
	if err := validateExternalIntegrationsConfig(tooLongProviderSecret); err == nil || !strings.Contains(err.Error(), envIntegrationWeComProviderSecret) {
		t.Fatalf("long WeCom provider secret validation error = %v", err)
	}

	invalidInstallAuthMode := validWeCom
	invalidInstallAuthMode.WeCom.InstallAuthMode = "1"
	if err := validateExternalIntegrationsConfig(invalidInstallAuthMode); err == nil || !strings.Contains(err.Error(), envIntegrationWeComInstallAuthMode) {
		t.Fatalf("invalid WeCom install auth mode validation error = %v", err)
	}
}

func TestExternalIntegrationsConfigRejectsUnsafeWeComCustomerCapabilities(t *testing.T) {
	for _, key := range []string{envIntegrationWeComCustomerContactEnabled, envIntegrationWeComCustomerMassMessageEnabled} {
		t.Run(key+"_invalid_boolean", func(t *testing.T) {
			cfg := &Config{}
			err := loadExternalIntegrationsConfig(cfg, externalIntegrationsEnvSource(map[string]string{key: "enabled"}))
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("invalid boolean error = %v", err)
			}
		})
	}

	massMessageWithoutContact := enabledExternalIntegrationsConfig()
	massMessageWithoutContact.WeCom.CustomerMassMessageEnabled = true
	if err := validateExternalIntegrationsConfig(massMessageWithoutContact); err == nil ||
		!strings.Contains(err.Error(), envIntegrationWeComCustomerMassMessageEnabled) ||
		!strings.Contains(err.Error(), envIntegrationWeComCustomerContactEnabled) {
		t.Fatalf("mass-message dependency validation error = %v", err)
	}

	for _, capability := range []struct {
		name   string
		mutate func(*ExternalIntegrationWeComConfig)
	}{
		{name: "customer_contact", mutate: func(cfg *ExternalIntegrationWeComConfig) { cfg.CustomerContactEnabled = true }},
		{name: "customer_mass_message", mutate: func(cfg *ExternalIntegrationWeComConfig) {
			cfg.CustomerContactEnabled = true
			cfg.CustomerMassMessageEnabled = true
		}},
	} {
		t.Run(capability.name+"_requires_suite", func(t *testing.T) {
			cfg := ExternalIntegrationsConfig{}
			capability.mutate(&cfg.WeCom)
			if err := validateExternalIntegrationsConfig(cfg); err == nil || !strings.Contains(err.Error(), "complete WeCom Suite") {
				t.Fatalf("incomplete Suite validation error = %v", err)
			}
		})
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
	cfg.WeCom = ExternalIntegrationWeComConfig{
		SuiteID: "ww0123456789abcdef", ProviderCorpID: "wwfedcba9876543210", ProviderSecret: "provider-secret", SuiteSecret: "suite-secret",
		CallbackToken: "callback-token", EncodingAESKey: "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG",
		CustomerContactEnabled: true, CustomerMassMessageEnabled: true,
		InstallCallbackURL: "https://app.example.com/install", SuiteEventsURL: "https://app.example.com/events",
		ResultURL: "https://app.example.com/result",
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
	for _, secret := range []string{cfg.WeCom.SuiteID, cfg.WeCom.ProviderCorpID, cfg.WeCom.ProviderSecret, cfg.WeCom.SuiteSecret, cfg.WeCom.CallbackToken, cfg.WeCom.EncodingAESKey} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("serialized integration config leaked WeCom secret material: %s", encoded)
		}
	}
	if !strings.Contains(string(encoded), `"customer_contact_enabled":true`) || !strings.Contains(string(encoded), `"customer_mass_message_enabled":true`) {
		t.Fatalf("serialized integration config omitted safe WeCom capability flags: %s", encoded)
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
