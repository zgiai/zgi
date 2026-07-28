package gmail

import (
	"context"
	"encoding/base64"
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

func TestProviderDefinitionOAuthAndWriteDefaults(t *testing.T) {
	definition := ProviderDefinition()
	if definition.ID != IntegrationID || definition.DriverID != DriverID || len(definition.AuthMethods) != 2 {
		t.Fatalf("definition = %#v", definition)
	}
	for _, method := range definition.AuthMethods {
		if method.Type != integrations.AuthMethodTypeOAuth2 || method.OAuth == nil ||
			method.SetupGuide == nil || len(method.SetupGuide.Steps) != 6 ||
			!strings.HasPrefix(method.SetupGuide.ConsoleURL, "https://") ||
			!strings.HasPrefix(method.SetupGuide.DocumentationURL, "https://") ||
			method.IdentityKind != integrations.AuthIdentityKindUser ||
			method.AcquisitionStrategy != integrations.AuthAcquisitionStrategyBrowserRedirect ||
			method.LifecycleStrategy != integrations.AuthLifecycleStrategyOAuthRefresh ||
			method.RequestAuthStrategy != integrations.RequestAuthStrategyBearerHeader ||
			!method.OAuth.ConnectEnabled || len(method.OAuth.ClientFields) != 2 ||
			!slices.Equal(method.OAuth.IdentityScopes, []string{ScopeOpenID, ScopeEmail, ScopeProfile}) ||
			!strings.HasPrefix(method.OAuth.ProviderSetupURL, "https://") {
			t.Fatalf("OAuth method = %#v", method)
		}
		if len(method.SetupGuide.Notices) != 2 ||
			!strings.Contains(method.SetupGuide.Notices[0].Text, "test users") {
			t.Fatalf("OAuth setup notices = %#v", method.SetupGuide.Notices)
		}
		if method.SetupGuide.Steps[2].ID != "configure_consent" ||
			method.SetupGuide.Steps[2].Action != integrations.AuthSetupStepActionOpenDocumentation ||
			method.SetupGuide.Steps[4].ID != "configure_callback" ||
			method.SetupGuide.Steps[4].Action != integrations.AuthSetupStepActionCopyCallbackURL {
			t.Fatalf("OAuth setup step actions = %#v", method.SetupGuide.Steps)
		}
	}
	var account integrations.ActionDefinition
	var send integrations.ActionDefinition
	for _, action := range definition.Actions {
		if !integrations.ActionSupportsAuthMethod(action, AccountOAuthAuthMethodID) ||
			!integrations.ActionSupportsAuthMethod(action, OrganizationOAuthAuthMethodID) ||
			integrations.ActionSupportsAuthMethod(action, "future_service_account") {
			t.Fatalf("action authentication compatibility = %#v", action)
		}
		if action.ID == ActionGetAccount {
			account = action
		}
		if action.ID == ActionSendMail {
			send = action
		}
	}
	if !slices.Equal(account.RequiredScopes, []string{ScopeOpenID, ScopeEmail, ScopeProfile}) ||
		account.ScopeLabelsI18n[ScopeProfile][integrations.LocaleEnglishUS] == "" {
		t.Fatalf("account identity scopes = %#v", account)
	}
	if send.DefaultPolicy == nil || send.DefaultPolicy.Enabled ||
		send.DefaultPolicy.ApprovalPolicy != toolgovernance.ApprovalPolicyAlwaysAsk ||
		send.Idempotent || send.Effect != toolgovernance.EffectExternalSend {
		t.Fatalf("send action governance = %#v", send)
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

func TestAuthorizationURLUsesPKCEOfflineAccessAndServerEndpoint(t *testing.T) {
	adapter, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := adapter.AuthorizationURL(integrations.OAuthAuthorizationRequest{
		Client:      integrations.OAuthClient{ClientID: "google-client"},
		RedirectURI: "https://zgi.example/oauth/callback", State: "opaque-state",
		CodeChallenge: "challenge", CodeChallengeMethod: integrations.OAuthPKCEChallengeMethodS256,
		Scopes: []string{ScopeOpenID, ScopeEmail},
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if parsed.Host != "accounts.google.com" || query.Get("client_id") != "google-client" ||
		query.Get("state") != "opaque-state" || query.Get("code_challenge") != "challenge" ||
		query.Get("access_type") != "offline" || query.Get("prompt") != "consent" {
		t.Fatalf("authorization URL = %s", raw)
	}
	if scopes := strings.Fields(query.Get("scope")); !slices.Equal(scopes, []string{ScopeOpenID, ScopeEmail}) {
		t.Fatalf("authorization scopes = %#v", scopes)
	}
}

func TestAdapterAccountAndSendContracts(t *testing.T) {
	var sendCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer access-secret" ||
			!strings.HasPrefix(request.Header.Get("User-Agent"), "ZGI-External-Integrations") {
			t.Errorf("headers = %#v", request.Header)
		}
		writer.Header().Set("X-Google-Request-ID", "request-1")
		switch request.URL.Path {
		case "/v1/userinfo":
			_, _ = io.WriteString(writer, `{"sub":"user-1","email":"owner@example.com","email_verified":true,"name":"Owner","picture":"https://images.example.com/avatar"}`)
		case "/gmail/v1/users/me/messages/send":
			sendCalls.Add(1)
			var payload struct {
				Raw string `json:"raw"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Errorf("decode body: %v", err)
			}
			message, err := base64.RawURLEncoding.DecodeString(payload.Raw)
			if err != nil || !strings.Contains(string(message), "recipient@example.com") ||
				!strings.Contains(string(message), "Subject: Status") {
				t.Errorf("raw message = %q, err = %v", message, err)
			}
			_, _ = io.WriteString(writer, `{"id":"message-1","threadId":"thread-1","labelIds":["SENT"]}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter, err := newForBaseURLs(server.Client(), server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	connection := &integrations.ResolvedConnection{
		IntegrationID: IntegrationID, DriverID: DriverID,
		Credentials:   map[string]string{"access_token": "access-secret"},
		GrantedScopes: []string{ScopeOpenID, ScopeEmail, ScopeMailSend},
	}
	account, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: ActionGetAccount, Connection: connection, Input: map[string]interface{}{},
	})
	if err != nil || account.Output["provider"] != IntegrationID {
		t.Fatalf("account = %#v, err = %v", account, err)
	}
	sent, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: ActionSendMail, Connection: connection,
		Input: map[string]interface{}{
			"to": "recipient@example.com", "subject": "Status", "body_text": "Ready",
		},
	})
	if err != nil || sent.ResultCount != 1 || sendCalls.Load() != 1 {
		t.Fatalf("sent = %#v, calls = %d, err = %v", sent, sendCalls.Load(), err)
	}
}

func TestSendDoesNotRetryAndErrorsDoNotExposeSecrets(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		writer.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(writer, `{"error":"access-secret client-secret"}`)
	}))
	defer server.Close()
	adapter, err := newForBaseURLs(server.Client(), server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: ActionSendMail,
		Connection: &integrations.ResolvedConnection{
			IntegrationID: IntegrationID, DriverID: DriverID,
			Credentials: map[string]string{"access_token": "access-secret"},
		},
		Input: map[string]interface{}{"to": "to@example.com", "subject": "Subject", "body_text": "Body"},
	})
	if err == nil || calls.Load() != 1 {
		t.Fatalf("err = %v, calls = %d", err, calls.Load())
	}
	if result == nil || result.AttemptCount != 1 ||
		result.ResultCount != 0 || result.Output != nil ||
		result.ProviderDiagnostics.HTTPStatus != http.StatusServiceUnavailable {
		t.Fatalf("failure result = %#v", result)
	}
	if strings.Contains(err.Error(), "access-secret") || strings.Contains(err.Error(), "client-secret") {
		t.Fatalf("secret leaked: %v", err)
	}
}

