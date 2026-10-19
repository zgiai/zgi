package config

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

func loadSocialOAuthProviders(source *envSource) map[string]SocialOAuthProviderConfig {
	result := make(map[string]SocialOAuthProviderConfig, 3)
	for _, entry := range []struct{ id, prefix, scopes string }{
		{"x-social", "CONNECTOR_X_OAUTH_", "tweet.read tweet.write users.read media.write dm.read dm.write offline.access"},
		{"linkedin-social", "CONNECTOR_LINKEDIN_OAUTH_", "openid profile r_organization_social w_organization_social rw_organization_admin"},
		{"facebook-social", "CONNECTOR_FACEBOOK_OAUTH_", "pages_show_list pages_read_engagement pages_read_user_content pages_manage_posts pages_manage_engagement pages_messaging read_insights"},
	} {
		cfg := SocialOAuthProviderConfig{
			ClientID:     strings.TrimSpace(source.string("", entry.prefix+"CLIENT_ID")),
			ClientSecret: strings.TrimSpace(source.string("", entry.prefix+"CLIENT_SECRET")),
			RedirectURL:  strings.TrimSpace(source.string("", entry.prefix+"REDIRECT_URL")),
			Scopes:       strings.FieldsFunc(source.string(entry.scopes, entry.prefix+"SCOPES"), func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' }),
		}
		if entry.id == "facebook-social" {
			cfg.APIVersion = strings.TrimSpace(source.string("v23.0", entry.prefix+"API_VERSION"))
		}
		result[entry.id] = cfg
	}
	return result
}

func validateSocialOAuthURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return nil, fmt.Errorf("invalid OAuth URL")
	}
	ip := net.ParseIP(u.Hostname())
	loopback := u.Hostname() == "localhost" || ip != nil && ip.IsLoopback()
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return nil, fmt.Errorf("OAuth URL requires HTTPS except on loopback")
	}
	return u, nil
}

func validateSocialOAuthConfig(cfg ConnectorsConfig) error {
	enabled := false
	for id, provider := range cfg.SocialOAuthProviders {
		if id != "x-social" && id != "linkedin-social" && id != "facebook-social" {
			return fmt.Errorf("unsupported social OAuth provider")
		}
		if provider.ClientID == "" && provider.ClientSecret == "" && provider.RedirectURL == "" {
			continue
		}
		enabled = true
		if provider.ClientID == "" || provider.ClientSecret == "" || provider.RedirectURL == "" || len(provider.Scopes) == 0 {
			return fmt.Errorf("social OAuth provider %s requires client ID, secret, callback and scopes", id)
		}
		callback, err := validateSocialOAuthURL(provider.RedirectURL)
		if err != nil || callback.Path != "/console/api/connectors/"+id+"/oauth/callback" {
			return fmt.Errorf("social OAuth provider %s requires its exact API callback URL", id)
		}
		if id == "facebook-social" && !regexp.MustCompile(`^v[0-9]+\.0$`).MatchString(provider.APIVersion) {
			return fmt.Errorf("Facebook OAuth API version is invalid")
		}
	}
	if cfg.SocialOAuthFrontendURL != "" || enabled {
		frontend, err := validateSocialOAuthURL(cfg.SocialOAuthFrontendURL)
		if err != nil || frontend.Path != "" && frontend.Path != "/" {
			return fmt.Errorf("CONNECTOR_SOCIAL_OAUTH_FRONTEND_URL must be an HTTPS origin (HTTP allowed on loopback)")
		}
	}
	return nil
}
