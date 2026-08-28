package dingtalk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestProviderDefinitionRegistersFailClosedMessageAction(t *testing.T) {
	adapter, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	registry := integrations.NewRegistry()
	definition := ProviderDefinition()
	if err := registry.Register(integrations.Registration{Definition: definition, Adapter: adapter, ConnectionTester: adapter, CredentialValidator: adapter, HealthProbe: adapter}); err != nil {
		t.Fatal(err)
	}
	if len(definition.Actions) != 12 {
		t.Fatalf("actions = %d, want 12", len(definition.Actions))
	}
	for _, action := range definition.Actions {
		if action.ID == ActionMessageSendUser {
			if action.DefaultPolicy == nil || action.DefaultPolicy.Enabled || action.SuccessDeduplication == nil || len(action.PreparationHints) != 1 {
				t.Fatalf("message action is not fail-closed: %#v", action)
			}
			return
		}
	}
	t.Fatal("message action not found")
}

func TestRegistrySearchMatchesNaturalChineseDingTalkActionQueries(t *testing.T) {
	adapter, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	registry := integrations.NewRegistry()
	if err := registry.Register(integrations.Registration{
		Definition: ProviderDefinition(), Adapter: adapter, ConnectionTester: adapter,
		CredentialValidator: adapter, HealthProbe: adapter,
	}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		query  string
		action string
	}{
		{query: "列出角色", action: ActionRoleList},
		{query: "查询通知状态", action: ActionMessageStatusGet},
		{query: "搜索成员", action: ActionContactSearch},
		{query: "列出部门", action: ActionDepartmentList},
		{query: "list roles", action: ActionRoleList},
	}
	for _, test := range tests {
		t.Run(test.query, func(t *testing.T) {
			found := registry.SearchActionSummaries(integrations.ActionSearchRequest{
				Query: test.query, IntegrationID: IntegrationID, Caller: tools.ToolInvokeFromAIChat, Limit: 5,
			})
			if len(found) == 0 || found[0].ID != test.action {
				t.Fatalf("SearchActionSummaries(%q) = %#v, want first action %q", test.query, found, test.action)
			}
		})
	}
	if found := registry.SearchActionSummaries(integrations.ActionSearchRequest{
		Query: "武汉天气", IntegrationID: IntegrationID, Caller: tools.ToolInvokeFromAIChat, Limit: 5,
	}); len(found) != 0 {
		t.Fatalf("unrelated query returned DingTalk actions: %#v", found)
	}
}

