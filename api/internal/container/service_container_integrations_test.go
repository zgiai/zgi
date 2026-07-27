package container

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/integrations/adapters/exa"
	"github.com/zgiai/zgi/api/internal/modules/integrations/adapters/feishu"
	"github.com/zgiai/zgi/api/internal/modules/integrations/adapters/github"
	"github.com/zgiai/zgi/api/internal/modules/integrations/adapters/gmail"
	xintegration "github.com/zgiai/zgi/api/internal/modules/integrations/adapters/x"
)

func TestIntegrationRegistryRegistersGitHubWithExternalIntegrations(t *testing.T) {
	container := &ServiceContainer{config: &config.Config{
		ExternalIntegrations: config.ExternalIntegrationsConfig{Enabled: true},
	}}

	registry := container.GetIntegrationRegistry()
	definition, found := registry.ProviderDefinition(github.IntegrationID)
	if !found {
		t.Fatal("GitHub provider was not registered")
	}
	if definition.DriverID != github.DriverID || len(definition.Actions) != 3 {
		t.Fatalf("GitHub definition = %#v", definition)
	}
	if len(definition.AuthMethods) != 2 || definition.AuthMethods[0].CredentialSource == definition.AuthMethods[1].CredentialSource {
		t.Fatalf("GitHub auth methods = %#v", definition.AuthMethods)
	}
}

func TestIntegrationRegistryRegistersOAuthProvidersWithoutPlatformCredentials(t *testing.T) {
	container := &ServiceContainer{config: &config.Config{
		ExternalIntegrations: config.ExternalIntegrationsConfig{Enabled: true},
	}}
	registry := container.GetIntegrationRegistry()
	for _, expected := range []struct {
		integrationID string
		driverID      string
	}{
		{gmail.IntegrationID, gmail.DriverID},
		{feishu.IntegrationID, feishu.DriverID},
		{xintegration.IntegrationID, xintegration.DriverID},
	} {
		definition, found := registry.ProviderDefinition(expected.integrationID)
		if !found {
			t.Fatalf("%s provider was not registered", expected.integrationID)
		}
		if definition.DriverID != expected.driverID {
			t.Fatalf("%s driver = %q", expected.integrationID, definition.DriverID)
		}
		if _, found := registry.OAuthProvider(expected.integrationID, expected.driverID); !found {
			t.Fatalf("%s OAuth provider was not registered", expected.integrationID)
		}
		for _, method := range definition.AuthMethods {
			if method.Type == integrations.AuthMethodTypePlatform ||
				method.CredentialSource == integrations.ConnectionCredentialSourcePlatform {
				t.Fatalf("%s exposed a platform-owned auth method: %#v", expected.integrationID, method)
			}
		}
	}
}

func TestIntegrationRegistryDoesNotRegisterGitHubForWebSearchOnly(t *testing.T) {
	container := &ServiceContainer{config: &config.Config{
		WebSearch: config.WebSearchConfig{
			Enabled: true,
			Exa: config.ExaConfig{
				TimeoutSeconds:       20,
				MaxResults:           10,
				DefaultSearchType:    "auto",
				MaxFetchURLs:         5,
				MaxContentCharacters: 20000,
			},
		},
	}}

	registry := container.GetIntegrationRegistry()
	if _, found := registry.ProviderDefinition(github.IntegrationID); found {
		t.Fatal("GitHub provider must require EXTERNAL_INTEGRATIONS_ENABLED")
	}
	definition, found := registry.ProviderDefinition(integrations.IntegrationWebSearch)
	if !found {
		t.Fatal("Exa provider was not registered without EXA_API_KEY")
	}
	sources := map[integrations.ConnectionCredentialSource]bool{}
	for _, method := range definition.AuthMethods {
		if method.Type == integrations.AuthMethodTypePlatform || method.CredentialSource == integrations.ConnectionCredentialSourcePlatform {
			t.Fatalf("Exa exposed deployment-owned credentials: %#v", method)
		}
		sources[method.CredentialSource] = true
	}
	if !sources[integrations.ConnectionCredentialSourceOrganization] || !sources[integrations.ConnectionCredentialSourceAccount] {
		t.Fatalf("Exa auth sources = %#v", sources)
	}
	catalogJSON, err := json.Marshal(registry.Catalog())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(catalogJSON), `"credential_source":"platform"`) ||
		strings.Contains(string(catalogJSON), `"type":"platform"`) ||
		strings.Contains(string(catalogJSON), "platform_credentials_configured") {
		t.Fatalf("catalog exposed platform credentials: %s", catalogJSON)
	}
	if definition.DriverID != exa.ProviderDefinition("auto").DriverID {
		t.Fatalf("Exa definition = %#v", definition)
	}
}

func TestParseIntegrationOAuthClientKey(t *testing.T) {
	tests := []struct {
		raw          string
		integration  string
		clientConfig string
		wantErr      bool
	}{
		{raw: "gmail", integration: "gmail", clientConfig: "gmail"},
		{raw: "feishu:feishu", integration: "feishu", clientConfig: "feishu"},
		{raw: "x/x", integration: "x", clientConfig: "x"},
		{raw: "", wantErr: true},
		{raw: "gmail:google:extra", wantErr: true},
	}
	for _, test := range tests {
		integrationID, clientConfigID, err := parseIntegrationOAuthClientKey(test.raw)
		if test.wantErr {
			if err == nil {
				t.Fatalf("parseIntegrationOAuthClientKey(%q) unexpectedly succeeded", test.raw)
			}
			continue
		}
		if err != nil || integrationID != test.integration || clientConfigID != test.clientConfig {
			t.Fatalf(
				"parseIntegrationOAuthClientKey(%q) = (%q, %q, %v)",
				test.raw,
				integrationID,
				clientConfigID,
				err,
			)
		}
	}
}
