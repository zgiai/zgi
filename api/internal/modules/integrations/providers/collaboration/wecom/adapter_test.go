package wecom

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/integrations/core"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestProviderDefinitionRegistersWithFailClosedSend(t *testing.T) {
	adapter, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	registry := integrations.NewRegistry()
	if err := registry.Register(integrations.Registration{Definition: ProviderDefinition(), Adapter: adapter, ConnectionTester: adapter, HealthProbe: adapter}); err != nil {
		t.Fatal(err)
	}
	definition, ok := registry.ProviderDefinition(IntegrationID)
	if !ok {
		t.Fatal("WeCom provider definition is missing")
	}
	coreActions := map[string]bool{
		ActionAppGet: false, ActionDepartmentList: false, ActionContactSearch: false,
		ActionUserGet: false, ActionMessageSendUser: false,
	}
	for _, action := range definition.Actions {
		if _, core := coreActions[action.ID]; core {
			coreActions[action.ID] = true
		}
		if action.ID == ActionMessageSendUser && (action.DefaultPolicy.Enabled || action.SuccessDeduplication == nil) {
			t.Fatal("send action must be disabled by default and success-deduplicated")
		}
		if action.ID == ActionContactSearch {
			properties, _ := action.InputSchema["properties"].(map[string]interface{})
			department, _ := properties["department_id"].(map[string]interface{})
			if _, hasRootDefault := department["default"]; hasRootDefault {
				t.Fatal("contact search must not default to the root department")
			}
		}
	}
	for actionID, found := range coreActions {
		if !found {
			t.Fatalf("core WeCom action %q is missing", actionID)
		}
	}
}

func TestProviderDepartmentNameDoesNotExposeNumericProviderMask(t *testing.T) {
	if got := providerDepartmentName("7", 7); got != "" {
		t.Fatalf("numeric provider department name=%q, want empty display name", got)
	}
	if got := providerDepartmentName("Engineering", 7); got != "Engineering" {
		t.Fatalf("real provider department name=%q", got)
	}
}

