package x

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

func TestProviderDefinitionGatesSearchAndWrite(t *testing.T) {
	definition := ProviderDefinition()
	if definition.ID != IntegrationID || definition.DriverID != DriverID || len(definition.AuthMethods) != 3 {
		t.Fatalf("definition = %#v", definition)
	}
	for _, method := range definition.AuthMethods {
		if method.ID == AppBearerAuthMethodID {
			if method.Type != integrations.AuthMethodTypeCustomCredential ||
				method.CredentialSource != integrations.ConnectionCredentialSourceOrganization ||
				method.IdentityKind != integrations.AuthIdentityKindApplication ||
				method.AcquisitionStrategy != integrations.AuthAcquisitionStrategyManualForm ||
				method.LifecycleStrategy != integrations.AuthLifecycleStrategyStatic ||
				method.RequestAuthStrategy != integrations.RequestAuthStrategyBearerHeader ||
				len(method.Fields) != 1 || method.Fields[0].Key != "bearer_token" ||
				!method.Fields[0].Required || !method.Fields[0].Secret ||
				method.SetupGuide == nil || len(method.SetupGuide.Steps) != 4 ||
				!strings.HasPrefix(method.SetupGuide.ConsoleURL, "https://") ||
				!strings.HasPrefix(method.SetupGuide.DocumentationURL, "https://") {
				t.Fatalf("app Bearer method = %#v", method)
			}
			if method.SetupGuide.Steps[0].Action != integrations.AuthSetupStepActionOpenConsole ||
				method.SetupGuide.Steps[1].Action != integrations.AuthSetupStepActionOpenDocumentation {
				t.Fatalf("app Bearer setup step actions = %#v", method.SetupGuide.Steps)
			}
			continue
		}
		if method.OAuth == nil || len(method.OAuth.ClientFields) != 2 ||
			method.SetupGuide == nil || len(method.SetupGuide.Steps) != 6 ||
			!strings.HasPrefix(method.SetupGuide.ConsoleURL, "https://") ||
			!strings.HasPrefix(method.SetupGuide.DocumentationURL, "https://") ||
			!method.OAuth.ConnectEnabled || !method.OAuth.ScopeUpgradeEnabled ||
			!slices.Equal(method.OAuth.IdentityScopes, []string{ScopeUsersRead, ScopePostsRead}) ||
			!strings.HasPrefix(method.OAuth.ProviderSetupURL, "https://") {
			t.Fatalf("OAuth method = %#v", method)
		}
		if len(method.SetupGuide.Notices) != 2 ||
			!strings.Contains(method.SetupGuide.Notices[0].Text, "public clients") {
			t.Fatalf("OAuth setup notices = %#v", method.SetupGuide.Notices)
		}
		if method.SetupGuide.Steps[1].ID != "enable_oauth2" ||
			method.SetupGuide.Steps[1].Action != integrations.AuthSetupStepActionOpenDocumentation ||
			method.SetupGuide.Steps[3].ID != "configure_callback" ||
			method.SetupGuide.Steps[3].Action != integrations.AuthSetupStepActionCopyCallbackURL {
			t.Fatalf("OAuth setup step actions = %#v", method.SetupGuide.Steps)
		}
		if method.OAuth.ClientFields[0].Key != "client_id" || !method.OAuth.ClientFields[0].Required ||
			method.OAuth.ClientFields[1].Key != "client_secret" || method.OAuth.ClientFields[1].Required ||
			!method.OAuth.ClientFields[1].Secret {
			t.Fatalf("OAuth public/confidential client fields = %#v", method.OAuth.ClientFields)
		}
	}
	expectedScopes := map[string][]string{
		ActionGetAccount:        {ScopeUsersRead, ScopePostsRead},
		ActionListOwnPosts:      {ScopeUsersRead, ScopePostsRead},
		ActionSearchRecentPosts: {ScopeUsersRead, ScopePostsRead},
		ActionCreatePost:        {ScopeUsersRead, ScopePostsRead, ScopePostsWrite},
	}
	for _, action := range definition.Actions {
		if !integrations.ActionSupportsAuthMethod(action, AccountOAuthAuthMethodID) ||
			!integrations.ActionSupportsAuthMethod(action, OrganizationOAuthAuthMethodID) {
			t.Fatalf("action authentication compatibility = %#v", action)
		}
		appBearerSupported := integrations.ActionSupportsAuthMethod(action, AppBearerAuthMethodID)
		if appBearerSupported != (action.ID == ActionSearchRecentPosts) {
			t.Fatalf("app Bearer compatibility for %s = %v", action.ID, appBearerSupported)
		}
		if !slices.Equal(action.RequiredScopes, expectedScopes[action.ID]) {
			t.Fatalf("%s scopes = %#v, want %#v", action.ID, action.RequiredScopes, expectedScopes[action.ID])
		}
		switch action.ID {
		case ActionSearchRecentPosts:
			if action.DefaultPolicy == nil || action.DefaultPolicy.Enabled {
				t.Fatalf("recent search must be plan-gated: %#v", action)
			}
		case ActionCreatePost:
			if action.DefaultPolicy == nil || action.DefaultPolicy.Enabled ||
				action.DefaultPolicy.ApprovalPolicy != toolgovernance.ApprovalPolicyAlwaysAsk ||
				action.Idempotent || action.Effect != toolgovernance.EffectExternalSend {
				t.Fatalf("unsafe create defaults = %#v", action)
			}
		}
	}
}

