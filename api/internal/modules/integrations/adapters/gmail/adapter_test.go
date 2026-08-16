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
	"github.com/zgiai/zgi/api/internal/modules/tools"
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
	if !slices.Equal(account.SupportedCallers, []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent}) ||
		!slices.Equal(send.SupportedCallers, []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat}) {
		t.Fatalf("action callers: account=%#v send=%#v", account.SupportedCallers, send.SupportedCallers)
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

func TestCoreMailActionsHaveStrictScopesAndGovernance(t *testing.T) {
	actions := map[string]integrations.ActionDefinition{}
	for _, action := range Actions() {
		actions[action.ID] = action
	}
	if len(actions) != 6 {
		t.Fatalf("actions = %#v", actions)
	}
	for _, actionID := range []string{ActionSearchMail, ActionGetMail, ActionReplyMail, ActionCreateDraft} {
		action, ok := actions[actionID]
		if !ok || action.InputSchema["$schema"] != "https://json-schema.org/draft/2020-12/schema" ||
			action.InputSchema["additionalProperties"] != false || action.OutputSchema["additionalProperties"] != false {
			t.Fatalf("strict action %s = %#v", actionID, action)
		}
	}
	for _, actionID := range []string{ActionSearchMail, ActionGetMail} {
		action := actions[actionID]
		if action.DefaultPolicy == nil || !action.DefaultPolicy.Enabled ||
			action.DefaultPolicy.ApprovalPolicy != toolgovernance.ApprovalPolicyNeverAsk ||
			!action.Idempotent || action.Effect != toolgovernance.EffectRead ||
			!slices.Equal(action.RequiredScopes, []string{ScopeMailReadonly}) ||
			!slices.Equal(action.SupportedCallers, []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat, tools.ToolInvokeFromAgent}) {
			t.Fatalf("read action %s = %#v", actionID, action)
		}
	}
	reply := actions[ActionReplyMail]
	if reply.DefaultPolicy == nil || reply.DefaultPolicy.Enabled ||
		reply.DefaultPolicy.ApprovalPolicy != toolgovernance.ApprovalPolicyAlwaysAsk || reply.Idempotent ||
		reply.Effect != toolgovernance.EffectExternalSend ||
		!slices.Equal(reply.RequiredScopes, []string{ScopeMailReadonly, ScopeMailSend}) ||
		!slices.Equal(reply.SupportedCallers, []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat}) {
		t.Fatalf("reply action = %#v", reply)
	}
	draft := actions[ActionCreateDraft]
	if draft.DefaultPolicy == nil || draft.DefaultPolicy.Enabled ||
		draft.DefaultPolicy.ApprovalPolicy != toolgovernance.ApprovalPolicyAlwaysAsk || draft.Idempotent ||
		draft.Effect != toolgovernance.EffectCreate ||
		!slices.Equal(draft.RequiredScopes, []string{ScopeMailCompose}) ||
		!slices.Equal(draft.SupportedCallers, []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat}) {
		t.Fatalf("draft action = %#v", draft)
	}
	searchProperties, _ := actions[ActionSearchMail].InputSchema["properties"].(map[string]interface{})
	querySchema, _ := searchProperties["query"].(map[string]interface{})
	if querySchema["pattern"] != `\S` {
		t.Fatalf("query schema = %#v", querySchema)
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

func TestSearchMailReturnsBoundedSummariesAndPagination(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		writer.Header().Set("X-Google-Request-ID", "gmail-search")
		switch request.URL.Path {
		case "/gmail/v1/users/me/messages":
			if request.URL.Query().Get("q") != "from:alerts@example.com" ||
				request.URL.Query().Get("maxResults") != "2" || request.URL.Query().Get("pageToken") != "page-1" ||
				request.URL.Query().Get("includeSpamTrash") != "true" {
				t.Errorf("search query = %s", request.URL.RawQuery)
			}
			_, _ = io.WriteString(writer, `{"messages":[{"id":"message-1","threadId":"thread-1"},{"id":"message-2","threadId":"thread-2"}],"nextPageToken":"page-2","resultSizeEstimate":42}`)
		case "/gmail/v1/users/me/messages/message-1":
			if request.URL.Query().Get("format") != "metadata" || len(request.URL.Query()["metadataHeaders"]) == 0 {
				t.Errorf("metadata query = %s", request.URL.RawQuery)
			}
			_, _ = io.WriteString(writer, `{"id":"message-1","threadId":"thread-1","labelIds":["INBOX"],"snippet":"first snippet","payload":{"headers":[{"name":"Subject","value":"=?UTF-8?Q?Status_=E2=9C=93?="},{"name":"From","value":"Alerts <alerts@example.com>"},{"name":"To","value":"Owner <owner@example.com>"},{"name":"Date","value":"Wed, 1 Aug 2026 12:00:00 +0000"}]}}`)
		case "/gmail/v1/users/me/messages/message-2":
			_, _ = io.WriteString(writer, `{"id":"message-2","threadId":"thread-2","labelIds":["IMPORTANT"],"snippet":"second snippet","payload":{"headers":[{"name":"Subject","value":"Second"},{"name":"From","value":"alerts@example.com"},{"name":"To","value":"owner@example.com"},{"name":"Date","value":"Wed, 1 Aug 2026 13:00:00 +0000"}]}}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	adapter, err := newForBaseURLs(server.Client(), server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: ActionSearchMail, Connection: gmailTestConnection(),
		Input: map[string]interface{}{
			"query": "from:alerts@example.com", "max_results": 2, "page_token": "page-1", "include_spam_trash": true,
		},
	})
	if err != nil || result == nil || result.ResultCount != 2 || result.AttemptCount != 3 || calls.Load() != 3 {
		t.Fatalf("result = %#v, calls = %d, err = %v", result, calls.Load(), err)
	}
	messages, ok := result.Output["messages"].([]interface{})
	if !ok || len(messages) != 2 || result.Output["next_page_token"] != "page-2" || result.Output["result_size_estimate"] != 42 {
		t.Fatalf("output = %#v", result.Output)
	}
	first, _ := messages[0].(map[string]interface{})
	if first["subject"] != "Status ✓" || first["snippet"] != "first snippet" || first["thread_id"] != "thread-1" {
		t.Fatalf("first summary = %#v", first)
	}
}

func TestGetMailSafelyParsesMIMEAndTruncates(t *testing.T) {
	body := strings.Repeat("a", 1100)
	encodedBody := base64.RawURLEncoding.EncodeToString([]byte(body))
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/gmail/v1/users/me/messages/message-1" || request.URL.Query().Get("format") != "full" {
			http.NotFound(writer, request)
			return
		}
		_, _ = io.WriteString(writer, `{"id":"message-1","threadId":"thread-1","labelIds":["INBOX"],"snippet":"bounded","payload":{"mimeType":"multipart/alternative","headers":[{"name":"Subject","value":"Long body"},{"name":"From","value":"from@example.com"},{"name":"To","value":"to@example.com"}],"parts":[{"mimeType":"text/plain","headers":[{"name":"Content-Type","value":"text/plain; charset=UTF-8"}],"body":{"data":"`+encodedBody+`"}},{"mimeType":"text/html","body":{"data":"`+base64.RawURLEncoding.EncodeToString([]byte(`<script>secret()</script><p>fallback</p>`))+`"}}]}}`)
	}))
	defer server.Close()
	adapter, err := newForBaseURLs(server.Client(), server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: ActionGetMail, Connection: gmailTestConnection(),
		Input: map[string]interface{}{"message_id": "message-1", "max_body_characters": 1000},
	})
	if err != nil || result == nil {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	message, _ := result.Output["message"].(map[string]interface{})
	if len([]rune(message["body_text"].(string))) != 1000 || message["body_truncated"] != true || message["mime_type"] != "text/plain" {
		t.Fatalf("message = %#v", message)
	}
	htmlText, mimeType, truncated, err := gmailMessageText(gmailMessagePart{
		MimeType: "text/html", Body: gmailMessageBody{Data: base64.RawURLEncoding.EncodeToString([]byte(`<style>.x{}</style><p>Hello <b>world</b></p>`))},
	}, 1000)
	if err != nil || htmlText != "Hello world" || mimeType != "text/html" || truncated {
		t.Fatalf("html body = %q, mime = %q, truncated = %v, err = %v", htmlText, mimeType, truncated, err)
	}
}

func TestReplyMailPreservesThreadHeadersAndDoesNotRetry(t *testing.T) {
	var getCalls atomic.Int32
	var sendCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/gmail/v1/users/me/messages/message-1":
			getCalls.Add(1)
			_, _ = io.WriteString(writer, `{"id":"message-1","threadId":"thread-1","payload":{"headers":[{"name":"Reply-To","value":"Support <reply@example.com>"},{"name":"From","value":"sender@example.com"},{"name":"Subject","value":"Ticket"},{"name":"Message-ID","value":"<source@example.com>"},{"name":"References","value":"<prior@example.com>"}]}}`)
		case "/gmail/v1/users/me/messages/send":
			sendCalls.Add(1)
			var payload struct {
				Raw      string `json:"raw"`
				ThreadID string `json:"threadId"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Errorf("decode reply: %v", err)
			}
			raw, err := base64.RawURLEncoding.DecodeString(payload.Raw)
			if err != nil || payload.ThreadID != "thread-1" ||
				!strings.Contains(string(raw), "<reply@example.com>") ||
				!strings.Contains(string(raw), "In-Reply-To: <source@example.com>") ||
				!strings.Contains(string(raw), "References: <prior@example.com> <source@example.com>") ||
				!strings.Contains(string(raw), "Subject: Re: Ticket") {
				t.Errorf("reply payload = %#v raw=%q err=%v", payload, raw, err)
			}
			_, _ = io.WriteString(writer, `{"id":"reply-1","threadId":"thread-1","labelIds":["SENT"]}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	adapter, err := newForBaseURLs(server.Client(), server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: ActionReplyMail, Connection: gmailTestConnection(),
		Input: map[string]interface{}{"message_id": "message-1", "body_text": "Thanks"},
	})
	if err != nil || result == nil || getCalls.Load() != 1 || sendCalls.Load() != 1 || result.AttemptCount != 2 {
		t.Fatalf("result=%#v get=%d send=%d err=%v", result, getCalls.Load(), sendCalls.Load(), err)
	}
}

func TestReplyAndDraftWritesNeverRetry(t *testing.T) {
	for _, actionID := range []string{ActionReplyMail, ActionCreateDraft} {
		t.Run(actionID, func(t *testing.T) {
			var writeCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if actionID == ActionReplyMail && request.Method == http.MethodGet {
					_, _ = io.WriteString(writer, `{"id":"message-1","threadId":"thread-1","payload":{"headers":[{"name":"From","value":"sender@example.com"},{"name":"Subject","value":"Subject"},{"name":"Message-ID","value":"<source@example.com>"}]}}`)
					return
				}
				writeCalls.Add(1)
				writer.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(writer, `{"error":{"message":"temporary"}}`)
			}))
			defer server.Close()
			adapter, err := newForBaseURLs(server.Client(), server.URL, server.URL)
			if err != nil {
				t.Fatal(err)
			}
			input := map[string]interface{}{"message_id": "message-1", "body_text": "Reply"}
			if actionID == ActionCreateDraft {
				input = map[string]interface{}{"to": "to@example.com", "subject": "Subject", "body_text": "Draft"}
			}
			_, err = adapter.Execute(context.Background(), integrations.ActionRequest{
				ActionID: actionID, Connection: gmailTestConnection(), Input: input,
			})
			if err == nil || writeCalls.Load() != 1 {
				t.Fatalf("err=%v write calls=%d", err, writeCalls.Load())
			}
		})
	}
}

func TestCreateDraftUsesGmailDraftEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/gmail/v1/users/me/drafts" || request.Method != http.MethodPost {
			http.NotFound(writer, request)
			return
		}
		var payload struct {
			Message struct {
				Raw string `json:"raw"`
			} `json:"message"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode draft: %v", err)
		}
		raw, err := base64.RawURLEncoding.DecodeString(payload.Message.Raw)
		if err != nil || !strings.Contains(string(raw), "Subject: Draft subject") || !strings.Contains(string(raw), "to@example.com") {
			t.Errorf("draft raw=%q err=%v", raw, err)
		}
		_, _ = io.WriteString(writer, `{"id":"draft-1","message":{"id":"message-1","threadId":"thread-1","labelIds":["DRAFT"]}}`)
	}))
	defer server.Close()
	adapter, err := newForBaseURLs(server.Client(), server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: ActionCreateDraft, Connection: gmailTestConnection(),
		Input: map[string]interface{}{"to": "to@example.com", "subject": "Draft subject", "body_text": "Body"},
	})
	if err != nil || result == nil || result.ResultCount != 1 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestCoreMailActionsRejectInvalidInputBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	adapter, err := newForBaseURLs(server.Client(), server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		actionID string
		input    map[string]interface{}
	}{
		{ActionSearchMail, map[string]interface{}{"query": "   "}},
		{ActionSearchMail, map[string]interface{}{"query": "in:inbox", "max_results": 21}},
		{ActionGetMail, map[string]interface{}{"message_id": "   "}},
		{ActionReplyMail, map[string]interface{}{"message_id": "message-1", "body_text": "\t"}},
		{ActionCreateDraft, map[string]interface{}{"to": "to@example.com", "subject": " ", "body_text": "body"}},
	}
	for _, test := range tests {
		_, err := adapter.Execute(context.Background(), integrations.ActionRequest{
			ActionID: test.actionID, Connection: gmailTestConnection(), Input: test.input,
		})
		if integrations.ErrorCode(err) != integrations.ErrorCodeInvalidInput {
			t.Fatalf("%s err=%v", test.actionID, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid input reached Gmail %d time(s)", calls.Load())
	}
}

func gmailTestConnection() *integrations.ResolvedConnection {
	return &integrations.ResolvedConnection{
		IntegrationID: IntegrationID, DriverID: DriverID,
		Credentials: map[string]string{"access_token": "access-secret"},
		GrantedScopes: []string{
			ScopeOpenID, ScopeEmail, ScopeProfile, ScopeMailReadonly, ScopeMailSend, ScopeMailCompose,
		},
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
