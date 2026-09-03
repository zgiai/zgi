package wecom

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations/core"
)

func TestCustomerActionContractsAndIdentityDomains(t *testing.T) {
	var payloadMu sync.Mutex
	var massMessagePayloads []map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/cgi-bin/service/get_corp_token":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "access_token": "corp-token", "expires_in": 7200})
		case "/cgi-bin/externalcontact/get_follow_user_list":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "requestid": "req-follow", "follow_user": []string{"alice", "bob"}})
		case "/cgi-bin/externalcontact/list":
			if got := request.URL.Query().Get("userid"); got != "alice" {
				t.Errorf("customer owner userid = %q", got)
			}
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "external_userid": []string{"ext-1", "ext-2"}})
		case "/cgi-bin/externalcontact/get":
			if got := request.URL.Query().Get("external_userid"); got != "ext-1" {
				t.Errorf("external_userid = %q", got)
			}
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{
				"errcode":          0,
				"external_contact": map[string]interface{}{"external_userid": "ext-1", "name": "客户甲", "type": 1, "corp_name": ""},
				"follow_user":      []map[string]interface{}{{"userid": "alice", "remark": "采购", "createtime": 1234, "add_way": 3}},
				"next_cursor":      "",
			})
		case "/cgi-bin/externalcontact/batch/get_by_user":
			var payload map[string]interface{}
			_ = json.NewDecoder(request.Body).Decode(&payload)
			if owners, _ := payload["userid_list"].([]interface{}); len(owners) != 1 || owners[0] != "alice" {
				t.Errorf("batch owners = %#v", payload["userid_list"])
			}
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{
				"errcode": 0,
				"external_contact_list": []map[string]interface{}{{
					"external_contact": map[string]interface{}{"external_userid": "ext-1", "name": "客户甲", "type": 1},
					"follow_info":      map[string]interface{}{"userid": "alice", "remark": "采购", "createtime": 1234, "add_way": 3},
				}},
				"next_cursor": "next-customer-page",
			})
		case "/cgi-bin/externalcontact/groupchat/list":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{
				"errcode": 0, "group_chat_list": []map[string]interface{}{{"chat_id": "chat-1", "status": 0}}, "next_cursor": "",
			})
		case "/cgi-bin/externalcontact/groupchat/get":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{
				"errcode": 0,
				"group_chat": map[string]interface{}{
					"chat_id": "chat-1", "name": "客户群", "owner": "alice", "create_time": 5678,
					"member_list": []map[string]interface{}{{"userid": "alice", "type": 1}, {"userid": "ext-1", "type": 2}},
				},
			})
		case "/cgi-bin/externalcontact/add_msg_template":
			var payload map[string]interface{}
			_ = json.NewDecoder(request.Body).Decode(&payload)
			payloadMu.Lock()
			massMessagePayloads = append(massMessagePayloads, payload)
			payloadMu.Unlock()
			if payload["chat_type"] == "single" {
				_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "msgid": "customer-task", "fail_list": []string{"ext-2"}})
				return
			}
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "msgid": "group-task", "fail_list": []string{}})
		case "/cgi-bin/externalcontact/get_groupmsg_task":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{
				"errcode": 0, "task_list": []map[string]interface{}{{"userid": "alice", "status": 2, "send_time": 2345}}, "next_cursor": "",
			})
		case "/cgi-bin/externalcontact/get_groupmsg_send_result":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{
				"errcode": 0, "send_list": []map[string]interface{}{{"userid": "alice", "external_userid": "ext-1", "status": 1, "send_time": 3456}}, "next_cursor": "",
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
	configureCustomerSuiteTestAdapter(adapter)
	connection := &integrations.ResolvedConnection{
		ID: "connection-customer", IntegrationID: IntegrationID, DriverID: DriverID,
		CredentialVersion: 1, Revision: 1,
		Credentials: map[string]string{"auth_corpid": "corp-customer", "agent_id": "1000002", "permanent_code": "permanent"},
	}
	execute := func(actionID string, input map[string]interface{}) *integrations.ActionResult {
		t.Helper()
		result, executeErr := adapter.Execute(context.Background(), integrations.ActionRequest{
			IntegrationID: IntegrationID, ActionID: actionID, Connection: connection, Input: input,
		})
		if executeErr != nil {
			t.Fatalf("%s: %v", actionID, executeErr)
		}
		assertWeComOutputContract(t, actionID, result.Output)
		return result
	}

	followUsers := execute(ActionCustomerFollowUserList, map[string]interface{}{"max_results": float64(10)})
	if followUsers.ProviderRequestID != "req-follow" || followUsers.ProviderDiagnostics.RequestID != "req-follow" {
		t.Fatalf("provider request ID = %#v", followUsers)
	}
	owners := followUsers.Output["owners"].([]map[string]interface{})
	ownerRef := owners[0]["owner_ref"].(string)
	if strings.Contains(ownerRef, "alice") {
		t.Fatal("member provider ID leaked in opaque owner reference")
	}

	listedCustomers := execute(ActionCustomerListByOwner, map[string]interface{}{"owner_ref": ownerRef})
	customerRefs := listedCustomers.Output["customer_refs"].([]map[string]interface{})
	customerRef1 := customerRefs[0]["customer_ref"].(string)
	customerRef2 := customerRefs[1]["customer_ref"].(string)
	if strings.Contains(customerRef1, "ext-1") {
		t.Fatal("external_userid leaked in opaque customer reference")
	}
	_ = execute(ActionCustomerGet, map[string]interface{}{"customer_ref": customerRef1})
	batch := execute(ActionCustomerBatchGetByOwner, map[string]interface{}{"owner_refs": []interface{}{ownerRef}, "limit": float64(50)})
	if batch.Output["is_last"] != false || batch.Output["next_cursor"] != "next-customer-page" {
		t.Fatalf("batch paging = %#v", batch.Output)
	}

	groups := execute(ActionCustomerGroupList, map[string]interface{}{"owner_refs": []interface{}{ownerRef}})
	groupRef := groups.Output["groups"].([]map[string]interface{})[0]["group_ref"].(string)
	group := execute(ActionCustomerGroupGet, map[string]interface{}{"group_ref": groupRef})
	projectedGroup := group.Output["group"].(map[string]interface{})
	if projectedGroup["member_count"] != 2 {
		t.Fatalf("group = %#v", projectedGroup)
	}

	customerTask := execute(ActionCustomerMassMessageCreate, map[string]interface{}{
		"customer_refs": []interface{}{customerRef1, customerRef2}, "content": "你好",
	})
	customerTaskOutput := customerTask.Output["task"].(map[string]interface{})
	if customerTask.Output["operation_status"] != "partially_succeeded" || customerTaskOutput["task_status"] != "created_pending_member_send" || customerTaskOutput["accepted_count"] != 1 || customerTaskOutput["rejected_count"] != 1 {
		t.Fatalf("customer task = %#v", customerTaskOutput)
	}
	taskRef := customerTaskOutput["task_ref"].(string)
	if strings.Contains(taskRef, "customer-task") {
		t.Fatal("provider task ID leaked directly instead of a signed task reference")
	}
	tasks := execute(ActionCustomerMassMessageTaskList, map[string]interface{}{"task_ref": taskRef})
	if got := tasks.Output["tasks"].([]map[string]interface{}); len(got) != 1 || got[0]["status"] != "sent" || got[0]["raw_status"] != 2 {
		t.Fatalf("mass-message member tasks = %#v", tasks.Output)
	}
	results := execute(ActionCustomerMassMessageResultList, map[string]interface{}{"task_ref": taskRef, "sender_ref": ownerRef})
	if got := results.Output["results"].([]map[string]interface{}); len(got) != 1 || got[0]["target_type"] != "customer" {
		t.Fatalf("mass-message send results = %#v", results.Output)
	}
	groupTask := execute(ActionCustomerGroupMessageCreate, map[string]interface{}{
		"group_refs": []interface{}{groupRef}, "sender_ref": ownerRef, "content": "群通知",
	})
	if groupTask.Output["operation_status"] != "succeeded" || groupTask.Output["task"].(map[string]interface{})["delivery_status"] != "pending_member_send" {
		t.Fatalf("group task = %#v", groupTask.Output)
	}

	payloadMu.Lock()
	defer payloadMu.Unlock()
	if len(massMessagePayloads) != 2 || massMessagePayloads[0]["chat_type"] != "single" || massMessagePayloads[1]["chat_type"] != "group" {
		t.Fatalf("mass-message payloads = %#v", massMessagePayloads)
	}
	for _, payload := range massMessagePayloads {
		if allowSelect, ok := payload["allow_select"].(bool); !ok || allowSelect {
			t.Fatalf("mass-message allow_select must be fixed false: %#v", payload)
		}
	}

	if _, executeErr := adapter.Execute(context.Background(), integrations.ActionRequest{
		IntegrationID: IntegrationID, ActionID: ActionMessageSendUser, Connection: connection,
		Input: map[string]interface{}{"recipient_ref": customerRef1, "content": "must not send"},
	}); executeErr == nil || integrations.ErrorCode(executeErr) != integrations.ErrorCodeInvalidInput {
		t.Fatalf("customer reference must not be accepted by member send: %v", executeErr)
	}
}