func TestProviderDefinitionRegisters(t *testing.T) {
	adapter, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	registry := integrations.NewRegistry()
	if err := registry.Register(integrations.Registration{Definition: ProviderDefinition(), Adapter: adapter}); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.OAuthProvider(IntegrationID, DriverID); !ok {
		t.Fatal("OAuth provider was not registered")
	}
}

func TestAuthorizationURLUsesPKCEAndOfflineAccess(t *testing.T) {
	adapter, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := adapter.AuthorizationURL(integrations.OAuthAuthorizationRequest{
		Client:      integrations.OAuthClient{ClientID: "x-client"},
		RedirectURI: "https://zgi.example/oauth/callback", State: "opaque-state",
		CodeChallenge: "challenge", CodeChallengeMethod: integrations.OAuthPKCEChallengeMethodS256,
		Scopes: []string{ScopeUsersRead, ScopePostsRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if parsed.Host != "x.com" || query.Get("state") != "opaque-state" ||
		query.Get("code_challenge") != "challenge" {
		t.Fatalf("authorization URL = %s", raw)
	}
	if scopes := strings.Fields(query.Get("scope")); !slices.Equal(scopes, []string{ScopeUsersRead, ScopePostsRead, "offline.access"}) {
		t.Fatalf("authorization scopes = %#v", scopes)
	}
}

func TestCreatePostUsesOneRequestAndNoRetry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.URL.Path != "/2/tweets" || request.Header.Get("Authorization") != "Bearer access-token" {
			t.Errorf("request = %s %s headers=%#v", request.Method, request.URL, request.Header)
		}
		var body map[string]string
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		if body["text"] != "hello from ZGI" {
			t.Errorf("body = %#v", body)
		}
		writer.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(writer, `{"detail":"access-token"}`)
	}))
	defer server.Close()
	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: ActionCreatePost,
		Connection: &integrations.ResolvedConnection{
			IntegrationID: IntegrationID, DriverID: DriverID,
			Credentials: map[string]string{"access_token": "access-token"},
		},
		Input: map[string]interface{}{"text": "hello from ZGI"},
	})
	if err == nil || calls.Load() != 1 {
		t.Fatalf("err = %v, calls = %d", err, calls.Load())
	}
	if strings.Contains(err.Error(), "access-token") {
		t.Fatalf("secret leaked: %v", err)
	}
}