func TestAdapterValidatesSearchesAndSendsWithTokenCache(t *testing.T) {
	var tokenCalls atomic.Int32
	var providerTokenCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/cgi-bin/service/get_provider_token":
			providerTokenCalls.Add(1)
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "provider_access_token": "provider-token", "expires_in": 7200})
		case "/cgi-bin/service/get_corp_token":
			tokenCalls.Add(1)
			json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "access_token": "token", "expires_in": 7200})
		case "/cgi-bin/agent/get":
			json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "agentid": 1000002, "name": "ZGI Assistant", "square_logo_url": "https://example.com/logo.png"})
		case "/cgi-bin/service/v2/get_auth_info":
			json.NewEncoder(writer).Encode(map[string]interface{}{
				"errcode": 0, "auth_corp_info": map[string]interface{}{"corpid": "corp"},
				"auth_info": map[string]interface{}{"agent": []map[string]interface{}{{
					"agentid": 1000002, "name": "ZGI Assistant", "auth_mode": wecomAuthorizationModeAdministrator,
					"privilege": map[string]interface{}{"level": 1, "allow_party": []int{7}},
				}}},
			})
		case "/cgi-bin/department/list":
			if request.URL.Query().Get("id") != "7" {
				t.Errorf("unexpected authorized department root query: %s", request.URL.RawQuery)
			}
			json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "department": []map[string]interface{}{{"id": 7, "name": "Engineering", "parentid": 1}}})
		case "/cgi-bin/user/simplelist":
			if request.URL.Query().Get("department_id") != "7" || request.URL.Query().Has("fetch_child") {
				t.Errorf("unexpected contact search query: %s", request.URL.RawQuery)
			}
			json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "userlist": []map[string]interface{}{{"userid": "yangzhihang", "name": "yangzhihang", "department": []int{7}}}})
		case "/cgi-bin/user/get":
			json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "userid": "yangzhihang", "name": "yangzhihang", "department": []int{7}, "position": "Engineer", "status": 1})
		case "/cgi-bin/service/contact/search":
			if request.URL.Query().Get("provider_access_token") != "provider-token" {
				t.Errorf("provider token query=%s", request.URL.RawQuery)
			}
			var payload map[string]interface{}
			_ = json.NewDecoder(request.Body).Decode(&payload)
			if payload["auth_corpid"] != "corp" || payload["query_word"] != "yangzhihang" || payload["query_type"] != float64(1) || payload["query_range"] != float64(0) || payload["agentid"] != float64(1000002) {
				t.Errorf("member search payload=%#v", payload)
			}
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{
				"errcode": 0,
				"query_result": map[string]interface{}{"user": map[string]interface{}{
					"userid": []string{"yangzhihang"}, "open_userid": []string{"open-yangzhihang"},
				}},
				"is_last": true,
			})
		case "/cgi-bin/message/send":
			var payload map[string]interface{}
			json.NewDecoder(request.Body).Decode(&payload)
			if payload["touser"] != "yangzhihang" {
				t.Errorf("touser=%v", payload["touser"])
			}
			json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "msgid": "message-1"})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	configureSuiteTestAdapter(adapter)
	connection := &integrations.ResolvedConnection{ID: "connection", IntegrationID: IntegrationID, DriverID: DriverID, CredentialVersion: 1, Revision: 1, Credentials: map[string]string{"auth_corpid": "corp", "agent_id": "1000002", "permanent_code": "permanent"}}
	profile, err := adapter.ValidateConnection(context.Background(), connection)
	if err != nil {
		t.Fatal(err)
	}
	if profile.DisplayName != "ZGI Assistant" {
		t.Fatalf("profile=%#v", profile)
	}
	if profile.AccountID == "corp:1000002" || strings.Contains(profile.AccountID, "corp") {
		t.Fatalf("profile account ID must be pseudonymous: %#v", profile)
	}
	search, err := adapter.Execute(context.Background(), integrations.ActionRequest{IntegrationID: IntegrationID, ActionID: ActionContactSearch, Connection: connection, Input: map[string]interface{}{"query": "yangzhihang"}})
	if err != nil {
		t.Fatal(err)
	}
	assertWeComOutputContract(t, ActionContactSearch, search.Output)
	members := search.Output["members"].([]map[string]interface{})
	ref := members[0]["recipient_ref"].(string)
	user, err := adapter.Execute(context.Background(), integrations.ActionRequest{IntegrationID: IntegrationID, ActionID: ActionUserGet, Connection: connection, Input: map[string]interface{}{"recipient_ref": ref}})
	if err != nil {
		t.Fatal(err)
	}
	assertWeComOutputContract(t, ActionUserGet, user.Output)
	sent, err := adapter.Execute(context.Background(), integrations.ActionRequest{IntegrationID: IntegrationID, ActionID: ActionMessageSendUser, Connection: connection, Input: map[string]interface{}{"recipient_ref": ref, "content": "hello"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(toJSON(sent.Output), "message-1") {
		t.Fatalf("output=%#v", sent.Output)
	}
	if tokenCalls.Load() != 1 {
		t.Fatalf("token calls=%d,want 1", tokenCalls.Load())
	}
	if providerTokenCalls.Load() != 1 {
		t.Fatalf("provider token calls=%d,want 1", providerTokenCalls.Load())
	}
}

func TestContactSearchUsesAuthorizedUsersAndExpandsAuthorizedPartyRoots(t *testing.T) {
	t.Skip("legacy local contact traversal was replaced by the official provider contact search contract")
	var departmentListCalls atomic.Int32
	var simpleListCalls atomic.Int32
	var tagCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/cgi-bin/service/v2/get_auth_info":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{
				"errcode": 0, "auth_corp_info": map[string]interface{}{"corpid": "corp"},
				"auth_info": map[string]interface{}{"agent": []map[string]interface{}{{
					"agentid": 1000002, "auth_mode": wecomAuthorizationModeAdministrator,
					"privilege": map[string]interface{}{
						"level": 1, "allow_user": []string{"direct-user"}, "extra_party": []int{12}, "extra_tag": []int{23},
					},
				}}},
			})
		case "/cgi-bin/service/get_corp_token":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "access_token": "token", "expires_in": 7200})
		case "/cgi-bin/user/get":
			if request.URL.Query().Get("userid") != "direct-user" {
				t.Errorf("unexpected explicit userid: %s", request.URL.RawQuery)
			}
			// A normal third-party app may receive the account identifier in the
			// name field. The search contract must still support a UserID query.
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "userid": "direct-user", "name": "direct-user", "department": []int{12}})
		case "/cgi-bin/department/list":
			departmentListCalls.Add(1)
			if request.URL.Query().Get("id") != "12" {
				t.Errorf("unexpected authorized department root query: %s", request.URL.RawQuery)
			}
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "department": []map[string]interface{}{
				{"id": 12, "name": "Root", "parentid": 1},
				{"id": 13, "name": "Child", "parentid": 12},
			}})
		case "/cgi-bin/user/simplelist":
			simpleListCalls.Add(1)
			departmentID := request.URL.Query().Get("department_id")
			if (departmentID != "12" && departmentID != "13") || request.URL.Query().Has("fetch_child") {
				t.Errorf("unexpected department query: %s", request.URL.RawQuery)
			}
			users := []map[string]interface{}{}
			if departmentID == "13" {
				users = append(users, map[string]interface{}{"userid": "party-user", "name": "party-user", "department": []int{13}})
			}
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "userlist": users})
		case "/cgi-bin/tag/get":
			tagCalls.Add(1)
			if request.URL.Query().Get("tagid") != "23" {
				t.Errorf("unexpected tag query: %s", request.URL.RawQuery)
			}
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "userlist": []map[string]interface{}{{"userid": "tag-user", "name": "tag-user"}}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	configureSuiteTestAdapter(adapter)
	connection := &integrations.ResolvedConnection{
		ID: "connection", IntegrationID: IntegrationID, DriverID: DriverID, CredentialVersion: 1, Revision: 1,
		Credentials: map[string]string{"auth_corpid": "corp", "agent_id": "1000002", "permanent_code": "permanent"},
	}
	search := func(query string) *integrations.ActionResult {
		t.Helper()
		result, executeErr := adapter.Execute(context.Background(), integrations.ActionRequest{
			IntegrationID: IntegrationID, ActionID: ActionContactSearch, Connection: connection,
			Input: map[string]interface{}{"query": query, "max_results": 1},
		})
		if executeErr != nil {
			t.Fatal(executeErr)
		}
		assertWeComOutputContract(t, ActionContactSearch, result.Output)
		if result.ResultCount != 1 {
			t.Fatalf("query %q result=%#v", query, result.Output)
		}
		members := result.Output["members"].([]map[string]interface{})
		if members[0]["name"] != "" {
			t.Fatalf("query %q exposed a masked provider name: %#v", query, members[0])
		}
		return result
	}
	direct := search("direct-user")
	directMembers := direct.Output["members"].([]map[string]interface{})
	if directMembers[0]["name"] != "" {
		t.Fatalf("masked provider name was exposed as a display name: %#v", directMembers[0])
	}
	resolved, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		IntegrationID: IntegrationID, ActionID: ActionUserGet, Connection: connection,
		Input: map[string]interface{}{"recipient_ref": directMembers[0]["recipient_ref"]},
	})
	if err != nil {
		t.Fatal(err)
	}
	resolvedMember := resolved.Output["member"].(map[string]interface{})
	if resolvedMember["name"] != "" {
		t.Fatalf("masked provider name was exposed by user.get: %#v", resolvedMember)
	}
	if departmentListCalls.Load() != 0 || simpleListCalls.Load() != 0 || tagCalls.Load() != 0 {
		t.Fatalf("explicitly authorized member search traversed unrelated subjects: departments=%d simple=%d tag=%d", departmentListCalls.Load(), simpleListCalls.Load(), tagCalls.Load())
	}
	search("party-user")
	if departmentListCalls.Load() != 1 || simpleListCalls.Load() != 2 || tagCalls.Load() != 0 {
		t.Fatalf("party traversal calls: departments=%d simple=%d tag=%d", departmentListCalls.Load(), simpleListCalls.Load(), tagCalls.Load())
	}
	search("tag-user")
	if departmentListCalls.Load() != 2 || simpleListCalls.Load() != 4 || tagCalls.Load() != 1 {
		t.Fatalf("tag traversal calls: departments=%d simple=%d tag=%d", departmentListCalls.Load(), simpleListCalls.Load(), tagCalls.Load())
	}
}