func TestAdapterConnectionContactSendAndDeliveryFlow(t *testing.T) {
	var tokenCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1.0/oauth2/accessToken":
			tokenCalls.Add(1)
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["appKey"] != "app-key" || body["appSecret"] != "app-secret" {
				t.Errorf("token body = %#v", body)
			}
			_, _ = w.Write([]byte(`{"accessToken":"token-1","expireIn":7200}`))
		case "/v1.0/contact/users/search":
			if r.Header.Get("x-acs-dingtalk-access-token") != "token-1" {
				t.Errorf("missing API token header")
			}
			_, _ = w.Write([]byte(`{"result":[{"userId":"user-1","name":"张三","title":"工程师"}]}`))
		case "/topapi/v2/department/listsub":
			assertLegacyToken(t, r)
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","result":[{"dept_id":2,"name":"研发部","parent_id":1}]}`))
		case "/topapi/v2/user/get":
			assertLegacyToken(t, r)
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","result":{"userid":"user-1","name":"张三","title":"工程师","dept_id_list":[2],"active":true}}`))
		case "/topapi/message/corpconversation/asyncsend_v2":
			assertLegacyToken(t, r)
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["userid_list"] != "user-1" || int(body["agent_id"].(float64)) != 123 {
				t.Errorf("send body = %#v", body)
			}
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","task_id":9988,"request_id":"request-1"}`))
		case "/topapi/message/corpconversation/getsendresult":
			assertLegacyToken(t, r)
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","result":{"send_result":{"read_user_id_list":[],"unread_user_id_list":["user-1"]}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	adapter, err := newForBaseURLs(server.Client(), server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	connection := testConnection("connection-1")
	profile, err := adapter.ValidateConnection(context.Background(), connection)
	if err != nil {
		t.Fatal(err)
	}
	if profile.DisplayName == "" || len(profile.GrantedScopes) != 0 ||
		profile.ScopeEvidence != integrations.AuthScopeEvidenceConnectorDeclared ||
		len(profile.VerifiedActionIDs) != 1 || profile.VerifiedActionIDs[0] != ActionDepartmentList {
		t.Fatalf("profile = %#v", profile)
	}
	search, err := adapter.Execute(context.Background(), integrations.ActionRequest{IntegrationID: IntegrationID, ActionID: ActionContactSearch, Connection: connection, Input: map[string]interface{}{"query": "张三", "max_results": 10}})
	if err != nil {
		t.Fatal(err)
	}
	assertDingTalkOutputContract(t, ActionContactSearch, search.Output)
	members := search.Output["members"].([]map[string]interface{})
	if hasMore, _ := search.Output["has_more"].(bool); hasMore {
		t.Fatalf("under-filled raw contact page reported has_more: %#v", search.Output)
	}
	recipientRef := members[0]["recipient_ref"].(string)
	if strings.Contains(recipientRef, "user-1") {
		t.Fatal("recipient reference exposes raw user ID")
	}
	user, err := adapter.Execute(context.Background(), integrations.ActionRequest{IntegrationID: IntegrationID, ActionID: ActionUserGet, Connection: connection, Input: map[string]interface{}{"recipient_ref": recipientRef}})
	if err != nil {
		t.Fatal(err)
	}
	assertDingTalkOutputContract(t, ActionUserGet, user.Output)
	sent, err := adapter.Execute(context.Background(), integrations.ActionRequest{IntegrationID: IntegrationID, ActionID: ActionMessageSendUser, Connection: connection, Input: map[string]interface{}{"recipient_ref": recipientRef, "content": "请查看通知"}})
	if err != nil {
		t.Fatal(err)
	}
	notification := sent.Output["notification"].(map[string]interface{})
	messageRef := notification["message_ref"].(string)
	if notification["delivery_status"] != "pending" {
		t.Fatalf("notification = %#v", notification)
	}
	status, err := adapter.Execute(context.Background(), integrations.ActionRequest{IntegrationID: IntegrationID, ActionID: ActionMessageStatusGet, Connection: connection, Input: map[string]interface{}{"message_ref": messageRef}})
	if err != nil {
		t.Fatal(err)
	}
	if status.Output["notification"].(map[string]interface{})["delivery_status"] != "delivered_unread" {
		t.Fatalf("status = %#v", status.Output)
	}
	if tokenCalls.Load() != 1 {
		t.Fatalf("token calls = %d, want 1", tokenCalls.Load())
	}
}

func TestContactSearchSupportsOfficialIDListResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1.0/oauth2/accessToken":
			_, _ = w.Write([]byte(`{"accessToken":"token-1","expireIn":7200}`))
		case "/v1.0/contact/users/search":
			_, _ = w.Write([]byte(`{"list":["user-1"]}`))
		case "/topapi/v2/user/get":
			assertLegacyToken(t, r)
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","result":{"userid":"user-1","name":"张三","title":"工程师","dept_id_list":[2],"active":true}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter, err := newForBaseURLs(server.Client(), server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		IntegrationID: IntegrationID,
		ActionID:      ActionContactSearch,
		Connection:    testConnection("connection-1"),
		Input:         map[string]interface{}{"query": "张三", "max_results": 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	members := result.Output["members"].([]map[string]interface{})
	if len(members) != 1 || members[0]["name"] != "张三" || members[0]["recipient_ref"] == "" {
		t.Fatalf("members = %#v", members)
	}
}