func TestOAuthExchangeUsesBasicAuthAndBoundedTokenResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/2/oauth2/token" {
			http.NotFound(writer, request)
			return
		}
		clientID, clientSecret, ok := request.BasicAuth()
		if !ok || clientID != "client-id" || clientSecret != "client-secret" {
			t.Errorf("basic auth = %q %q %v", clientID, clientSecret, ok)
		}
		if err := request.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if request.Form.Get("grant_type") != "authorization_code" ||
			request.Form.Get("code_verifier") != "verifier" || request.Form.Get("code") != "code" {
			t.Errorf("form = %#v", request.Form)
		}
		if request.Form.Get("client_id") != "" || request.URL.Query().Get("client_id") != "" {
			t.Errorf("confidential client ID must only use basic auth: form=%#v query=%q", request.Form, request.URL.RawQuery)
		}
		_, _ = io.WriteString(writer, `{"access_token":"new-access","refresh_token":"new-refresh","token_type":"bearer","expires_in":7200,"scope":"users.read tweet.read offline.access"}`)
	}))
	defer server.Close()
	httpClient := &http.Client{Transport: rewriteTransport{target: mustURL(t, server.URL)}}
	adapter, err := newForBaseURL(httpClient, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := adapter.ExchangeCode(context.Background(), integrations.OAuthCodeExchangeRequest{
		Client: integrations.OAuthClient{ClientID: "client-id", ClientSecret: "client-secret"},
		Code:   "code", RedirectURI: "https://zgi.example/oauth/callback", CodeVerifier: "verifier",
	})
	if err != nil || tokens.AccessToken != "new-access" || tokens.RefreshToken != "new-refresh" ||
		len(tokens.Scopes) != 3 || tokens.ExpiresAt == nil {
		t.Fatalf("tokens = %#v, err = %v", tokens, err)
	}
}

func TestOAuthExchangeSendsPublicClientIDInFormBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/2/oauth2/token" {
			http.NotFound(writer, request)
			return
		}
		if _, _, ok := request.BasicAuth(); ok {
			t.Error("public client request unexpectedly used basic authentication")
		}
		if request.URL.Query().Get("client_id") != "" {
			t.Errorf("public client ID leaked into query: %q", request.URL.RawQuery)
		}
		if err := request.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if request.Form.Get("client_id") != "public-client-id" ||
			request.Form.Get("grant_type") != "authorization_code" {
			t.Errorf("form = %#v", request.Form)
		}
		_, _ = io.WriteString(writer, `{"access_token":"new-access","refresh_token":"new-refresh","token_type":"bearer","expires_in":7200}`)
	}))
	defer server.Close()
	httpClient := &http.Client{Transport: rewriteTransport{target: mustURL(t, server.URL)}}
	adapter, err := newForBaseURL(httpClient, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.ExchangeCode(context.Background(), integrations.OAuthCodeExchangeRequest{
		Client: integrations.OAuthClient{ClientID: "public-client-id"},
		Code:   "code", RedirectURI: "https://zgi.example/oauth/callback", CodeVerifier: "verifier",
	}); err != nil {
		t.Fatalf("ExchangeCode() error = %v", err)
	}
}

func TestOAuthRefreshSendsPublicClientIDInFormBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/2/oauth2/token" {
			http.NotFound(writer, request)
			return
		}
		assertPublicOAuthClientRequest(t, request, "public-client-id")
		if request.Form.Get("grant_type") != "refresh_token" ||
			request.Form.Get("refresh_token") != "rotating-refresh-token" {
			t.Errorf("form = %#v", request.Form)
		}
		_, _ = io.WriteString(writer, `{"access_token":"new-access","refresh_token":"next-refresh","token_type":"bearer","expires_in":7200}`)
	}))
	defer server.Close()
	httpClient := &http.Client{Transport: rewriteTransport{target: mustURL(t, server.URL)}}
	adapter, err := newForBaseURL(httpClient, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := adapter.RefreshToken(context.Background(), integrations.OAuthRefreshRequest{
		Client:       integrations.OAuthClient{ClientID: "public-client-id"},
		RefreshToken: "rotating-refresh-token",
	})
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}
	if tokens.AccessToken != "new-access" || tokens.RefreshToken != "next-refresh" {
		t.Fatalf("RefreshToken() tokens = %#v", tokens)
	}
}