func TestContactSearchFailsBeforeUserTraversalWhenAuthorizedDepartmentRangeIsTooBroad(t *testing.T) {
	t.Skip("legacy traversal call-budget behavior no longer applies to the single official search request")
	var simpleListCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/cgi-bin/service/v2/get_auth_info":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{
				"errcode": 0, "auth_corp_info": map[string]interface{}{"corpid": "corp"},
				"auth_info": map[string]interface{}{"agent": []map[string]interface{}{{
					"agentid": 1000002, "auth_mode": wecomAuthorizationModeAdministrator,
					"privilege": map[string]interface{}{"level": 1, "allow_party": []int{12}},
				}}},
			})
		case "/cgi-bin/department/list":
			departments := make([]map[string]interface{}, 0, maxContactSearchProviderCalls)
			for index := 0; index < maxContactSearchProviderCalls; index++ {
				departments = append(departments, map[string]interface{}{"id": 12 + index, "parentid": 12})
			}
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "department": departments})
		case "/cgi-bin/service/get_corp_token":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "access_token": "token", "expires_in": 7200})
		case "/cgi-bin/user/simplelist":
			simpleListCalls.Add(1)
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "userlist": []interface{}{}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	configureSuiteTestAdapter(adapter)
	connection := &integrations.ResolvedConnection{ID: "broad", IntegrationID: IntegrationID, DriverID: DriverID, Credentials: map[string]string{"auth_corpid": "corp", "agent_id": "1000002", "permanent_code": "permanent"}}
	_, err = adapter.Execute(context.Background(), integrations.ActionRequest{IntegrationID: IntegrationID, ActionID: ActionContactSearch, Connection: connection, Input: map[string]interface{}{"query": "nobody"}})
	if integrations.ErrorCode(err) != integrations.ErrorCodeInvalidInput {
		t.Fatalf("broad traversal error=%v", err)
	}
	integrationError, ok := err.(*integrations.Error)
	if !ok || integrationError.ReasonCode != "wecom_provider_call_budget_exceeded" {
		t.Fatalf("broad traversal error=%#v", err)
	}
	if simpleListCalls.Load() != 0 {
		t.Fatalf("broad traversal queried users %d times before failing", simpleListCalls.Load())
	}
}

func TestContactSearchRejectsEmptyVisibleScopeBeforeSubjectAPIs(t *testing.T) {
	t.Skip("visible-scope enforcement is now performed by the official provider search endpoint")
	var subjectCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/cgi-bin/service/v2/get_auth_info" {
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{
				"errcode": 0, "auth_corp_info": map[string]interface{}{"corpid": "corp"},
				"auth_info": map[string]interface{}{"agent": []map[string]interface{}{{
					"agentid": 1000002, "auth_mode": wecomAuthorizationModeAdministrator,
					"privilege": map[string]interface{}{"level": 1},
				}}},
			})
			return
		}
		subjectCalls.Add(1)
		http.NotFound(writer, request)
	}))
	defer server.Close()
	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	configureSuiteTestAdapter(adapter)
	connection := &integrations.ResolvedConnection{ID: "empty", IntegrationID: IntegrationID, DriverID: DriverID, Credentials: map[string]string{"auth_corpid": "corp", "agent_id": "1000002", "permanent_code": "permanent"}}
	_, err = adapter.Execute(context.Background(), integrations.ActionRequest{IntegrationID: IntegrationID, ActionID: ActionContactSearch, Connection: connection, Input: map[string]interface{}{"query": "nobody"}})
	if integrations.ErrorCode(err) != integrations.ErrorCodeInsufficientScope {
		t.Fatalf("empty visible scope error=%v", err)
	}
	integrationError, ok := err.(*integrations.Error)
	if !ok || integrationError.ReasonCode != "wecom_visible_scope_empty" {
		t.Fatalf("empty visible scope error=%#v", err)
	}
	if subjectCalls.Load() != 0 {
		t.Fatalf("empty visible scope made %d subject calls", subjectCalls.Load())
	}
}

func TestContactSearchPreflightsUnifiedProviderCallBudget(t *testing.T) {
	t.Skip("legacy subject traversal was removed in favor of one bounded official search request")
	tests := []struct {
		name  string
		field string
		value interface{}
	}{
		{name: "users", field: "allow_user", value: func() []string {
			values := make([]string, maxContactSearchProviderCalls+1)
			for index := range values {
				values[index] = fmt.Sprintf("user-%d", index)
			}
			return values
		}()},
		{name: "tags", field: "allow_tag", value: func() []int {
			values := make([]int, maxContactSearchProviderCalls+1)
			for index := range values {
				values[index] = index + 1
			}
			return values
		}()},
		{name: "departments", field: "allow_party", value: func() []int {
			values := make([]int, maxContactSearchProviderCalls+1)
			for index := range values {
				values[index] = index + 1
			}
			return values
		}()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var subjectCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				if request.URL.Path == "/cgi-bin/service/v2/get_auth_info" {
					privilege := map[string]interface{}{"level": 1, test.field: test.value}
					_ = json.NewEncoder(writer).Encode(map[string]interface{}{
						"errcode": 0, "auth_corp_info": map[string]interface{}{"corpid": "corp"},
						"auth_info": map[string]interface{}{"agent": []map[string]interface{}{{
							"agentid": 1000002, "auth_mode": wecomAuthorizationModeAdministrator, "privilege": privilege,
						}}},
					})
					return
				}
				subjectCalls.Add(1)
				http.NotFound(writer, request)
			}))
			defer server.Close()
			adapter, err := newForBaseURL(server.Client(), server.URL)
			if err != nil {
				t.Fatal(err)
			}
			configureSuiteTestAdapter(adapter)
			connection := &integrations.ResolvedConnection{ID: test.name, IntegrationID: IntegrationID, DriverID: DriverID, Credentials: map[string]string{"auth_corpid": "corp", "agent_id": "1000002", "permanent_code": "permanent"}}
			_, err = adapter.Execute(context.Background(), integrations.ActionRequest{IntegrationID: IntegrationID, ActionID: ActionContactSearch, Connection: connection, Input: map[string]interface{}{"query": "nobody"}})
			if integrations.ErrorCode(err) != integrations.ErrorCodeInvalidInput {
				t.Fatalf("provider call budget error=%v", err)
			}
			integrationError, ok := err.(*integrations.Error)
			if !ok || integrationError.ReasonCode != "wecom_provider_call_budget_exceeded" {
				t.Fatalf("provider call budget error=%#v", err)
			}
			if subjectCalls.Load() != 0 {
				t.Fatalf("provider call budget made %d subject calls", subjectCalls.Load())
			}
		})
	}
}