func TestCustomerReferencesAreConnectionAndDomainBound(t *testing.T) {
	adapter, err := newForBaseURL(http.DefaultClient, "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	configureCustomerSuiteTestAdapter(adapter)
	first := credentials{AuthCorpID: "corp-1", AgentID: "1000002", ConnectionID: "connection-1"}
	second := credentials{AuthCorpID: "corp-1", AgentID: "1000002", ConnectionID: "connection-2"}
	ref, err := adapter.encodeSubjectRef(first, SubjectTypeCustomer, SubjectIDKindExternalUserID, "external-1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.decodeSubjectRef(ref, second, SubjectTypeCustomer, SubjectIDKindExternalUserID); err == nil {
		t.Fatal("cross-connection customer reference was accepted")
	}
	if _, err := adapter.decodeSubjectRef(ref, first, SubjectTypeMember, SubjectIDKindUserID); err == nil {
		t.Fatal("customer reference was accepted as an internal member")
	}
}

func TestCustomerTaskReferenceIsEncryptedRandomizedAndBound(t *testing.T) {
	adapter, err := newForBaseURL(http.DefaultClient, "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	configureCustomerSuiteTestAdapter(adapter)
	creds := credentials{AuthCorpID: "corp-1", AgentID: "1000002", ConnectionID: "connection-1"}
	first, err := adapter.encodeCustomerTaskRef(creds, "provider-msgid-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := adapter.encodeCustomerTaskRef(creds, "provider-msgid-1")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("identical task claims produced the same encrypted reference")
	}
	if decoded, err := adapter.decodeCustomerTaskRef(first, creds); err != nil || decoded != "provider-msgid-1" {
		t.Fatalf("decoded task id=%q err=%v", decoded, err)
	}
	parts := strings.Split(first, ".")
	if len(parts) != 3 || parts[0] != "v2" {
		t.Fatalf("task reference envelope=%q", first)
	}
	nonce, nonceErr := base64.RawURLEncoding.DecodeString(parts[1])
	ciphertext, ciphertextErr := base64.RawURLEncoding.DecodeString(parts[2])
	if nonceErr != nil || ciphertextErr != nil || bytes.Contains(nonce, []byte("provider-msgid-1")) || bytes.Contains(ciphertext, []byte("provider-msgid-1")) {
		t.Fatalf("task reference envelope exposed or failed to encode provider msgid: nonce=%v ciphertext=%v", nonceErr, ciphertextErr)
	}
	for name, other := range map[string]credentials{
		"connection": {AuthCorpID: "corp-1", AgentID: "1000002", ConnectionID: "connection-2"},
		"tenant":     {AuthCorpID: "corp-2", AgentID: "1000002", ConnectionID: "connection-1"},
		"agent":      {AuthCorpID: "corp-1", AgentID: "1000003", ConnectionID: "connection-1"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := adapter.decodeCustomerTaskRef(first, other); err == nil {
				t.Fatal("cross-binding task reference was accepted")
			}
		})
	}
	tampered := first[:len(first)-1] + "A"
	if strings.HasSuffix(first, "A") {
		tampered = first[:len(first)-1] + "B"
	}
	if _, err := adapter.decodeCustomerTaskRef(tampered, creds); err == nil {
		t.Fatal("tampered task reference was accepted")
	}
	wrongType, err := adapter.encodeSubjectRef(creds, SubjectTypeCustomer, SubjectIDKindExternalUserID, "provider-msgid-1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.decodeCustomerTaskRef(wrongType, creds); err == nil {
		t.Fatal("customer reference was accepted as a mass-message task")
	}
}

func TestCustomerActionsDeduplicateDecodedProviderIDs(t *testing.T) {
	var batchOwnerIDs, groupOwnerIDs, massCustomerIDs []interface{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/cgi-bin/service/get_corp_token":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "access_token": "token", "expires_in": 7200})
		case "/cgi-bin/externalcontact/batch/get_by_user":
			var payload map[string]interface{}
			_ = json.NewDecoder(request.Body).Decode(&payload)
			batchOwnerIDs, _ = payload["userid_list"].([]interface{})
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "external_contact_list": []interface{}{}})
		case "/cgi-bin/externalcontact/groupchat/list":
			var payload map[string]interface{}
			_ = json.NewDecoder(request.Body).Decode(&payload)
			filter, _ := payload["owner_filter"].(map[string]interface{})
			groupOwnerIDs, _ = filter["userid_list"].([]interface{})
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "group_chat_list": []interface{}{}})
		case "/cgi-bin/externalcontact/add_msg_template":
			var payload map[string]interface{}
			_ = json.NewDecoder(request.Body).Decode(&payload)
			massCustomerIDs, _ = payload["external_userid"].([]interface{})
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "msgid": "task-1", "fail_list": []interface{}{}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	configureCustomerSuiteTestAdapter(adapter)
	creds := credentials{AuthCorpID: "corp", AgentID: "1000002", ConnectionID: "connection"}
	ownerRef1, _ := adapter.encodeSubjectRef(creds, SubjectTypeMember, SubjectIDKindUserID, "owner-1", "", "")
	ownerRef2, _ := adapter.encodeSubjectRef(creds, SubjectTypeMember, SubjectIDKindUserID, "owner-1", "", "")
	customerRef1, _ := adapter.encodeSubjectRef(creds, SubjectTypeCustomer, SubjectIDKindExternalUserID, "external-1", "", "")
	customerRef2, _ := adapter.encodeSubjectRef(creds, SubjectTypeCustomer, SubjectIDKindExternalUserID, "external-1", "", "")
	if ownerRef1 == ownerRef2 || customerRef1 == customerRef2 {
		t.Fatal("test references must differ despite carrying the same provider identity")
	}
	ownerInput := map[string]interface{}{"owner_refs": []interface{}{ownerRef1, ownerRef2}}
	if _, _, _, err := adapter.batchGetCustomersByOwner(context.Background(), creds, ownerInput); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := adapter.listCustomerGroups(context.Background(), creds, ownerInput); err != nil {
		t.Fatal(err)
	}
	result, _, _, err := adapter.createCustomerMassMessage(context.Background(), creds, map[string]interface{}{"customer_refs": []interface{}{customerRef1, customerRef2}, "content": "hello"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(batchOwnerIDs) != 1 || len(groupOwnerIDs) != 1 || len(massCustomerIDs) != 1 {
		t.Fatalf("decoded provider IDs were not deduplicated: batch=%#v groups=%#v mass=%#v", batchOwnerIDs, groupOwnerIDs, massCustomerIDs)
	}
	if task := result["task"].(map[string]interface{}); task["accepted_count"] != 1 {
		t.Fatalf("deduplicated accepted count=%#v", task)
	}
}

func TestCustomerMassMessageActionsRemainGuardedTasks(t *testing.T) {
	wanted := map[string]bool{ActionCustomerMassMessageCreate: false, ActionCustomerGroupMessageCreate: false}
	for _, action := range ProviderDefinitionForConfig(SuiteConfig{CustomerContactEnabled: true, CustomerMassMessageEnabled: true}).Actions {
		if _, ok := wanted[action.ID]; !ok {
			continue
		}
		wanted[action.ID] = true
		if action.DefaultPolicy == nil || action.DefaultPolicy.Enabled || action.DefaultPolicy.ApprovalPolicy != toolgovernance.ApprovalPolicyAlwaysAsk {
			t.Fatalf("%s policy = %#v", action.ID, action.DefaultPolicy)
		}
		if action.Idempotent || action.SuccessDeduplication == nil {
			t.Fatalf("%s must use guarded success-only replay protection", action.ID)
		}
		if strings.Contains(strings.ToLower(action.Description), "delivered") && !strings.Contains(strings.ToLower(action.Description), "not delivered") {
			t.Fatalf("%s overstates delivery: %s", action.ID, action.Description)
		}
	}
	for actionID, found := range wanted {
		if !found {
			t.Fatalf("customer mass-message action %q is missing", actionID)
		}
	}
	for _, action := range ProviderDefinitionForConfig(SuiteConfig{CustomerContactEnabled: true, CustomerMassMessageEnabled: true}).Actions {
		if strings.Contains(action.ID, "customer") && strings.Contains(action.ID, "direct_message") {
			t.Fatalf("unsupported direct customer message action was registered: %s", action.ID)
		}
	}
}

func TestCustomerActionsRequireExplicitDeploymentCapabilities(t *testing.T) {
	base := ProviderDefinition()
	if countCustomerActions(base.Actions) != 0 || hasScope(base.Scopes, ScopeCustomers) || hasScope(base.Scopes, ScopeCustomerMassMessage) {
		t.Fatalf("default definition exposed customer capabilities: actions=%d scopes=%#v", countCustomerActions(base.Actions), base.Scopes)
	}
	contact := ProviderDefinitionForConfig(SuiteConfig{CustomerContactEnabled: true})
	if countCustomerActions(contact.Actions) != 6 || !hasScope(contact.Scopes, ScopeCustomers) || !hasScope(contact.Scopes, ScopeCustomerGroups) || hasScope(contact.Scopes, ScopeCustomerMassMessage) {
		t.Fatalf("contact-only definition=%#v", contact)
	}
	all := ProviderDefinitionForConfig(SuiteConfig{CustomerContactEnabled: true, CustomerMassMessageEnabled: true})
	if countCustomerActions(all.Actions) != 11 || !hasScope(all.Scopes, ScopeCustomerMassMessage) {
		t.Fatalf("full customer definition actions=%d scopes=%#v", countCustomerActions(all.Actions), all.Scopes)
	}

	adapter, err := newForBaseURL(http.DefaultClient, "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	configureSuiteTestAdapter(adapter)
	connection := testWeComConnection("connection", "corp", "1000002")
	if _, err := adapter.Execute(context.Background(), integrations.ActionRequest{IntegrationID: IntegrationID, ActionID: ActionCustomerFollowUserList, Connection: connection}); integrations.ErrorCode(err) != integrations.ErrorCodeDisabled {
		t.Fatalf("disabled customer read error=%v", err)
	}
	adapter.suite.CustomerContactEnabled = true
	if _, err := adapter.Execute(context.Background(), integrations.ActionRequest{IntegrationID: IntegrationID, ActionID: ActionCustomerMassMessageTaskList, Connection: connection}); integrations.ErrorCode(err) != integrations.ErrorCodeDisabled {
		t.Fatalf("disabled customer mass-message error=%v", err)
	}
}

func countCustomerActions(actions []integrations.ActionDefinition) int {
	count := 0
	for _, action := range actions {
		if isCustomerContactAction(action.ID) || isCustomerMassMessageAction(action.ID) {
			count++
		}
	}
	return count
}

func hasScope(scopes []integrations.ProviderScopeDefinition, scopeID string) bool {
	for _, scope := range scopes {
		if scope.ID == scopeID {
			return true
		}
	}
	return false
}

func TestCustomerLocalPaginationAndCursorBinding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/cgi-bin/service/get_corp_token":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "access_token": "token", "expires_in": 7200})
		case "/cgi-bin/externalcontact/get_follow_user_list":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "follow_user": []string{"alice", "bob", "carol"}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	configureCustomerSuiteTestAdapter(adapter)
	connection := &integrations.ResolvedConnection{ID: "page-connection", IntegrationID: IntegrationID, DriverID: DriverID, CredentialVersion: 1, Revision: 1, Credentials: map[string]string{"auth_corpid": "corp", "agent_id": "1000002", "permanent_code": "permanent"}}
	execute := func(input map[string]interface{}) *integrations.ActionResult {
		result, executeErr := adapter.Execute(context.Background(), integrations.ActionRequest{IntegrationID: IntegrationID, ActionID: ActionCustomerFollowUserList, Connection: connection, Input: input})
		if executeErr != nil {
			t.Fatal(executeErr)
		}
		return result
	}
	first := execute(map[string]interface{}{"max_results": float64(2)})
	if first.Output["is_last"] != false || first.Output["next_cursor"] == "" {
		t.Fatalf("first local page = %#v", first.Output)
	}
	second := execute(map[string]interface{}{"max_results": float64(2), "cursor": first.Output["next_cursor"]})
	if second.Output["is_last"] != true || len(second.Output["owners"].([]map[string]interface{})) != 1 {
		t.Fatalf("second local page = %#v", second.Output)
	}
	otherConnection := *connection
	otherConnection.ID = "other-page-connection"
	_, executeErr := adapter.Execute(context.Background(), integrations.ActionRequest{IntegrationID: IntegrationID, ActionID: ActionCustomerFollowUserList, Connection: &otherConnection, Input: map[string]interface{}{"cursor": first.Output["next_cursor"]}})
	if executeErr == nil || integrations.ErrorCode(executeErr) != integrations.ErrorCodeInvalidInput {
		t.Fatalf("cross-connection cursor error = %v", executeErr)
	}
}

func TestCustomerMassMessageRejectsInvalidFailureLists(t *testing.T) {
	for _, test := range []struct {
		name     string
		failList []string
		wantCode string
	}{
		{name: "unknown", failList: []string{"unknown"}, wantCode: integrations.ErrorCodeResponseInvalid},
		{name: "duplicate", failList: []string{"ext-1", "ext-1"}, wantCode: integrations.ErrorCodeResponseInvalid},
		{name: "all rejected", failList: []string{"ext-1", "ext-2"}, wantCode: integrations.ErrorCodeProviderRejected},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				switch request.URL.Path {
				case "/cgi-bin/service/get_corp_token":
					_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "access_token": "token", "expires_in": 7200})
				case "/cgi-bin/externalcontact/add_msg_template":
					_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "msgid": "task", "fail_list": test.failList})
				default:
					http.NotFound(writer, request)
				}
			}))
			defer server.Close()
			adapter, err := newForBaseURL(server.Client(), server.URL)
			if err != nil {
				t.Fatal(err)
			}
			configureCustomerSuiteTestAdapter(adapter)
			creds := credentials{AuthCorpID: "corp", AgentID: "1000002", ConnectionID: "connection"}
			ref1, _ := adapter.encodeSubjectRef(creds, SubjectTypeCustomer, SubjectIDKindExternalUserID, "ext-1", "", "")
			ref2, _ := adapter.encodeSubjectRef(creds, SubjectTypeCustomer, SubjectIDKindExternalUserID, "ext-2", "", "")
			connection := &integrations.ResolvedConnection{ID: "connection", IntegrationID: IntegrationID, DriverID: DriverID, CredentialVersion: 1, Revision: 1, Credentials: map[string]string{"auth_corpid": "corp", "agent_id": "1000002", "permanent_code": "permanent"}}
			_, executeErr := adapter.Execute(context.Background(), integrations.ActionRequest{IntegrationID: IntegrationID, ActionID: ActionCustomerMassMessageCreate, Connection: connection, Input: map[string]interface{}{"customer_refs": []interface{}{ref1, ref2}, "content": "hello"}})
			if executeErr == nil || integrations.ErrorCode(executeErr) != test.wantCode {
				t.Fatalf("error = %v, want %s", executeErr, test.wantCode)
			}
		})
	}
}