func TestOAuthRefreshUsesBasicAuthForConfidentialClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/2/oauth2/token" {
			http.NotFound(writer, request)
			return
		}
		assertConfidentialOAuthClientRequest(t, request, "client-id", "client-secret")
		if request.Form.Get("grant_type") != "refresh_token" ||
			request.Form.Get("refresh_token") != "rotating-refresh-token" {
			t.Errorf("form = %#v", request.Form)
		}
		_, _ = io.WriteString(writer, `{"access_token":"new-access","refresh_token":"next-refresh","token_type":"bearer","expires_in":7200}`)
	}))
	defer server.Close()
	httpClient := &http.Client{Transport: rewriteTransport{target: mustURL(t, server.URL)}}
	adapter, err := newForBaseURL(httpClient, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.RefreshToken(context.Background(), integrations.OAuthRefreshRequest{
		Client: integrations.OAuthClient{
			ClientID: "client-id", ClientSecret: "client-secret",
		},
		RefreshToken: "rotating-refresh-token",
	}); err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}
}

func TestOAuthRevokeSendsPublicClientIDInFormBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/2/oauth2/revoke" {
			http.NotFound(writer, request)
			return
		}
		assertPublicOAuthClientRequest(t, request, "public-client-id")
		if request.Form.Get("token") != "refresh-value" ||
			request.Form.Get("token_type_hint") != "refresh_token" {
			t.Errorf("form = %#v", request.Form)
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	httpClient := &http.Client{Transport: rewriteTransport{target: mustURL(t, server.URL)}}
	adapter, err := newForBaseURL(httpClient, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.RevokeToken(context.Background(), integrations.OAuthRevokeRequest{
		Client:        integrations.OAuthClient{ClientID: "public-client-id"},
		Token:         "refresh-value",
		TokenTypeHint: "refresh_token",
	}); err != nil {
		t.Fatalf("RevokeToken() error = %v", err)
	}
}

func assertPublicOAuthClientRequest(t *testing.T, request *http.Request, wantClientID string) {
	t.Helper()
	if _, _, ok := request.BasicAuth(); ok {
		t.Error("public client request unexpectedly used basic authentication")
	}
	if request.URL.Query().Get("client_id") != "" {
		t.Errorf("public client ID leaked into query: %q", request.URL.RawQuery)
	}
	if err := request.ParseForm(); err != nil {
		t.Fatalf("parse form: %v", err)
	}
	if request.Form.Get("client_id") != wantClientID {
		t.Errorf("client_id = %q, want %q; form = %#v", request.Form.Get("client_id"), wantClientID, request.Form)
	}
}

func assertConfidentialOAuthClientRequest(
	t *testing.T,
	request *http.Request,
	wantClientID string,
	wantClientSecret string,
) {
	t.Helper()
	clientID, clientSecret, ok := request.BasicAuth()
	if !ok || clientID != wantClientID || clientSecret != wantClientSecret {
		t.Errorf("basic auth = %q %q %v", clientID, clientSecret, ok)
	}
	if request.URL.Query().Get("client_id") != "" {
		t.Errorf("confidential client ID leaked into query: %q", request.URL.RawQuery)
	}
	if err := request.ParseForm(); err != nil {
		t.Fatalf("parse form: %v", err)
	}
	if request.Form.Get("client_id") != "" {
		t.Errorf("confidential client ID was duplicated in body: form=%#v", request.Form)
	}
}

func TestOAuthRevokePreservesRefreshTokenHint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/2/oauth2/revoke" {
			http.NotFound(writer, request)
			return
		}
		if err := request.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		clientID, clientSecret, ok := request.BasicAuth()
		if !ok || clientID != "client-id" || clientSecret != "client-secret" ||
			request.Form.Get("client_id") != "" || request.URL.Query().Get("client_id") != "" {
			t.Errorf("confidential client authentication: %q %q %v form=%#v query=%q",
				clientID, clientSecret, ok, request.Form, request.URL.RawQuery)
		}
		if request.Form.Get("token") != "refresh-value" || request.Form.Get("token_type_hint") != "refresh_token" {
			t.Errorf("form = %#v", request.Form)
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	httpClient := &http.Client{Transport: rewriteTransport{target: mustURL(t, server.URL)}}
	adapter, err := newForBaseURL(httpClient, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = adapter.RevokeToken(context.Background(), integrations.OAuthRevokeRequest{
		Client: integrations.OAuthClient{ClientID: "client-id", ClientSecret: "client-secret"},
		Token:  "refresh-value", TokenTypeHint: "refresh_token",
	})
	if err != nil {
		t.Fatalf("RevokeToken() error = %v", err)
	}
}

func TestOAuthTokenRequestDoesNotFollowRedirectsOrMutateCallerClient(t *testing.T) {
	var forwarded atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		forwarded.Add(1)
		_, _ = io.Copy(io.Discard, request.Body)
		writer.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()

	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			redirector := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Location", sink.URL)
				writer.WriteHeader(status)
			}))
			defer redirector.Close()

			callerClient := &http.Client{Transport: rewriteTransport{target: mustURL(t, redirector.URL)}}
			adapter, err := newForBaseURL(callerClient, redirector.URL)
			if err != nil {
				t.Fatal(err)
			}
			if callerClient.CheckRedirect != nil {
				t.Fatal("constructor mutated the caller-provided HTTP client")
			}
			_, err = adapter.ExchangeCode(context.Background(), integrations.OAuthCodeExchangeRequest{
				Client: integrations.OAuthClient{ClientID: "client-id", ClientSecret: "client-secret"},
				Code:   "one-time-code", RedirectURI: "https://zgi.example/oauth/callback", CodeVerifier: "verifier",
			})
			if err == nil {
				t.Fatal("redirecting token endpoint unexpectedly succeeded")
			}
		})
	}
	if forwarded.Load() != 0 {
		t.Fatalf("OAuth secret-bearing request followed a redirect %d time(s)", forwarded.Load())
	}
}