func TestGoogleErrorReasonsAreClassifiedAndBounded(t *testing.T) {
	tests := []struct {
		reason string
		code   string
	}{
		{reason: "rateLimitExceeded", code: integrations.ErrorCodeRateLimited},
		{reason: "userRateLimitExceeded", code: integrations.ErrorCodeRateLimited},
		{reason: "dailyLimitExceeded", code: integrations.ErrorCodeBudgetExceeded},
		{reason: "domainPolicy", code: integrations.ErrorCodeAccessDenied},
		{reason: "insufficientPermissions", code: integrations.ErrorCodeAccessDenied},
	}
	for _, test := range tests {
		t.Run(test.reason, func(t *testing.T) {
			payload := []byte(`{"error":{"status":"PERMISSION_DENIED","message":"do not expose","errors":[{"reason":"` + test.reason + `","message":"do not expose"}]}}`)
			err, diagnostics := mapGoogleStatus(http.StatusForbidden, http.Header{}, payload, "request-1")
			if integrations.ErrorCode(err) != test.code {
				t.Fatalf("error = %v, want %s", err, test.code)
			}
			if diagnostics.ErrorCode != test.reason || diagnostics.RequestID != "request-1" ||
				diagnostics.HTTPStatus != http.StatusForbidden {
				t.Fatalf("diagnostics = %#v", diagnostics)
			}
			if strings.Contains(err.Error(), "do not expose") {
				t.Fatalf("provider message leaked: %v", err)
			}
		})
	}
}