func TestContactSearchPreservesRawPageCompletenessAfterFiltering(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1.0/oauth2/accessToken":
			_, _ = w.Write([]byte(`{"accessToken":"token-1","expireIn":7200}`))
		case "/v1.0/contact/users/search":
			_, _ = w.Write([]byte(`{"result":[{"userId":"user-1","name":"张三"},{"userId":"bad|user","name":"invalid"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter, err := newForBaseURLs(server.Client(), server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		IntegrationID: IntegrationID,
		ActionID:      ActionContactSearch,
		Connection:    testConnection("connection-1"),
		Input:         map[string]interface{}{"query": "张三", "max_results": 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(result.Output["members"].([]map[string]interface{})); got != 1 {
		t.Fatalf("filtered member count = %d, output=%#v", got, result.Output)
	}
	if hasMore, _ := result.Output["has_more"].(bool); !hasMore {
		t.Fatalf("full raw contact page lost pagination evidence: %#v", result.Output)
	}
	assertDingTalkOutputContract(t, ActionContactSearch, result.Output)
}

func TestSearchResponseHasMorePrefersNativePaginationMetadata(t *testing.T) {
	tests := []struct {
		name     string
		response map[string]json.RawMessage
		rawCount int
		limit    int
		want     bool
	}{
		{
			name: "top-level total proves another page",
			response: map[string]json.RawMessage{
				"list":       json.RawMessage(`[1]`),
				"totalCount": json.RawMessage(`2`),
			},
			rawCount: 1, limit: 10, want: true,
		},
		{
			name: "nested has-more proves completion despite full page",
			response: map[string]json.RawMessage{
				"result": json.RawMessage(`{"list":[1,2],"hasMore":false}`),
			},
			rawCount: 2, limit: 2, want: false,
		},
		{
			name: "malformed top-level has-more fails closed",
			response: map[string]json.RawMessage{
				"list":     json.RawMessage(`[1]`),
				"has_more": json.RawMessage(`"true"`),
			},
			rawCount: 1, limit: 10, want: true,
		},
		{
			name: "null top-level has-more fails closed",
			response: map[string]json.RawMessage{
				"list":    json.RawMessage(`[1]`),
				"hasMore": json.RawMessage(`null`),
			},
			rawCount: 1, limit: 10, want: true,
		},
		{
			name: "negative nested total fails closed",
			response: map[string]json.RawMessage{
				"result": json.RawMessage(`{"list":[1],"totalCount":-1}`),
			},
			rawCount: 1, limit: 10, want: true,
		},
		{
			name: "malformed nested total fails closed",
			response: map[string]json.RawMessage{
				"result": json.RawMessage(`{"list":[1],"total_count":"one"}`),
			},
			rawCount: 1, limit: 10, want: true,
		},
		{
			name: "null nested total fails closed",
			response: map[string]json.RawMessage{
				"result": json.RawMessage(`{"list":[1],"totalCount":null}`),
			},
			rawCount: 1, limit: 10, want: true,
		},
		{
			name: "false has-more conflicts with larger total",
			response: map[string]json.RawMessage{
				"hasMore": json.RawMessage(`false`),
				"result":  json.RawMessage(`{"list":[1],"totalCount":2}`),
			},
			rawCount: 1, limit: 10, want: true,
		},
		{
			name: "top-level and nested has-more conflict",
			response: map[string]json.RawMessage{
				"has_more": json.RawMessage(`false`),
				"result":   json.RawMessage(`{"list":[1],"hasMore":true}`),
			},
			rawCount: 1, limit: 10, want: true,
		},
		{
			name: "total smaller than observed page fails closed",
			response: map[string]json.RawMessage{
				"list":  json.RawMessage(`[1,2]`),
				"total": json.RawMessage(`1`),
			},
			rawCount: 2, limit: 10, want: true,
		},
		{
			name: "full page without metadata is conservative",
			response: map[string]json.RawMessage{
				"list": json.RawMessage(`[1,2]`),
			},
			rawCount: 2, limit: 2, want: true,
		},
		{
			name: "under-filled page without metadata is complete",
			response: map[string]json.RawMessage{
				"list": json.RawMessage(`[1]`),
			},
			rawCount: 1, limit: 2, want: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := searchResponseHasMore(test.response, test.rawCount, test.limit); got != test.want {
				t.Fatalf("searchResponseHasMore() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestListDepartmentsExplainsSuccessfulEmptyChildList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1.0/oauth2/accessToken":
			_, _ = w.Write([]byte(`{"accessToken":"token-1","expireIn":7200}`))
		case "/topapi/v2/department/listsub":
			assertLegacyToken(t, r)
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","result":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter, err := newForBaseURLs(server.Client(), server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		IntegrationID: IntegrationID,
		ActionID:      ActionDepartmentList,
		Connection:    testConnection("connection-1"),
		Input:         map[string]interface{}{"department_id": 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ResultCount != 0 || result.Output["empty_reason"] != "no_child_departments" {
		t.Fatalf("empty department result = %#v", result.Output)
	}
	assertDingTalkOutputContract(t, ActionDepartmentList, result.Output)
}

func TestReferencesAreConnectionBoundAndRawUserIDsAreRejected(t *testing.T) {
	recipientRef := encodeRecipientRef("connection-1", "user-1")
	if _, err := decodeRecipientRef(recipientRef, "connection-2"); err == nil {
		t.Fatal("expected cross-connection recipient rejection")
	}
	if _, err := decodeRecipientRef("user-1", "connection-1"); err == nil {
		t.Fatal("expected raw user ID rejection")
	}
	messageRef := encodeMessageRef("connection-1", 7, "user-1")
	if _, err := decodeMessageRef(messageRef, "connection-2"); err == nil {
		t.Fatal("expected cross-connection message rejection")
	}
}

func TestValidateCredentialsRejectsInvalidAgentID(t *testing.T) {
	adapter, _ := New(nil)
	err := adapter.ValidateCredentials(context.Background(), integrations.CredentialValidationRequest{IntegrationID: IntegrationID, DriverID: DriverID, AuthMethodID: AuthMethodID, Credentials: map[string]string{"app_key": "key", "app_secret": "secret", "agent_id": "not-a-number"}})
	if integrations.ErrorCode(err) != integrations.ErrorCodeConnectionInvalid {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateConnectionPreservesSafeDingTalkDiagnostics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":"Forbidden.AccessDenied","message":"provider detail","requestid":"req-ding-1"}`))
	}))
	defer server.Close()
	adapter, err := newForBaseURLs(server.Client(), server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.ValidateConnection(context.Background(), testConnection("connection-1"))
	if integrations.ErrorCode(err) != integrations.ErrorCodeAccessDenied {
		t.Fatalf("error = %v", err)
	}
	diagnostics := integrations.ProviderDiagnosticsFromError(err)
	if diagnostics.ErrorCode != "Forbidden.AccessDenied" || diagnostics.RequestID != "req-ding-1" || diagnostics.HTTPStatus != http.StatusForbidden {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	if strings.Contains(err.Error(), "provider detail") {
		t.Fatalf("provider message must not leak: %v", err)
	}
}

func TestContactSearchMapsExplicitAccessTokenPermissionDenialToInsufficientScope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1.0/oauth2/accessToken":
			_, _ = w.Write([]byte(`{"accessToken":"token-1","expireIn":7200}`))
		case "/v1.0/contact/users/search":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"code":"Forbidden.AccessDenied.AccessTokenPermissionDenied","message":"permission denied","requestid":"req-scope-1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	adapter, err := newForBaseURLs(server.Client(), server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Execute(context.Background(), integrations.ActionRequest{
		IntegrationID: IntegrationID,
		ActionID:      ActionContactSearch,
		Connection:    testConnection("connection-1"),
		Input:         map[string]interface{}{"query": "杨阳", "max_results": 10},
	})
	if integrations.ErrorCode(err) != integrations.ErrorCodeInsufficientScope {
		t.Fatalf("error = %v", err)
	}
	diagnostics := integrations.ProviderDiagnosticsFromError(err)
	if diagnostics.ErrorCode != "Forbidden.AccessDenied.AccessTokenPermissionDenied" ||
		diagnostics.RequestID != "req-scope-1" || diagnostics.HTTPStatus != http.StatusForbidden {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
}