func TestMemberAuthorizationSearchNeverCallsDepartmentAPI(t *testing.T) {
	t.Skip("the official provider search no longer calls department APIs for either authorization mode")
	var departmentCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/cgi-bin/service/v2/get_auth_info":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{
				"errcode": 0, "auth_corp_info": map[string]interface{}{"corpid": "corp"},
				"auth_info": map[string]interface{}{"agent": []map[string]interface{}{{
					"agentid": 1000002, "auth_mode": wecomAuthorizationModeMember,
					"privilege": map[string]interface{}{"level": 1, "allow_party": []int{1}, "allow_user": []string{"member-user"}},
				}}},
			})
		case "/cgi-bin/service/get_corp_token":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "access_token": "token", "expires_in": 7200})
		case "/cgi-bin/user/get":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "userid": "member-user", "name": "member-user", "department": []int{1}})
		case "/cgi-bin/user/simplelist":
			departmentCalls.Add(1)
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 60011})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	configureSuiteTestAdapter(adapter)
	connection := &integrations.ResolvedConnection{ID: "member", IntegrationID: IntegrationID, DriverID: DriverID, CredentialVersion: 1, Revision: 1, Credentials: map[string]string{"auth_corpid": "corp", "agent_id": "1000002", "permanent_code": "permanent"}}
	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{IntegrationID: IntegrationID, ActionID: ActionContactSearch, Connection: connection, Input: map[string]interface{}{"query": "member-user"}})
	if err != nil || result.ResultCount != 1 {
		t.Fatalf("member authorization search result=%#v err=%v", result, err)
	}
	if departmentCalls.Load() != 0 {
		t.Fatal("member authorization attempted a forbidden department query")
	}
	_, err = adapter.Execute(context.Background(), integrations.ActionRequest{IntegrationID: IntegrationID, ActionID: ActionContactSearch, Connection: connection, Input: map[string]interface{}{"query": "member-user", "department_id": 1}})
	if integrations.ErrorCode(err) != integrations.ErrorCodeInsufficientScope {
		t.Fatalf("member department search error=%v", err)
	}
	if departmentCalls.Load() != 0 {
		t.Fatal("member authorization sent an explicit forbidden department query")
	}
}

func TestAccessTokenCacheIncludesConnectionRevision(t *testing.T) {
	var tokenCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path != "/cgi-bin/service/get_corp_token" {
			http.NotFound(writer, request)
			return
		}
		call := tokenCalls.Add(1)
		_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "access_token": fmt.Sprintf("token-%d", call), "expires_in": 7200})
	}))
	defer server.Close()
	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	configureSuiteTestAdapter(adapter)
	base := &integrations.ResolvedConnection{ID: "connection", IntegrationID: IntegrationID, DriverID: DriverID, CredentialVersion: 1, Revision: 1, Credentials: map[string]string{"auth_corpid": "corp", "agent_id": "1000002", "permanent_code": "permanent"}}
	firstCredentials, err := parseCredentials(base)
	if err != nil {
		t.Fatal(err)
	}
	first, err := adapter.accessToken(context.Background(), firstCredentials)
	if err != nil {
		t.Fatal(err)
	}
	changed := *base
	changed.Revision = 2
	secondCredentials, err := parseCredentials(&changed)
	if err != nil {
		t.Fatal(err)
	}
	second, err := adapter.accessToken(context.Background(), secondCredentials)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || tokenCalls.Load() != 2 {
		t.Fatalf("revision did not invalidate corp token cache: first=%q second=%q calls=%d", first, second, tokenCalls.Load())
	}
}

func TestGetAuthorizationInfoRejectsMalformedAuthorizationEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]interface{}, map[string]interface{}, map[string]interface{})
	}{
		{name: "missing corp id", mutate: func(corp, _, _ map[string]interface{}) { delete(corp, "corpid") }},
		{name: "mismatched corp id", mutate: func(corp, _, _ map[string]interface{}) { corp["corpid"] = "other-corp" }},
		{name: "missing auth mode", mutate: func(_, agent, _ map[string]interface{}) { delete(agent, "auth_mode") }},
		{name: "unknown auth mode", mutate: func(_, agent, _ map[string]interface{}) { agent["auth_mode"] = 2 }},
		{name: "missing privilege", mutate: func(_, agent, _ map[string]interface{}) { delete(agent, "privilege") }},
		{name: "missing privilege level", mutate: func(_, _ map[string]interface{}, privilege map[string]interface{}) { delete(privilege, "level") }},
		{name: "ordinary level zero", mutate: func(_, _ map[string]interface{}, privilege map[string]interface{}) { privilege["level"] = 0 }},
		{name: "customized nonzero level", mutate: func(_, agent, _ map[string]interface{}) { agent["is_customized_app"] = true }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			corp := map[string]interface{}{"corpid": "corp"}
			privilege := map[string]interface{}{"level": 1, "allow_user": []string{"member"}}
			agent := map[string]interface{}{"agentid": 1000002, "auth_mode": wecomAuthorizationModeAdministrator, "privilege": privilege}
			test.mutate(corp, agent, privilege)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				if request.URL.Path != "/cgi-bin/service/v2/get_auth_info" {
					http.NotFound(writer, request)
					return
				}
				_ = json.NewEncoder(writer).Encode(map[string]interface{}{
					"errcode": 0, "auth_corp_info": corp,
					"auth_info": map[string]interface{}{"agent": []map[string]interface{}{agent}},
				})
			}))
			defer server.Close()
			adapter, err := newForBaseURL(server.Client(), server.URL)
			if err != nil {
				t.Fatal(err)
			}
			configureSuiteTestAdapter(adapter)
			_, _, err = adapter.getAuthorizationInfo(context.Background(), "corp", "permanent", 1000002)
			if integrations.ErrorCode(err) != integrations.ErrorCodeResponseInvalid {
				t.Fatalf("malformed authorization error=%v", err)
			}
		})
	}
}

