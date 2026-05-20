package service

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildCasdoorAuthorizationURL(t *testing.T) {
	t.Parallel()

	discovery := oidcDiscoveryDocument{
		AuthorizationEndpoint: "https://door.example.com/login/oauth/authorize",
	}

	cfg := casdoorOIDCConfig{
		ClientID:    "client-id",
		RedirectURI: "https://api.example.com/sso/casdoor/callback",
		Scopes:      []string{"openid", "profile", "email"},
	}

	got, err := buildCasdoorAuthorizationURL(discovery, cfg, "state-1")
	require.NoError(t, err)

	parsed, err := url.Parse(got)
	require.NoError(t, err)
	require.Equal(t, "https", parsed.Scheme)
	require.Equal(t, "door.example.com", parsed.Host)
	require.Equal(t, "client-id", parsed.Query().Get("client_id"))
	require.Equal(t, "state-1", parsed.Query().Get("state"))
	require.Equal(t, "code", parsed.Query().Get("response_type"))
	require.Equal(t, "openid profile email", parsed.Query().Get("scope"))
}

func TestIdentityFromClaims(t *testing.T) {
	t.Parallel()

	claims := map[string]any{
		"sub":          "sub-1",
		"email":        "User@Example.com",
		"name":         "Case Door",
		"phone_number": "+1 415 555 2671",
	}

	identity, err := identityFromClaims(claims)
	require.NoError(t, err)
	require.Equal(t, "sub-1", identity.Subject)
	require.Equal(t, "user@example.com", identity.Email)
	require.Equal(t, "Case Door", identity.Name)
	require.Equal(t, "+14155552671", identity.PhoneNumber)
}

func TestNormalizeCasdoorDiscoveryEndpointsUsesConfiguredPortWhenDiscoveryOmitsIt(t *testing.T) {
	t.Parallel()

	discovery := oidcDiscoveryDocument{
		AuthorizationEndpoint: "http://sso.example.test/login/oauth/authorize",
		TokenEndpoint:         "http://sso.example.test/api/login/oauth/access_token",
		UserinfoEndpoint:      "http://sso.example.test/api/userinfo",
		JWKSURI:               "http://sso.example.test/.well-known/jwks",
		Issuer:                "http://sso.example.test",
	}

	got := normalizeCasdoorDiscoveryEndpoints("http://sso.example.test:8000", discovery)

	require.Equal(t, "http://sso.example.test:8000/login/oauth/authorize", got.AuthorizationEndpoint)
	require.Equal(t, "http://sso.example.test:8000/api/login/oauth/access_token", got.TokenEndpoint)
	require.Equal(t, "http://sso.example.test:8000/api/userinfo", got.UserinfoEndpoint)
	require.Equal(t, "http://sso.example.test:8000/.well-known/jwks", got.JWKSURI)
	require.Equal(t, "http://sso.example.test", got.Issuer)
}
