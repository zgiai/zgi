package feishu

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

func TestProviderDefinitionSeparatesUserOAuthAndTenantApp(t *testing.T) {
	definition := ProviderDefinition()
	if definition.ID != IntegrationID || definition.DriverID != DriverID || len(definition.AuthMethods) != 3 {
		t.Fatalf("definition = %#v", definition)
	}
	var oauthCount, tenantCount int
	for _, method := range definition.AuthMethods {
		switch method.Type {
		case integrations.AuthMethodTypeOAuth2:
			oauthCount++
			if method.OAuth == nil || len(method.OAuth.ClientFields) != 2 ||
				method.SetupGuide == nil || len(method.SetupGuide.Steps) != 6 ||
				!strings.HasPrefix(method.SetupGuide.ConsoleURL, "https://") ||
				!strings.HasPrefix(method.SetupGuide.DocumentationURL, "https://") ||
				method.IdentityKind != integrations.AuthIdentityKindUser ||
				method.AcquisitionStrategy != integrations.AuthAcquisitionStrategyBrowserRedirect ||
				method.LifecycleStrategy != integrations.AuthLifecycleStrategyOAuthRefresh ||
				method.RequestAuthStrategy != integrations.RequestAuthStrategyBearerHeader ||
				!slices.Equal(method.OAuth.IdentityScopes, []string{ScopeOfflineAccess}) ||
				!strings.HasPrefix(method.OAuth.ProviderSetupURL, "https://") {
				t.Fatalf("OAuth method = %#v", method)
			}
			if len(method.SetupGuide.Notices) != 2 ||
				!strings.Contains(method.SetupGuide.Notices[0].Text, "user_profile") {
				t.Fatalf("OAuth setup notices = %#v", method.SetupGuide.Notices)
			}
			if method.SetupGuide.Steps[2].ID != "configure_callback" ||
				method.SetupGuide.Steps[2].Action != integrations.AuthSetupStepActionCopyCallbackURL ||
				method.SetupGuide.Steps[3].ID != "request_permissions" ||
				method.SetupGuide.Steps[3].Action != integrations.AuthSetupStepActionOpenDocumentation {
				t.Fatalf("OAuth setup step actions = %#v", method.SetupGuide.Steps)
			}
		case integrations.AuthMethodTypeServiceAccount:
			tenantCount++
			if method.IdentityKind != integrations.AuthIdentityKindApplication ||
				method.AcquisitionStrategy != integrations.AuthAcquisitionStrategyManualForm ||
				method.LifecycleStrategy != integrations.AuthLifecycleStrategyExchangeOnDemand ||
				method.RequestAuthStrategy != integrations.RequestAuthStrategyBearerHeader ||
				len(method.Fields) != 2 ||
				method.SetupGuide == nil || len(method.SetupGuide.Steps) != 4 ||
				!strings.HasPrefix(method.SetupGuide.ConsoleURL, "https://") ||
				!strings.HasPrefix(method.SetupGuide.DocumentationURL, "https://") {
				t.Fatalf("tenant method = %#v", method)
			}
			if method.SetupGuide.Steps[0].Action != integrations.AuthSetupStepActionOpenConsole ||
				method.SetupGuide.Steps[1].Action != integrations.AuthSetupStepActionOpenDocumentation {
				t.Fatalf("tenant setup step actions = %#v", method.SetupGuide.Steps)
			}
		}
	}
	if oauthCount != 2 || tenantCount != 1 {
		t.Fatalf("OAuth = %d, tenant = %d", oauthCount, tenantCount)
	}
	for _, action := range definition.Actions {
		userOAuthSupported := integrations.ActionSupportsAuthMethod(action, UserOAuthAuthMethodID) &&
			integrations.ActionSupportsAuthMethod(action, OrganizationOAuthAuthMethodID)
		tenantAppSupported := integrations.ActionSupportsAuthMethod(action, TenantAppAuthMethodID)
		switch action.ID {
		case ActionGetAccount, ActionListDriveFiles, ActionReadDocument, ActionSendUserMessage:
			if !userOAuthSupported || tenantAppSupported {
				t.Fatalf("user OAuth action authentication compatibility = %#v", action)
			}
		case ActionSendBotMessage:
			if userOAuthSupported || !tenantAppSupported {
				t.Fatalf("tenant app action authentication compatibility = %#v", action)
			}
		default:
			t.Fatalf("unexpected action in authentication matrix: %q", action.ID)
		}
		if action.Effect == toolgovernance.EffectExternalSend {
			if action.DefaultPolicy == nil || action.DefaultPolicy.Enabled ||
				action.DefaultPolicy.ApprovalPolicy != toolgovernance.ApprovalPolicyAlwaysAsk || action.Idempotent {
				t.Fatalf("unsafe send defaults = %#v", action)
			}
		}
		if action.ID == ActionGetAccount && len(action.RequiredScopes) != 0 {
			t.Fatalf("account action requested implicit scopes: %#v", action.RequiredScopes)
		}
		if action.ID == ActionSendUserMessage {
			expectedScopes := []string{ScopeSendAsUser}
			if !slices.Equal(action.RequiredScopes, expectedScopes) {
				t.Fatalf("send-user scopes = %#v, want %#v", action.RequiredScopes, expectedScopes)
			}
			for _, scope := range expectedScopes {
				labels := action.ScopeLabelsI18n[scope]
				if strings.TrimSpace(labels[integrations.LocaleEnglishUS]) == "" ||
					strings.TrimSpace(labels[integrations.LocaleSimplifiedChinese]) == "" {
					t.Fatalf("scope %q labels = %#v", scope, labels)
				}
			}
		}
		if action.ID == ActionSendBotMessage && !slices.Equal(action.RequiredScopes, []string{ScopeSendAsBot}) {
			t.Fatalf("send-bot scopes = %#v", action.RequiredScopes)
		}
	}
}