func TestDingTalkGuideCoversPermissionsPublishingAndIPAllowlist(t *testing.T) {
	guide := ProviderDefinition().AuthMethods[0].SetupGuide
	if guide == nil || !guide.ExpandedByDefault {
		t.Fatal("DingTalk setup guide must be expanded")
	}
	ids := map[string]bool{}
	for _, step := range guide.Steps {
		ids[step.ID] = true
	}
	for _, id := range []string{"copy_credentials", "grant_contact_permissions", "configure_visibility", "publish_application", "configure_ip_whitelist", "save_and_verify"} {
		if !ids[id] {
			t.Fatalf("setup guide missing %q", id)
		}
	}
}

func assertLegacyToken(t *testing.T, r *http.Request) {
	t.Helper()
	if r.URL.Query().Get("access_token") != "token-1" {
		t.Errorf("missing legacy access token")
	}
}
func testConnection(id string) *integrations.ResolvedConnection {
	return &integrations.ResolvedConnection{ID: id, IntegrationID: IntegrationID, DriverID: DriverID, AuthMethodID: AuthMethodID, Credentials: map[string]string{"app_key": "app-key", "app_secret": "app-secret", "agent_id": "123"}}
}

func assertDingTalkOutputContract(t *testing.T, actionID string, output map[string]interface{}) {
	t.Helper()
	var schema map[string]interface{}
	for _, action := range ProviderDefinition().Actions {
		if action.ID == actionID {
			schema = action.OutputSchema
			break
		}
	}
	if schema == nil {
		t.Fatalf("action %q was not found", actionID)
	}
	normalized, err := tools.NormalizeJSONValue(output)
	if err != nil {
		t.Fatalf("normalize %s output: %v", actionID, err)
	}
	if err := tools.ValidateJSONSchemaValue(schema, normalized); err != nil {
		t.Fatalf("%s output contract: %v; output=%#v", actionID, err, normalized)
	}
}