func TestGmailReadRetriesRateLimitReason(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Google-Request-ID", "request-"+strconv.Itoa(int(calls.Add(1))))
		if calls.Load() == 1 {
			writer.Header().Set("Retry-After", "0")
			writer.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(writer, `{"error":{"errors":[{"reason":"rateLimitExceeded"}]}}`)
			return
		}
		_, _ = io.WriteString(writer, `{"sub":"user-1","email":"owner@example.com","name":"Owner"}`)
	}))
	defer server.Close()
	adapter, err := newForBaseURLs(server.Client(), server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: ActionGetAccount,
		Connection: &integrations.ResolvedConnection{
			IntegrationID: IntegrationID, DriverID: DriverID,
			Credentials: map[string]string{"access_token": "access-secret"},
		},
	})
	if err != nil || result == nil || calls.Load() != 2 || result.AttemptCount != 2 {
		t.Fatalf("result = %#v, calls = %d, err = %v", result, calls.Load(), err)
	}
}

func TestGmailReadRetriesGenericHTTP500(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		call := calls.Add(1)
		writer.Header().Set("X-Google-Request-ID", "gmail-500-"+strconv.Itoa(int(call)))
		if call == 1 {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(writer, `{"error":{"message":"temporary"}}`)
			return
		}
		_, _ = io.WriteString(writer, `{"sub":"user-1","email":"owner@example.com","name":"Owner"}`)
	}))
	defer server.Close()
	adapter, err := newForBaseURLs(server.Client(), server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: ActionGetAccount,
		Connection: &integrations.ResolvedConnection{
			IntegrationID: IntegrationID, DriverID: DriverID,
			Credentials: map[string]string{"access_token": "access-secret"},
		},
	})
	if err != nil || calls.Load() != 2 || result == nil || result.AttemptCount != 2 ||
		result.ProviderRequestID != "gmail-500-2" {
		t.Fatalf("result = %#v, calls = %d, err = %v", result, calls.Load(), err)
	}
}

func TestGmailRetryBackoffDeadlineReturnsTimeoutWithDiagnostics(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writer.Header().Set("X-Google-Request-ID", "gmail-retry-deadline")
		writer.Header().Set("Retry-After", "1")
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(writer, `{"error":{"errors":[{"reason":"rateLimitExceeded"}]}}`)
	}))
	defer server.Close()
	adapter, err := newForBaseURLs(server.Client(), server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	result, err := adapter.Execute(ctx, integrations.ActionRequest{
		ActionID: ActionGetAccount,
		Connection: &integrations.ResolvedConnection{
			IntegrationID: IntegrationID, DriverID: DriverID,
			Credentials: map[string]string{"access_token": "access-secret"},
		},
	})
	if integrations.ErrorCode(err) != integrations.ErrorCodeTimeout {
		t.Fatalf("error = %v (%s)", err, integrations.ErrorCode(err))
	}
	if calls.Load() != 1 || result == nil || result.AttemptCount != 1 ||
		result.ProviderRequestID != "gmail-retry-deadline" ||
		result.ProviderDiagnostics.ErrorCode != "rateLimitExceeded" ||
		result.ProviderDiagnostics.HTTPStatus != http.StatusTooManyRequests {
		t.Fatalf("result = %#v, calls = %d", result, calls.Load())
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

			callerClient := &http.Client{Transport: oauthRewriteTransport{target: mustOAuthURL(t, redirector.URL)}}
			adapter, err := newForBaseURLs(callerClient, redirector.URL, redirector.URL)
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
		if request.URL.Path != "/token" {
			http.NotFound(writer, request)
			return
		}
		_, _ = io.WriteString(writer, `{"access_token":"access","refresh_token":"refresh","token_type":"Bearer","expires_in":3600,"refresh_token_expires_in":7200}`)
	}))
	defer server.Close()
	client := &http.Client{Transport: oauthRewriteTransport{target: mustOAuthURL(t, server.URL)}}
	adapter, err := newForBaseURLs(client, server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := adapter.ExchangeCode(context.Background(), integrations.OAuthCodeExchangeRequest{
		Client: integrations.OAuthClient{ClientID: "client-id", ClientSecret: "client-secret"},
		Code:   "one-time-code", RedirectURI: "https://zgi.example/oauth/callback", CodeVerifier: "verifier",
		Scopes: []string{ScopeOpenID, ScopeEmail, ScopeMailSend},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(tokens.Scopes, []string{ScopeOpenID, ScopeEmail, ScopeMailSend}) {
		t.Fatalf("token scopes = %#v", tokens.Scopes)
	}
	if tokens.RefreshTokenExpiresAt == nil ||
		!tokens.RefreshTokenExpiresAt.After(time.Now().UTC().Add(90*time.Minute)) {
		t.Fatalf("refresh token expiry = %#v", tokens.RefreshTokenExpiresAt)
	}
}

type oauthRewriteTransport struct {
	target *url.URL
}

func (transport oauthRewriteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	urlCopy := *request.URL
	urlCopy.Scheme = transport.target.Scheme
	urlCopy.Host = transport.target.Host
	clone.URL = &urlCopy
	return http.DefaultTransport.RoundTrip(clone)
}

func mustOAuthURL(t *testing.T, value string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