func TestAuthorizationURLRequestsOnlyFeishuApplicationScopes(t *testing.T) {
	adapter, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := adapter.AuthorizationURL(integrations.OAuthAuthorizationRequest{
		Client: integrations.OAuthClient{
			ClientID: "client-id",
			Config:   map[string]interface{}{"region": RegionCN},
		},
		RedirectURI:         "https://zgi.example/oauth/callback",
		State:               "state",
		CodeChallenge:       "challenge",
		CodeChallengeMethod: integrations.OAuthPKCEChallengeMethodS256,
		Scopes:              []string{ScopeOfflineAccess},
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	scopes := strings.Fields(parsed.Query().Get("scope"))
	if !slices.Equal(scopes, []string{ScopeOfflineAccess}) {
		t.Fatalf("authorization scopes = %#v", scopes)
	}
	if strings.Contains(raw, "user_profile") || strings.Contains(raw, "auth%3Auser.id%3Aread") {
		t.Fatalf("authorization URL leaked implicit token scopes: %s", raw)
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

func TestBusinessCodeInHTTP200IsAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Tt-Logid", "log-1")
		_, _ = io.WriteString(writer, `{"code":99991668,"msg":"access-secret"}`)
	}))
	defer server.Close()
	adapter, err := newForBaseURLs(server.Client(), server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: ActionGetAccount,
		Connection: &integrations.ResolvedConnection{
			IntegrationID: IntegrationID, DriverID: DriverID, AuthMethodID: UserOAuthAuthMethodID,
			Credentials: map[string]string{"access_token": "access-secret"},
			Config:      map[string]interface{}{"region": RegionCN},
		},
	})
	if integrations.ErrorCode(err) != integrations.ErrorCodeAuthInvalid {
		t.Fatalf("error = %v (%s)", err, integrations.ErrorCode(err))
	}
	if strings.Contains(err.Error(), "access-secret") {
		t.Fatalf("secret leaked: %v", err)
	}
}