func TestContactReadPrivilegeFailsClosedForUnsupportedValidLevels(t *testing.T) {
	administratorMode := wecomAuthorizationModeAdministrator
	tests := []struct {
		name       string
		level      int
		customized bool
		wantReason string
	}{
		{name: "write only", level: 5, wantReason: "wecom_contacts_read_permission_missing"},
		{name: "customized permissions require separate evidence", level: 0, customized: true, wantReason: "wecom_customized_app_permissions_unsupported"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			level := test.level
			agent, err := validateAuthorizationAgent(authorizationAgent{
				AgentID: 1000002, AuthMode: &administratorMode,
				Privilege: &authorizationPrivilege{Level: &level}, IsCustomizedApp: test.customized,
			})
			if err != nil {
				t.Fatalf("structurally valid authorization rejected: %v", err)
			}
			err = authorizeContactRead(agent)
			if integrations.ErrorCode(err) != integrations.ErrorCodeInsufficientScope {
				t.Fatalf("contact read privilege error=%v", err)
			}
			integrationError, ok := err.(*integrations.Error)
			if !ok || integrationError.ReasonCode != test.wantReason {
				t.Fatalf("contact read privilege error=%#v", err)
			}
		})
	}
}

func TestProviderInstallUsesV2PermanentCodeAndAuthInfo(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		paths = append(paths, request.URL.Path)
		if request.URL.Query().Get("suite_access_token") != "suite-token" {
			t.Errorf("suite_access_token=%q", request.URL.Query().Get("suite_access_token"))
		}
		switch request.URL.Path {
		case "/cgi-bin/service/get_pre_auth_code":
			_ = json.NewEncoder(writer).Encode(map[string]any{"errcode": 0, "pre_auth_code": "pre-auth", "expires_in": 1200})
		case "/cgi-bin/service/set_session_info":
			if request.Method != http.MethodPost {
				t.Errorf("set_session_info method=%s", request.Method)
			}
			var payload installSessionInfoRequest
			_ = json.NewDecoder(request.Body).Decode(&payload)
			if payload.PreAuthCode != "pre-auth" || payload.SessionInfo.AuthType != 1 {
				t.Errorf("set_session_info payload=%#v", payload)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"errcode": 0, "errmsg": "ok"})
		case "/cgi-bin/service/v2/get_permanent_code":
			var payload map[string]string
			_ = json.NewDecoder(request.Body).Decode(&payload)
			if payload["auth_code"] != "one-time-code" {
				t.Errorf("auth_code=%q", payload["auth_code"])
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"errcode": 0, "permanent_code": "permanent", "auth_corp_info": map[string]any{"corpid": "wwcorp", "corp_name": "Example Corp"}})
		case "/cgi-bin/service/v2/get_auth_info":
			var payload map[string]string
			_ = json.NewDecoder(request.Body).Decode(&payload)
			if payload["auth_corpid"] != "wwcorp" || payload["permanent_code"] != "permanent" {
				t.Errorf("auth info payload=%#v", payload)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"errcode": 0, "auth_corp_info": map[string]any{"corpid": "wwcorp", "corp_name": "Example Corp"},
				"auth_info": map[string]any{"agent": []map[string]any{{
					"agentid": 1000002, "name": "ZGI", "auth_mode": wecomAuthorizationModeAdministrator,
					"privilege": map[string]any{
						"level": 1, "allow_party": []int{7}, "allow_user": []string{"yang"}, "allow_tag": []int{9},
						"extra_party": []int{8, 7}, "extra_user": []string{"extra", "yang"}, "extra_tag": []int{10, 9},
					},
				}}},
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	configureSuiteTestAdapter(adapter)
	authorizationURL, err := adapter.BuildProviderInstallAuthorizationURL(context.Background(), integrations.ProviderInstallAuthorizationRequest{
		RedirectURI: adapter.suite.InstallCallbackURL,
		State:       "0123456789abcdef",
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := url.Parse(authorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	if target.Host != "open.work.weixin.qq.com" || target.Query().Get("suite_id") != "ww_suite" || target.Query().Get("pre_auth_code") != "pre-auth" || target.Query().Get("state") != "0123456789abcdef" {
		t.Fatalf("authorization URL=%s", authorizationURL)
	}
	grant, err := adapter.ExchangeProviderInstallCode(context.Background(), integrations.ProviderInstallExchangeRequest{AuthorizationCode: "one-time-code"})
	if err != nil {
		t.Fatal(err)
	}
	defer grant.Destroy()
	if grant.Credentials["auth_corpid"] != "wwcorp" || grant.Credentials["permanent_code"] != "permanent" || grant.Credentials["agent_id"] != "1000002" {
		t.Fatalf("grant credentials=%#v", grant.Credentials)
	}
	if len(grant.Config) != 1 || grant.Config["authorization_mode"] != "suite" {
		t.Fatalf("grant configuration must not persist mutable authorization evidence: %#v", grant.Config)
	}
	if grant.ExternalTenantHash == "" || strings.Contains(grant.ExternalTenantHash, "wwcorp") {
		t.Fatalf("external tenant hash=%q", grant.ExternalTenantHash)
	}
	wantPaths := []string{"/cgi-bin/service/get_pre_auth_code", "/cgi-bin/service/set_session_info", "/cgi-bin/service/v2/get_permanent_code", "/cgi-bin/service/v2/get_auth_info"}
	if strings.Join(paths, ",") != strings.Join(wantPaths, ",") {
		t.Fatalf("paths=%v want=%v", paths, wantPaths)
	}
}

func TestProviderInstallUsesFormalAuthorizationByDefault(t *testing.T) {
	var gotAuthType int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/cgi-bin/service/get_pre_auth_code":
			_ = json.NewEncoder(writer).Encode(map[string]any{"errcode": 0, "pre_auth_code": "formal-pre-auth", "expires_in": 1200})
		case "/cgi-bin/service/set_session_info":
			var payload installSessionInfoRequest
			_ = json.NewDecoder(request.Body).Decode(&payload)
			gotAuthType = payload.SessionInfo.AuthType
			if payload.PreAuthCode != "formal-pre-auth" {
				t.Errorf("pre_auth_code=%q", payload.PreAuthCode)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"errcode": 0, "errmsg": "ok"})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	configureSuiteTestAdapter(adapter)
	adapter.suite.InstallAuthMode = ""
	if _, err := adapter.BuildProviderInstallAuthorizationURL(context.Background(), integrations.ProviderInstallAuthorizationRequest{
		RedirectURI: adapter.suite.InstallCallbackURL,
		State:       "formal-state",
	}); err != nil {
		t.Fatal(err)
	}
	if gotAuthType != 0 {
		t.Fatalf("formal auth_type=%d want=0", gotAuthType)
	}
}

func TestProviderInstallStopsWhenSessionConfigurationFails(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		paths = append(paths, request.URL.Path)
		switch request.URL.Path {
		case "/cgi-bin/service/get_pre_auth_code":
			_ = json.NewEncoder(writer).Encode(map[string]any{"errcode": 0, "pre_auth_code": "pre-auth", "expires_in": 1200})
		case "/cgi-bin/service/set_session_info":
			_ = json.NewEncoder(writer).Encode(map[string]any{"errcode": 40093, "errmsg": "invalid session"})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	configureSuiteTestAdapter(adapter)
	authorizationURL, err := adapter.BuildProviderInstallAuthorizationURL(context.Background(), integrations.ProviderInstallAuthorizationRequest{
		RedirectURI: adapter.suite.InstallCallbackURL,
		State:       "failure-state",
	})
	if err == nil || authorizationURL != "" {
		t.Fatalf("authorizationURL=%q error=%v", authorizationURL, err)
	}
	wantPaths := []string{"/cgi-bin/service/get_pre_auth_code", "/cgi-bin/service/set_session_info"}
	if strings.Join(paths, ",") != strings.Join(wantPaths, ",") {
		t.Fatalf("paths=%v want=%v", paths, wantPaths)
	}
}

func TestSuiteCallbackVerifiesSignatureDecryptsAndRejectsWrongReceiver(t *testing.T) {
	adapter, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	configureSuiteTestAdapter(adapter)
	timestamp, nonce := "1723776000", "nonce-1"
	payload := []byte(`<xml><SuiteId>ww_suite</SuiteId><InfoType>cancel_auth</InfoType><AuthCorpId>wwcorp</AuthCorpId><TimeStamp>1723776000</TimeStamp></xml>`)
	encrypted := encryptWeComCallbackForTest(t, adapter.suite.EncodingAESKey, adapter.suite.SuiteID, payload)
	body, _ := xml.Marshal(encryptedCallbackEnvelope{Encrypt: encrypted})
	event, err := adapter.ReceiveSuiteCallback(context.Background(), callbackSignature(adapter.suite.CallbackToken, timestamp, nonce, encrypted), timestamp, nonce, body)
	if err != nil {
		t.Fatal(err)
	}
	if event.InfoType != "cancel_auth" || event.AuthCorpID != "wwcorp" {
		t.Fatalf("event=%#v", event)
	}
	if _, err := adapter.ReceiveSuiteCallback(context.Background(), "bad-signature", timestamp, nonce, body); integrations.ErrorCode(err) != integrations.ErrorCodeAuthInvalid {
		t.Fatalf("wrong signature error=%v", err)
	}
	wrongReceiver := encryptWeComCallbackForTest(t, adapter.suite.EncodingAESKey, adapter.suite.ProviderCorpID, payload)
	wrongBody, _ := xml.Marshal(encryptedCallbackEnvelope{Encrypt: wrongReceiver})
	if _, err := adapter.ReceiveSuiteCallback(context.Background(), callbackSignature(adapter.suite.CallbackToken, timestamp, nonce, wrongReceiver), timestamp, nonce, wrongBody); integrations.ErrorCode(err) != integrations.ErrorCodeAuthInvalid {
		t.Fatalf("wrong receiver error=%v", err)
	}
}

func TestSuiteCallbackURLUsesProviderCorpIDReceiver(t *testing.T) {
	adapter, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	configureSuiteTestAdapter(adapter)
	timestamp, nonce := "1723776000", "nonce-suite-url"
	echo := []byte("verified-echo")

	providerEncryptedEcho := encryptWeComCallbackForTest(t, adapter.suite.EncodingAESKey, adapter.suite.ProviderCorpID, echo)
	plaintext, err := adapter.VerifySuiteCallbackURL(
		callbackSignature(adapter.suite.CallbackToken, timestamp, nonce, providerEncryptedEcho),
		timestamp,
		nonce,
		providerEncryptedEcho,
	)
	if err != nil {
		t.Fatal(err)
	}
	if plaintext != string(echo) {
		t.Fatalf("plaintext=%q want=%q", plaintext, echo)
	}

	suiteEncryptedEcho := encryptWeComCallbackForTest(t, adapter.suite.EncodingAESKey, adapter.suite.SuiteID, echo)
	_, err = adapter.VerifySuiteCallbackURL(
		callbackSignature(adapter.suite.CallbackToken, timestamp, nonce, suiteEncryptedEcho),
		timestamp,
		nonce,
		suiteEncryptedEcho,
	)
	if got := CallbackFailureStage(err); got != callbackFailureStageReceiverMismatch {
		t.Fatalf("failure stage=%q", got)
	}
	if integrations.ErrorCode(err) != integrations.ErrorCodeAuthInvalid {
		t.Fatalf("error code=%q", integrations.ErrorCode(err))
	}
}

func TestDataCallbackUsesTheCorpIDReceiver(t *testing.T) {
	adapter, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	configureSuiteTestAdapter(adapter)
	timestamp, nonce := "1723776000", "nonce-data"
	receiverID := "ww_provider_or_authorized_corp"
	echo := []byte("verified-echo")
	encryptedEcho := encryptWeComCallbackForTest(t, adapter.suite.EncodingAESKey, receiverID, echo)
	plaintext, err := adapter.VerifyDataCallbackURL(receiverID, callbackSignature(adapter.suite.CallbackToken, timestamp, nonce, encryptedEcho), timestamp, nonce, encryptedEcho)
	if err != nil {
		t.Fatal(err)
	}
	if plaintext != string(echo) {
		t.Fatalf("plaintext=%q", plaintext)
	}

	payload := []byte(`<xml><ToUserName>ww_provider_or_authorized_corp</ToUserName><InfoType>create_auth</InfoType></xml>`)
	encryptedPayload := encryptWeComCallbackForTest(t, adapter.suite.EncodingAESKey, receiverID, payload)
	body, _ := xml.Marshal(encryptedCallbackEnvelope{Encrypt: encryptedPayload})
	if err := adapter.ReceiveDataCallback(receiverID, callbackSignature(adapter.suite.CallbackToken, timestamp, nonce, encryptedPayload), timestamp, nonce, body); err != nil {
		t.Fatal(err)
	}
	if err := adapter.ReceiveDataCallback("ww_wrong", callbackSignature(adapter.suite.CallbackToken, timestamp, nonce, encryptedPayload), timestamp, nonce, body); integrations.ErrorCode(err) != integrations.ErrorCodeAuthInvalid {
		t.Fatalf("wrong receiver error=%v", err)
	}
}

func TestSuiteCallbackRestoresBase64PlusDecodedAsSpace(t *testing.T) {
	adapter, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	configureSuiteTestAdapter(adapter)
	timestamp, nonce := "1723776000", "nonce-plus"

	var echo []byte
	var encryptedEcho string
	for suffix := 0; suffix < 256; suffix++ {
		echo = []byte(fmt.Sprintf("verified-echo-%d", suffix))
		encryptedEcho = encryptWeComCallbackForTest(t, adapter.suite.EncodingAESKey, adapter.suite.ProviderCorpID, echo)
		if strings.Contains(encryptedEcho, "+") {
			break
		}
	}
	if !strings.Contains(encryptedEcho, "+") {
		t.Fatal("test ciphertext does not contain a Base64 plus character")
	}

	queryDecodedEcho := strings.ReplaceAll(encryptedEcho, "+", " ")
	plaintext, err := adapter.VerifySuiteCallbackURL(
		callbackSignature(adapter.suite.CallbackToken, timestamp, nonce, encryptedEcho),
		timestamp,
		nonce,
		queryDecodedEcho,
	)
	if err != nil {
		t.Fatal(err)
	}
	if plaintext != string(echo) {
		t.Fatalf("plaintext=%q want=%q", plaintext, echo)
	}
}

func TestCallbackFailureStages(t *testing.T) {
	timestamp, nonce := "1723776000", "nonce-stage"
	newAdapter := func(t *testing.T) *Adapter {
		t.Helper()
		adapter, err := New(nil)
		if err != nil {
			t.Fatal(err)
		}
		configureSuiteTestAdapter(adapter)
		return adapter
	}

	t.Run("provider corp id missing", func(t *testing.T) {
		adapter := newAdapter(t)
		adapter.suite.ProviderCorpID = ""
		_, err := adapter.VerifySuiteCallbackURL("signature", timestamp, nonce, "ciphertext")
		if got := CallbackFailureStage(err); got != callbackFailureStageConfigurationUnavailable {
			t.Fatalf("failure stage=%q", got)
		}
		if integrations.ErrorCode(err) != integrations.ErrorCodeDisabled {
			t.Fatalf("error code=%q", integrations.ErrorCode(err))
		}
	})

	t.Run("input incomplete", func(t *testing.T) {
		adapter := newAdapter(t)
		_, err := adapter.VerifySuiteCallbackURL("", timestamp, nonce, "")
		if got := CallbackFailureStage(err); got != callbackFailureStageInputIncomplete {
			t.Fatalf("failure stage=%q", got)
		}
		if integrations.ErrorCode(err) != integrations.ErrorCodeAuthInvalid {
			t.Fatalf("error code=%q", integrations.ErrorCode(err))
		}
	})

	t.Run("signature mismatch", func(t *testing.T) {
		adapter := newAdapter(t)
		encrypted := encryptWeComCallbackForTest(t, adapter.suite.EncodingAESKey, adapter.suite.SuiteID, []byte("echo"))
		_, err := adapter.VerifySuiteCallbackURL("bad-signature", timestamp, nonce, encrypted)
		if got := CallbackFailureStage(err); got != callbackFailureStageSignatureMismatch {
			t.Fatalf("failure stage=%q", got)
		}
	})

	t.Run("ciphertext invalid", func(t *testing.T) {
		adapter := newAdapter(t)
		encrypted := "not-base64"
		_, err := adapter.VerifySuiteCallbackURL(callbackSignature(adapter.suite.CallbackToken, timestamp, nonce, encrypted), timestamp, nonce, encrypted)
		if got := CallbackFailureStage(err); got != callbackFailureStageCiphertextInvalid {
			t.Fatalf("failure stage=%q", got)
		}
	})

	t.Run("padding invalid", func(t *testing.T) {
		adapter := newAdapter(t)
		encrypted := base64.StdEncoding.EncodeToString(make([]byte, aes.BlockSize*2))
		_, err := adapter.VerifySuiteCallbackURL(callbackSignature(adapter.suite.CallbackToken, timestamp, nonce, encrypted), timestamp, nonce, encrypted)
		if got := CallbackFailureStage(err); got != callbackFailureStagePaddingInvalid {
			t.Fatalf("failure stage=%q", got)
		}
	})

	t.Run("receiver mismatch", func(t *testing.T) {
		adapter := newAdapter(t)
		encrypted := encryptWeComCallbackForTest(t, adapter.suite.EncodingAESKey, "ww_other", []byte("echo"))
		_, err := adapter.VerifySuiteCallbackURL(callbackSignature(adapter.suite.CallbackToken, timestamp, nonce, encrypted), timestamp, nonce, encrypted)
		if got := CallbackFailureStage(err); got != callbackFailureStageReceiverMismatch {
			t.Fatalf("failure stage=%q", got)
		}
		if integrations.ErrorCode(err) != integrations.ErrorCodeAuthInvalid {
			t.Fatalf("error code=%q", integrations.ErrorCode(err))
		}
	})
}

func TestValidateConnectionPreservesSafeWeComDiagnostics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"errcode":60020,"errmsg":"not allow to access from your ip","request_id":"req-wecom-1"}`))
	}))
	defer server.Close()
	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	configureSuiteTestAdapter(adapter)
	_, err = adapter.ValidateConnection(context.Background(), &integrations.ResolvedConnection{
		ID: "connection", IntegrationID: IntegrationID, DriverID: DriverID,
		Credentials: map[string]string{"auth_corpid": "corp", "agent_id": "1000002", "permanent_code": "permanent"},
	})
	if integrations.ErrorCode(err) != integrations.ErrorCodeAccessDenied {
		t.Fatalf("error = %v", err)
	}
	diagnostics := integrations.ProviderDiagnosticsFromError(err)
	if diagnostics.ErrorCode != "60020" || diagnostics.RequestID != "req-wecom-1" || diagnostics.HTTPStatus != http.StatusOK {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	if strings.Contains(err.Error(), "not allow to access") {
		t.Fatalf("provider errmsg must not leak: %v", err)
	}
}

func TestProviderPermissionErrorsKeepDistinctRecoveryReasons(t *testing.T) {
	tests := []struct {
		providerCode int
		wantCode     string
		wantReason   string
	}{
		{providerCode: 60011, wantCode: integrations.ErrorCodeInsufficientScope, wantReason: "wecom_subject_outside_visible_scope"},
		{providerCode: 60021, wantCode: integrations.ErrorCodeInsufficientScope, wantReason: "wecom_member_outside_visible_scope"},
		{providerCode: 660003, wantCode: integrations.ErrorCodeInsufficientScope, wantReason: "wecom_member_outside_visible_scope"},
		{providerCode: 660004, wantCode: integrations.ErrorCodeInsufficientScope, wantReason: "wecom_department_outside_visible_scope"},
		{providerCode: 660005, wantCode: integrations.ErrorCodeInsufficientScope, wantReason: "wecom_tag_outside_visible_scope"},
		{providerCode: 48002, wantCode: integrations.ErrorCodeInsufficientScope, wantReason: "wecom_api_permission_missing"},
		{providerCode: 50001, wantCode: integrations.ErrorCodeConnectionInvalid, wantReason: "wecom_trusted_domain_mismatch"},
		{providerCode: 60020, wantCode: integrations.ErrorCodeAccessDenied, wantReason: "wecom_trusted_ip_denied"},
	}
	for _, test := range tests {
		t.Run(strconv.Itoa(test.providerCode), func(t *testing.T) {
			err := mapProviderError(http.StatusOK, apiEnvelope{ErrCode: test.providerCode, RequestID: "request-1"})
			if integrations.ErrorCode(err) != test.wantCode {
				t.Fatalf("error code=%q want=%q", integrations.ErrorCode(err), test.wantCode)
			}
			integrationError, ok := err.(*integrations.Error)
			if !ok || integrationError.ReasonCode != test.wantReason {
				t.Fatalf("error=%#v want reason=%q", err, test.wantReason)
			}
			diagnostics := integrations.ProviderDiagnosticsFromError(err)
			if diagnostics.ErrorCode != strconv.Itoa(test.providerCode) || diagnostics.RequestID != "request-1" {
				t.Fatalf("diagnostics=%#v", diagnostics)
			}
		})
	}
}

func TestWeComGuideCoversProviderInstallAndPermissionReview(t *testing.T) {
	guide := ProviderDefinition().AuthMethods[0].SetupGuide
	if guide == nil || !guide.ExpandedByDefault {
		t.Fatal("WeCom setup guide must be expanded")
	}
	ids := map[string]bool{}
	for _, step := range guide.Steps {
		ids[step.ID] = true
	}
	for _, id := range []string{"provider_configuration", "start_install", "review_permissions", "verify_connection"} {
		if !ids[id] {
			t.Fatalf("setup guide missing %q", id)
		}
	}
}

func configureSuiteTestAdapter(adapter *Adapter) {
	adapter.suite = SuiteConfig{
		SuiteID: "ww_suite", ProviderCorpID: "ww_provider", ProviderSecret: "provider-secret", SuiteSecret: "suite-secret", CallbackToken: "token",
		EncodingAESKey:     "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG",
		InstallAuthMode:    installAuthModeTest,
		InstallCallbackURL: "https://example.com/console/api/integrations/wecom/install/callback",
	}
	adapter.suiteState = &SuiteStateRepository{}
	adapter.suiteToken = cachedToken{value: "suite-token", expiresAt: time.Now().Add(time.Hour)}
}

func encryptWeComCallbackForTest(t *testing.T, encodingAESKey, receiverID string, message []byte) string {
	t.Helper()
	key, err := base64.StdEncoding.DecodeString(encodingAESKey + "=")
	if err != nil {
		t.Fatal(err)
	}
	length := make([]byte, 4)
	binary.BigEndian.PutUint32(length, uint32(len(message)))
	plaintext := append(bytes.Repeat([]byte{0x42}, 16), length...)
	plaintext = append(plaintext, message...)
	plaintext = append(plaintext, []byte(receiverID)...)
	padding := 32 - len(plaintext)%32
	plaintext = append(plaintext, bytes.Repeat([]byte{byte(padding)}, padding)...)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext := make([]byte, len(plaintext))
	cipher.NewCBCEncrypter(block, key[:aes.BlockSize]).CryptBlocks(ciphertext, plaintext)
	return base64.StdEncoding.EncodeToString(ciphertext)
}
func toJSON(value interface{}) string { raw, _ := json.Marshal(value); return string(raw) }

func assertWeComOutputContract(t *testing.T, actionID string, output map[string]interface{}) {
	t.Helper()
	var schema map[string]interface{}
	for _, action := range ProviderDefinitionForConfig(SuiteConfig{CustomerContactEnabled: true, CustomerMassMessageEnabled: true}).Actions {
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
