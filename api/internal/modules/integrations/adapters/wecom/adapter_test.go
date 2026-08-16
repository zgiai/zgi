package wecom

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
	if !ok || len(definition.Actions) != 5 {
		t.Fatalf("actions = %d", len(definition.Actions))
	}
	for _, action := range definition.Actions {
		if action.ID == ActionMessageSendUser && (action.DefaultPolicy.Enabled || action.SuccessDeduplication == nil) {
			t.Fatal("send action must be disabled by default and success-deduplicated")
		}
	}
}

func TestAdapterValidatesSearchesAndSendsWithTokenCache(t *testing.T) {
	var tokenCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/cgi-bin/gettoken":
			tokenCalls.Add(1)
			json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "access_token": "token", "expires_in": 7200})
		case "/cgi-bin/agent/get":
			json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "agentid": 1000002, "name": "ZGI Assistant", "square_logo_url": "https://example.com/logo.png"})
		case "/cgi-bin/user/simplelist":
			json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "userlist": []map[string]interface{}{{"userid": "yangzhihang", "name": "杨志航", "department": []int{1}}}})
		case "/cgi-bin/user/get":
			json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "userid": "yangzhihang", "name": "杨志航", "department": []int{1}, "position": "Engineer", "status": 1})
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
	connection := &integrations.ResolvedConnection{ID: "connection", IntegrationID: IntegrationID, DriverID: DriverID, Credentials: map[string]string{"corp_id": "corp", "agent_id": "1000002", "secret": "secret"}}
	profile, err := adapter.ValidateConnection(context.Background(), connection)
	if err != nil {
		t.Fatal(err)
	}
	if profile.DisplayName != "ZGI Assistant" {
		t.Fatalf("profile=%#v", profile)
	}
	search, err := adapter.Execute(context.Background(), integrations.ActionRequest{IntegrationID: IntegrationID, ActionID: ActionContactSearch, Connection: connection, Input: map[string]interface{}{"query": "杨志航"}})
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
}

func TestRecipientReferenceRejectsRawUserID(t *testing.T) {
	if _, err := decodeRecipientRef("yangzhihang"); err == nil {
		t.Fatal("raw provider user id must not be accepted as a recipient reference")
	}
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
	_, err = adapter.ValidateConnection(context.Background(), &integrations.ResolvedConnection{
		ID: "connection", IntegrationID: IntegrationID, DriverID: DriverID,
		Credentials: map[string]string{"corp_id": "corp", "agent_id": "1000002", "secret": "secret"},
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

func TestWeComGuideCoversCredentialOriginVisibilityAndTrustedIP(t *testing.T) {
	guide := ProviderDefinition().AuthMethods[0].SetupGuide
	if guide == nil || !guide.ExpandedByDefault {
		t.Fatal("WeCom setup guide must be expanded")
	}
	ids := map[string]bool{}
	for _, step := range guide.Steps {
		ids[step.ID] = true
	}
	for _, id := range []string{"copy_corp_id", "copy_app_credentials", "configure_visibility", "configure_trusted_ip", "save_and_verify"} {
		if !ids[id] {
			t.Fatalf("setup guide missing %q", id)
		}
	}
}
func toJSON(value interface{}) string { raw, _ := json.Marshal(value); return string(raw) }

func assertWeComOutputContract(t *testing.T, actionID string, output map[string]interface{}) {
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