func TestSendUserMessageUsesContractAndNeverRetries(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.URL.Path != "/open-apis/im/v1/messages" ||
			request.URL.Query().Get("receive_id_type") != "open_id" ||
			request.Header.Get("Authorization") != "Bearer user-token" {
			t.Errorf("request = %s %s headers=%#v", request.Method, request.URL, request.Header)
		}
		var payload struct {
			ReceiveID string `json:"receive_id"`
			MsgType   string `json:"msg_type"`
			Content   string `json:"content"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if payload.ReceiveID != "ou_target" || payload.MsgType != "text" || !strings.Contains(payload.Content, "hello") {
			t.Errorf("payload = %#v", payload)
		}
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(writer, `{"code":230020,"msg":"rate limited"}`)
	}))
	defer server.Close()
	adapter, err := newForBaseURLs(server.Client(), server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: ActionSendUserMessage,
		Connection: &integrations.ResolvedConnection{
			IntegrationID: IntegrationID, DriverID: DriverID, AuthMethodID: UserOAuthAuthMethodID,
			Credentials: map[string]string{"access_token": "user-token"}, Config: map[string]interface{}{"region": RegionCN},
		},
		Input: map[string]interface{}{"receive_id": "ou_target", "receive_id_type": "open_id", "text": "hello"},
	})
	if integrations.ErrorCode(err) != integrations.ErrorCodeRateLimited || calls.Load() != 1 {
		t.Fatalf("err = %v, calls = %d", err, calls.Load())
	}
}

func TestRefreshRequiresRotatedRefreshToken(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != mustURL(t, feishuOAuthTokenEndpoint).Path {
			http.NotFound(writer, request)
			return
		}
		calls.Add(1)
		var body map[string]string
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		if body["grant_type"] != "refresh_token" || body["refresh_token"] != "old-refresh" ||
			body["client_id"] != "client-id" || body["client_secret"] != "client-secret" {
			t.Errorf("body = %#v", body)
		}
		if calls.Load() == 1 {
			_, _ = io.WriteString(writer, `{"code":0,"access_token":"new-access","expires_in":7200,"scope":"offline_access auth:user.id:read user_profile","token_type":"Bearer"}`)
			return
		}
		_, _ = io.WriteString(writer, `{"code":0,"access_token":"new-access","refresh_token":"new-refresh","expires_in":7200,"refresh_token_expires_in":2592000,"scope":"offline_access auth:user.id:read user_profile","token_type":"Bearer"}`)
	}))
	defer server.Close()
	httpClient := &http.Client{Transport: rewriteTransport{target: mustURL(t, server.URL)}}
	adapter, err := newForBaseURLs(httpClient, server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	request := integrations.OAuthRefreshRequest{
		Client: integrations.OAuthClient{
			ClientID: "client-id", ClientSecret: "client-secret", Config: map[string]interface{}{"region": RegionCN},
		},
		RefreshToken: "old-refresh", Scopes: []string{ScopeOfflineAccess},
	}
	if _, err := adapter.RefreshToken(context.Background(), request); integrations.ErrorCode(err) != integrations.ErrorCodeResponseInvalid {
		t.Fatalf("missing rotation error = %v", err)
	}
	tokens, err := adapter.RefreshToken(context.Background(), request)
	if err != nil || tokens.RefreshToken != "new-refresh" || tokens.AccessToken != "new-access" {
		t.Fatalf("tokens = %#v, err = %v", tokens, err)
	}
	if tokens.RefreshTokenExpiresAt == nil ||
		tokens.RefreshTokenExpiresAt.Before(time.Now().UTC().Add(29*24*time.Hour)) ||
		tokens.RefreshTokenExpiresAt.After(time.Now().UTC().Add(31*24*time.Hour)) {
		t.Fatalf("refresh token expiry = %v", tokens.RefreshTokenExpiresAt)
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
			adapter, err := newForBaseURLs(callerClient, redirector.URL, redirector.URL)
			if err != nil {
				t.Fatal(err)
			}
			if callerClient.CheckRedirect != nil {
				t.Fatal("constructor mutated the caller-provided HTTP client")
			}
			_, err = adapter.ExchangeCode(context.Background(), integrations.OAuthCodeExchangeRequest{
				Client: integrations.OAuthClient{
					ClientID: "client-id", ClientSecret: "client-secret",
					Config: map[string]interface{}{"region": RegionCN},
				},
				Code: "one-time-code", RedirectURI: "https://zgi.example/oauth/callback", CodeVerifier: "verifier",
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
		if request.URL.Path != mustURL(t, feishuOAuthTokenEndpoint).Path {
			http.NotFound(writer, request)
			return
		}
		_, _ = io.WriteString(writer, `{"code":0,"access_token":"access","refresh_token":"refresh","token_type":"Bearer","expires_in":7200}`)
	}))
	defer server.Close()
	httpClient := &http.Client{Transport: rewriteTransport{target: mustURL(t, server.URL)}}
	adapter, err := newForBaseURLs(httpClient, server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{ScopeOfflineAccess, ScopeSendAsUser}
	tokens, err := adapter.ExchangeCode(context.Background(), integrations.OAuthCodeExchangeRequest{
		Client: integrations.OAuthClient{
			ClientID: "client-id", ClientSecret: "client-secret",
			Config: map[string]interface{}{"region": RegionCN},
		},
		Code: "one-time-code", RedirectURI: "https://zgi.example/oauth/callback", CodeVerifier: "verifier",
		Scopes: expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(tokens.Scopes, expected) {
		t.Fatalf("token scopes = %#v", tokens.Scopes)
	}
}

func TestOAuthTokenRequestsUseCurrentOpenAPIEndpointAndJSON(t *testing.T) {
	var calls atomic.Int32
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		endpoint := mustURL(t, feishuOAuthTokenEndpoint)
		if request.Method != http.MethodPost || request.URL.Scheme != endpoint.Scheme ||
			request.URL.Host != endpoint.Host || request.URL.Path != endpoint.Path ||
			request.URL.RawQuery != "" {
			t.Fatalf("request URL = %s %s", request.Method, request.URL)
		}
		if request.Header.Get("Content-Type") != "application/json; charset=utf-8" {
			t.Fatalf("Content-Type = %q", request.Header.Get("Content-Type"))
		}
		var body map[string]string
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		switch body["grant_type"] {
		case "authorization_code":
			if body["client_id"] != "client-id" || body["client_secret"] != "client-secret" ||
				body["code"] != "one-time-code" || body["redirect_uri"] != "https://zgi.example/oauth/callback" ||
				body["code_verifier"] != "pkce-verifier" {
				t.Fatalf("exchange body = %#v", body)
			}
		case "refresh_token":
			if body["client_id"] != "client-id" || body["client_secret"] != "client-secret" ||
				body["refresh_token"] != "old-refresh" || body["code_verifier"] != "" {
				t.Fatalf("refresh body = %#v", body)
			}
		default:
			t.Fatalf("grant_type = %q", body["grant_type"])
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"code":"0","access_token":"access","refresh_token":"refresh","token_type":"Bearer","expires_in":7200}`,
			)),
			Request: request,
		}, nil
	})
	adapter, err := newForBaseURLs(&http.Client{Transport: transport}, "https://open.feishu.invalid", "https://open.larksuite.invalid")
	if err != nil {
		t.Fatal(err)
	}
	client := integrations.OAuthClient{
		ClientID: "client-id", ClientSecret: "client-secret",
		Config: map[string]interface{}{"region": RegionCN},
	}
	if _, err := adapter.ExchangeCode(context.Background(), integrations.OAuthCodeExchangeRequest{
		Client: client, Code: "one-time-code", RedirectURI: "https://zgi.example/oauth/callback",
		CodeVerifier: "pkce-verifier", Scopes: []string{ScopeOfflineAccess},
	}); err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if _, err := adapter.RefreshToken(context.Background(), integrations.OAuthRefreshRequest{
		Client: client, RefreshToken: "old-refresh", Scopes: []string{ScopeOfflineAccess},
	}); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d", calls.Load())
	}
}