func TestCustomerGroupMassMessageValidatesOwnerBeforeCreate(t *testing.T) {
	createCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/cgi-bin/service/get_corp_token":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "access_token": "token", "expires_in": 7200})
		case "/cgi-bin/externalcontact/groupchat/get":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "group_chat": map[string]interface{}{"chat_id": "chat-1", "owner": "bob"}})
		case "/cgi-bin/externalcontact/add_msg_template":
			createCalled = true
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "msgid": "unexpected"})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	configureCustomerSuiteTestAdapter(adapter)
	creds := credentials{AuthCorpID: "corp", AgentID: "1000002", ConnectionID: "connection"}
	groupRef, _ := adapter.encodeSubjectRef(creds, SubjectTypeCustomerGroup, SubjectIDKindChatID, "chat-1", "", "")
	senderRef, _ := adapter.encodeSubjectRef(creds, SubjectTypeMember, SubjectIDKindUserID, "alice", "", "")
	connection := &integrations.ResolvedConnection{ID: "connection", IntegrationID: IntegrationID, DriverID: DriverID, CredentialVersion: 1, Revision: 1, Credentials: map[string]string{"auth_corpid": "corp", "agent_id": "1000002", "permanent_code": "permanent"}}
	_, executeErr := adapter.Execute(context.Background(), integrations.ActionRequest{IntegrationID: IntegrationID, ActionID: ActionCustomerGroupMessageCreate, Connection: connection, Input: map[string]interface{}{"group_refs": []interface{}{groupRef}, "sender_ref": senderRef, "content": "hello"}})
	if executeErr == nil || integrations.ErrorCode(executeErr) != integrations.ErrorCodeInvalidInput || createCalled {
		t.Fatalf("owner mismatch error=%v createCalled=%v", executeErr, createCalled)
	}
}