func TestOAuthExchangePreservesRequestedScopesWhenResponseOmitsScope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/2/oauth2/token" {
			http.NotFound(writer, request)
			return
		}
		_, _ = io.WriteString(writer, `{"access_token":"access","refresh_token":"refresh","token_type":"Bearer","expires_in":7200}`)
	}))
	defer server.Close()
	client := &http.Client{Transport: rewriteTransport{target: mustURL(t, server.URL)}}
	adapter, err := newForBaseURL(client, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{ScopeUsersRead, ScopePostsWrite}
	tokens, err := adapter.ExchangeCode(context.Background(), integrations.OAuthCodeExchangeRequest{
		Client: integrations.OAuthClient{ClientID: "client-id", ClientSecret: "client-secret"},
		Code:   "one-time-code", RedirectURI: "https://zgi.example/oauth/callback", CodeVerifier: "verifier",
		Scopes: expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(tokens.Scopes, expected) {
		t.Fatalf("token scopes = %#v", tokens.Scopes)
	}
}

func TestAccountOutputIsBounded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/2/users/me" {
			http.NotFound(writer, request)
			return
		}
		_, _ = io.WriteString(writer, `{"data":{"id":"1","name":"Owner","username":"owner","description":"profile","verified":true,"profile_image_url":"https://pbs.twimg.com/avatar.jpg","public_metrics":{"followers_count":10}}}`)
	}))
	defer server.Close()
	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: ActionGetAccount,
		Connection: &integrations.ResolvedConnection{
			IntegrationID: IntegrationID, DriverID: DriverID,
			Credentials: map[string]string{"access_token": "token"},
		},
	})
	if err != nil || result.Output["provider"] != IntegrationID {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
}

type rewriteTransport struct {
	target *url.URL
}

func (transport rewriteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	urlCopy := *request.URL
	urlCopy.Scheme = transport.target.Scheme
	urlCopy.Host = transport.target.Host
	clone.URL = &urlCopy
	return http.DefaultTransport.RoundTrip(clone)
}

func mustURL(t *testing.T, value string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