func TestOAuthBusinessCodeWinsOverHTTPStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(writer, `{"code":"20026","msg":"invalid authorization code"}`)
	}))
	defer server.Close()
	httpClient := &http.Client{Transport: rewriteTransport{target: mustURL(t, server.URL)}}
	adapter, err := newForBaseURLs(httpClient, server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.ExchangeCode(context.Background(), integrations.OAuthCodeExchangeRequest{
		Client: integrations.OAuthClient{
			ClientID: "client-id", ClientSecret: "client-secret",
			Config: map[string]interface{}{"region": RegionCN},
		},
		Code: "one-time-code", RedirectURI: "https://zgi.example/oauth/callback", CodeVerifier: "verifier",
	})
	if integrations.ErrorCode(err) != integrations.ErrorCodeAuthInvalid {
		t.Fatalf("error = %v (%s)", err, integrations.ErrorCode(err))
	}
}

func TestOpenAPIBusinessCodeWinsOverHTTPStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(writer, `{"code":230027,"msg":"missing permission"}`)
	}))
	defer server.Close()
	adapter, err := newForBaseURLs(server.Client(), server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: ActionGetAccount,
		Connection: &integrations.ResolvedConnection{
			IntegrationID: IntegrationID, DriverID: DriverID, AuthMethodID: UserOAuthAuthMethodID,
			Credentials: map[string]string{"access_token": "access-token"},
			Config:      map[string]interface{}{"region": RegionCN},
		},
	})
	if integrations.ErrorCode(err) != integrations.ErrorCodeAccessDenied {
		t.Fatalf("error = %v (%s)", err, integrations.ErrorCode(err))
	}
}

func TestFeishuBusinessCodeMapping(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{20002, integrations.ErrorCodeAuthInvalid},
		{20026, integrations.ErrorCodeAuthInvalid},
		{20037, integrations.ErrorCodeAuthInvalid},
		{20049, integrations.ErrorCodeAuthInvalid},
		{20064, integrations.ErrorCodeAuthInvalid},
		{20073, integrations.ErrorCodeAuthInvalid},
		{20050, integrations.ErrorCodeUpstream},
		{20072, integrations.ErrorCodeUpstream},
		{230020, integrations.ErrorCodeRateLimited},
		{230027, integrations.ErrorCodeAccessDenied},
		{230035, integrations.ErrorCodeAccessDenied},
	}
	for _, test := range tests {
		t.Run(strconv.Itoa(test.code), func(t *testing.T) {
			err := mapFeishuBusinessCode(test.code)
			if got := integrations.ErrorCode(err); got != test.want {
				t.Fatalf("code %d: got %q, want %q (%v)", test.code, got, test.want, err)
			}
		})
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func mustURL(t *testing.T, value string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