func TestCustomerGroupMassMessageFailsBeforeProviderCallsAboveValidationBudget(t *testing.T) {
	var providerCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		providerCalls.Add(1)
		http.Error(writer, "unexpected provider call", http.StatusInternalServerError)
	}))
	defer server.Close()
	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	configureCustomerSuiteTestAdapter(adapter)
	creds := credentials{AuthCorpID: "corp", AgentID: "1000002", ConnectionID: "connection"}
	refs := make([]interface{}, 0, maxCustomerGroupMassMessageTargets+1)
	for index := 0; index <= maxCustomerGroupMassMessageTargets; index++ {
		ref, encodeErr := adapter.encodeSubjectRef(creds, SubjectTypeCustomerGroup, SubjectIDKindChatID, fmt.Sprintf("chat-%d", index), "", "")
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		refs = append(refs, ref)
	}
	_, _, _, executeErr := adapter.createCustomerMassMessage(context.Background(), creds, map[string]interface{}{
		"group_refs": refs, "sender_ref": refs[0], "content": "hello",
	}, true)
	if integrations.ErrorCode(executeErr) != integrations.ErrorCodeInvalidInput {
		t.Fatalf("over-budget group mass-message error=%v", executeErr)
	}
	if providerCalls.Load() != 0 {
		t.Fatalf("over-budget validation made %d provider calls", providerCalls.Load())
	}

	definition := ProviderDefinitionForConfig(SuiteConfig{CustomerContactEnabled: true, CustomerMassMessageEnabled: true})
	for _, action := range definition.Actions {
		if action.ID != ActionCustomerGroupMessageCreate {
			continue
		}
		properties := action.InputSchema["properties"].(map[string]interface{})
		groupRefs := properties["group_refs"].(map[string]interface{})
		if groupRefs["maxItems"] != maxCustomerGroupMassMessageTargets {
			t.Fatalf("group_refs schema maxItems=%#v", groupRefs["maxItems"])
		}
		return
	}
	t.Fatal("customer-group mass-message action was not projected")
}

func TestCustomerMassMessageUsesUTF8ByteLimit(t *testing.T) {
	adapter, err := newForBaseURL(http.DefaultClient, "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	configureCustomerSuiteTestAdapter(adapter)
	creds := credentials{AuthCorpID: "corp", AgentID: "1000002", ConnectionID: "connection"}
	ref, err := adapter.encodeSubjectRef(creds, SubjectTypeCustomer, SubjectIDKindExternalUserID, "external-1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, executeErr := adapter.createCustomerMassMessage(context.Background(), creds, map[string]interface{}{
		"customer_refs": []interface{}{ref}, "content": strings.Repeat("界", 1334),
	}, false)
	if executeErr == nil || integrations.ErrorCode(executeErr) != integrations.ErrorCodeInvalidInput {
		t.Fatalf("UTF-8 byte overflow error = %v", executeErr)
	}
}

func configureCustomerSuiteTestAdapter(adapter *Adapter) {
	configureSuiteTestAdapter(adapter)
	adapter.suite.CustomerContactEnabled = true
	adapter.suite.CustomerMassMessageEnabled = true
}
