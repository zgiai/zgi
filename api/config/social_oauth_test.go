package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSocialOAuthDeploymentConfig(t *testing.T) {
	values := map[string]string{
		"CONNECTOR_SOCIAL_OAUTH_FRONTEND_URL": "https://zgi.example",
		"CONNECTOR_X_OAUTH_CLIENT_ID":         "client-x", "CONNECTOR_X_OAUTH_CLIENT_SECRET": "secret-sentinel",
		"CONNECTOR_X_OAUTH_REDIRECT_URL":     "https://api.example/console/api/connectors/x-social/oauth/callback",
		"CONNECTOR_LINKEDIN_OAUTH_CLIENT_ID": "client-li", "CONNECTOR_LINKEDIN_OAUTH_CLIENT_SECRET": "secret-li",
		"CONNECTOR_LINKEDIN_OAUTH_REDIRECT_URL": "https://api.example/console/api/connectors/linkedin-social/oauth/callback",
		"CONNECTOR_LINKEDIN_OAUTH_SCOPES":       "r_organization_social,w_organization_social rw_organization_admin",
	}
	cfg := &Config{}
	if err := loadConnectorsConfig(cfg, &envSource{lookupEnv: func(k string) (string, bool) { v, ok := values[k]; return v, ok }}); err != nil {
		t.Fatal(err)
	}
	if cfg.Connectors.SocialOAuthFrontendURL != "https://zgi.example" || cfg.Connectors.SocialOAuthProviders["x-social"].ClientID != "client-x" {
		t.Fatal("OAuth configuration not loaded")
	}
	li := cfg.Connectors.SocialOAuthProviders["linkedin-social"]
	if len(li.Scopes) != 3 {
		t.Fatal("scope parsing failed")
	}
	data, _ := json.Marshal(cfg.Connectors)
	if strings.Contains(string(data), "secret-sentinel") || strings.Contains(string(data), "secret-li") {
		t.Fatal("secret serialized")
	}
	cfg.Connectors.CredentialActiveKeyID = "default"
	cfg.Connectors.CredentialKeys = map[string]string{"default": "12345678901234567890123456789012"}
	if err := validateConnectorsConfig(cfg.Connectors); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"http://public.example/console/api/connectors/x-social/oauth/callback", "https://api.example/other", "https://user:pass@api.example/console/api/connectors/x-social/oauth/callback", "https://api.example/console/api/connectors/x-social/oauth/callback?token=foo"} {
		provider := cfg.Connectors.SocialOAuthProviders["x-social"]
		old := provider.RedirectURL
		provider.RedirectURL = bad
		cfg.Connectors.SocialOAuthProviders["x-social"] = provider
		if validateConnectorsConfig(cfg.Connectors) == nil {
			t.Fatalf("unsafe callback accepted %s", bad)
		}
		provider.RedirectURL = old
		cfg.Connectors.SocialOAuthProviders["x-social"] = provider
	}
}

func TestSocialOAuthDefaultsIncludeIdentityAndRenewalScopes(t *testing.T) {
	providers := loadSocialOAuthProviders(&envSource{lookupEnv: func(string) (string, bool) { return "", false }})
	for id, required := range map[string][]string{"x-social": {"users.read", "offline.access", "dm.read", "dm.write"}, "linkedin-social": {"openid", "profile", "r_organization_social", "w_organization_social"}, "facebook-social": {"pages_show_list", "pages_messaging", "read_insights"}} {
		scopes := " " + strings.Join(providers[id].Scopes, " ") + " "
		for _, scope := range required {
			if !strings.Contains(scopes, " "+scope+" ") {
				t.Fatalf("%s missing %s", id, scope)
			}
		}
	}
}

func TestGitHubOAuthConfigRequiresAppMetadata(t *testing.T) {
	values := map[string]string{"CONNECTOR_GITHUB_OAUTH_CLIENT_ID": "Iv-client", "CONNECTOR_GITHUB_OAUTH_CLIENT_SECRET": "secret", "CONNECTOR_GITHUB_OAUTH_REDIRECT_URL": "https://api.example/console/api/connectors/github/oauth/callback", "CONNECTOR_GITHUB_OAUTH_APP_ID": "12345", "CONNECTOR_GITHUB_OAUTH_APP_SLUG": "example-app"}
	providers := loadSocialOAuthProviders(&envSource{lookupEnv: func(k string) (string, bool) { v, ok := values[k]; return v, ok }})
	p, ok := providers["github"]
	if !ok {
		t.Fatal("GitHub provider not loaded")
	}
	cfg := ConnectorsConfig{SocialOAuthProviders: providers, SocialOAuthFrontendURL: "https://app.example"}
	if err := validateSocialOAuthConfig(cfg); err != nil {
		t.Fatal(err)
	}
	for _, slug := range []string{"../evil", "app?next=evil", "app/evil", ""} {
		p.AppSlug = slug
		cfg.SocialOAuthProviders["github"] = p
		if validateSocialOAuthConfig(cfg) == nil {
			t.Fatal("unsafe app slug accepted")
		}
	}
}
