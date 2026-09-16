package config

import (
	"strings"
	"testing"
)

func TestLoadConnectorsConfigIsIndependentFromExternalIntegrations(t *testing.T) {
	values := map[string]string{
		envConnectorCredentialActiveKeyID: "primary",
		envConnectorCredentialKeysJSON:    `{"primary":"12345678901234567890123456789012"}`,
		envConnectorPluginDirectoriesJSON: `["D:/plugins/github", "D:/plugins/search", "D:/plugins/github"]`,
	}
	source := &envSource{lookupEnv: func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}}
	cfg := &Config{}

	if err := loadConnectorsConfig(cfg, source); err != nil {
		t.Fatalf("loadConnectorsConfig() error = %v", err)
	}
	if got := cfg.Connectors.CredentialKeys["primary"]; got != "12345678901234567890123456789012" {
		t.Fatalf("connector credential key = %q, want configured literal", got)
	}
	if len(cfg.Connectors.PluginDirectories) != 2 || cfg.Connectors.PluginDirectories[0] != "D:/plugins/github" ||
		cfg.Connectors.PluginDirectories[1] != "D:/plugins/search" {
		t.Fatalf("connector plugin directories = %#v", cfg.Connectors.PluginDirectories)
	}
	if err := validateConnectorsConfig(cfg.Connectors); err != nil {
		t.Fatalf("validateConnectorsConfig() error = %v", err)
	}
}

func TestLoadConnectorsConfigRejectsInvalidPluginDirectories(t *testing.T) {
	for _, value := range []string{`{"not":"an array"}`, `["valid", " "]`} {
		t.Run(value, func(t *testing.T) {
			source := &envSource{lookupEnv: func(key string) (string, bool) {
				if key == envConnectorPluginDirectoriesJSON {
					return value, true
				}
				return "", false
			}}
			if err := loadConnectorsConfig(&Config{}, source); err == nil ||
				!strings.Contains(err.Error(), envConnectorPluginDirectoriesJSON) {
				t.Fatalf("loadConnectorsConfig() error = %v", err)
			}
		})
	}
}

func TestLoadConnectorsConfigRejectsEncryptionKeyFallback(t *testing.T) {
	cfg := &Config{Encryption: EncryptionConfig{APIKeyEncryptionKey: "12345678901234567890123456789012"}}
	source := &envSource{lookupEnv: func(string) (string, bool) { return "", false }}

	if err := loadConnectorsConfig(cfg, source); err != nil {
		t.Fatalf("loadConnectorsConfig() error = %v", err)
	}
	if cfg.Connectors.CredentialActiveKeyID != "" || len(cfg.Connectors.CredentialKeys) != 0 {
		t.Fatalf("connector fallback keyring = %#v", cfg.Connectors)
	}
	if err := validateConnectorsConfig(cfg.Connectors); err == nil {
		t.Fatal("Connector keyring must be explicitly configured")
	}
}

func TestLoadConnectorsConfigIgnoresRetiredEnableSwitch(t *testing.T) {
	for _, value := range []string{"", "false", "true", "invalid"} {
		t.Run("retired_switch_"+value, func(t *testing.T) {
			cfg := &Config{Encryption: EncryptionConfig{APIKeyEncryptionKey: strings.Repeat("c", 32)}}
			source := &envSource{lookupEnv: func(key string) (string, bool) {
				if key == envConnectorCredentialKeysJSON {
					return `{"default":"12345678901234567890123456789012"}`, true
				}
				if key == "CONNECTORS_ENABLED" {
					t.Errorf("loadConnectorsConfig() read the retired enable switch")
					return value, value != ""
				}
				return "", false
			}}
			if err := loadConnectorsConfig(cfg, source); err != nil {
				t.Fatalf("loadConnectorsConfig() error = %v", err)
			}
			if err := validateConnectorsConfig(cfg.Connectors); err != nil {
				t.Fatalf("validateConnectorsConfig() error = %v", err)
			}
		})
	}
}

func TestConnectorsConfigRequiresValidKeyring(t *testing.T) {
	err := validateConnectorsConfig(ConnectorsConfig{})
	if err == nil || !strings.Contains(err.Error(), envConnectorCredentialKeysJSON) {
		t.Fatalf("missing connector keyring error = %v", err)
	}

	err = validateConnectorsConfig(ConnectorsConfig{
		CredentialActiveKeyID: "primary",
		CredentialKeys:        map[string]string{"primary": "short"},
	})
	if err == nil || !strings.Contains(err.Error(), "32 bytes") {
		t.Fatalf("short connector credential key error = %v", err)
	}
}

func TestConnectorsConfigRequiresActiveKey(t *testing.T) {
	for _, activeKeyID := range []string{"", "missing"} {
		cfg := ConnectorsConfig{
			CredentialActiveKeyID: activeKeyID,
			CredentialKeys:        map[string]string{"primary": strings.Repeat("c", 32)},
		}
		if err := validateConnectorsConfig(cfg); err == nil || !strings.Contains(err.Error(), envConnectorCredentialActiveKeyID) {
			t.Fatalf("active key %q validation error = %v", activeKeyID, err)
		}
	}
}
