package dingtalk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

func TestExtendedActionsHaveExpectedGovernance(t *testing.T) {
	definition := ProviderDefinition()
	actions := make(map[string]integrations.ActionDefinition, len(definition.Actions))
	for _, action := range definition.Actions {
		actions[action.ID] = action
	}
	for _, actionID := range []string{
		ActionDepartmentSearch, ActionDepartmentGet, ActionDepartmentUsers,
		ActionRoleList, ActionRoleUsers, ActionAttendanceList, ActionMessageSendDept,
	} {
		if _, ok := actions[actionID]; !ok {
			t.Fatalf("action %s is not registered", actionID)
		}
	}
	attendance := actions[ActionAttendanceList]
	if attendance.RiskLevel != toolgovernance.RiskLevelMedium || attendance.DefaultPolicy == nil || !attendance.DefaultPolicy.Enabled {
		t.Fatalf("attendance governance = %#v", attendance)
	}
	sendDepartment := actions[ActionMessageSendDept]
	if sendDepartment.DefaultPolicy == nil || sendDepartment.DefaultPolicy.Enabled || sendDepartment.DefaultPolicy.ApprovalPolicy != toolgovernance.ApprovalPolicyAlwaysAsk {
		t.Fatalf("department send must fail closed: %#v", sendDepartment)
	}
	if sendDepartment.SuccessDeduplication == nil || len(sendDepartment.PreparationHints) != 1 || sendDepartment.PreparationHints[0].ActionID != ActionDepartmentSearch {
		t.Fatalf("department send preparation = %#v", sendDepartment)
	}
}

func TestAdapterExtendedDirectoryAttendanceAndDepartmentNotificationFlow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1.0/oauth2/accessToken":
			_, _ = w.Write([]byte(`{"accessToken":"token-1","expireIn":7200}`))
		case "/v1.0/contact/departments/search":
			if r.Header.Get("x-acs-dingtalk-access-token") != "token-1" {
				t.Error("missing API token header")
			}
			_, _ = w.Write([]byte(`{"list":[2]}`))
		case "/topapi/v2/department/get":
			assertLegacyToken(t, r)
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","result":{"dept_id":2,"name":"研发部","parent_id":1}}`))
		case "/topapi/v2/user/list":
			assertLegacyToken(t, r)
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","result":{"has_more":false,"list":[{"userid":"user-1","name":"张三","title":"工程师","active":true}]}}`))
		case "/topapi/role/list":
			assertLegacyToken(t, r)
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","result":{"hasMore":false,"list":[{"name":"研发","roles":[{"id":7,"name":"技术负责人"}]}]}}`))
		case "/topapi/role/simplelist":
			assertLegacyToken(t, r)
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","result":{"hasMore":false,"list":[{"userid":"user-1","name":"张三"}]}}`))
		case "/attendance/listRecord":
			assertLegacyToken(t, r)
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","recordresult":[{"workDate":1785859200000,"planCheckTime":1785891600000,"userCheckTime":1785891540000,"checkType":"OnDuty","timeResult":"Normal","locationResult":"Normal","sourceType":"USER","userAddress":"secret address","userLatitude":30.0,"userLongitude":120.0}]}`))
		case "/topapi/message/corpconversation/asyncsend_v2":
			assertLegacyToken(t, r)
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["dept_id_list"] != "2" || body["to_all_user"] != false {
				t.Errorf("department send body = %#v", body)
			}
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","task_id":7788}`))
		case "/topapi/message/corpconversation/getsendresult":
			assertLegacyToken(t, r)
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","result":{"send_result":{"read_user_id_list":["user-1"],"unread_user_id_list":["user-2"],"failed_user_id_list":["user-3"]}}}`))
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

	searched, err := adapter.Execute(context.Background(), integrations.ActionRequest{ActionID: ActionDepartmentSearch, Connection: connection, Input: map[string]interface{}{"query": "研发", "max_results": 10}})
	if err != nil {
		t.Fatal(err)
	}
	departments := searched.Output["departments"].([]map[string]interface{})
	if hasMore, _ := searched.Output["has_more"].(bool); hasMore {
		t.Fatalf("under-filled raw department page reported has_more: %#v", searched.Output)
	}
	departmentRef := departments[0]["department_ref"].(string)
	if departmentRef == "" {
		t.Fatal("department reference is empty")
	}
	if _, err = adapter.Execute(context.Background(), integrations.ActionRequest{ActionID: ActionDepartmentGet, Connection: connection, Input: map[string]interface{}{"department_ref": departmentRef}}); err != nil {
		t.Fatal(err)
	}
	membersResult, err := adapter.Execute(context.Background(), integrations.ActionRequest{ActionID: ActionDepartmentUsers, Connection: connection, Input: map[string]interface{}{"department_ref": departmentRef, "max_results": 10}})
	if err != nil {
		t.Fatal(err)
	}
	members := membersResult.Output["members"].([]map[string]interface{})
	recipientRef := members[0]["recipient_ref"].(string)

	rolesResult, err := adapter.Execute(context.Background(), integrations.ActionRequest{ActionID: ActionRoleList, Connection: connection, Input: map[string]interface{}{"max_results": 10}})
	if err != nil {
		t.Fatal(err)
	}
	roles := rolesResult.Output["roles"].([]map[string]interface{})
	if _, err = adapter.Execute(context.Background(), integrations.ActionRequest{ActionID: ActionRoleUsers, Connection: connection, Input: map[string]interface{}{"role_ref": roles[0]["role_ref"], "max_results": 10}}); err != nil {
		t.Fatal(err)
	}

	end := time.Now().Add(-time.Hour).UTC()
	start := end.Add(-24 * time.Hour)
	attendance, err := adapter.Execute(context.Background(), integrations.ActionRequest{ActionID: ActionAttendanceList, Connection: connection, Input: map[string]interface{}{"recipient_ref": recipientRef, "start_time": start.Format(time.RFC3339), "end_time": end.Format(time.RFC3339)}})
	if err != nil {
		t.Fatal(err)
	}
	record := attendance.Output["records"].([]map[string]interface{})[0]
	for _, forbidden := range []string{"userAddress", "userLatitude", "userLongitude", "remark", "images"} {
		if _, ok := record[forbidden]; ok {
			t.Fatalf("attendance output exposes %s", forbidden)
		}
	}

	sent, err := adapter.Execute(context.Background(), integrations.ActionRequest{ActionID: ActionMessageSendDept, Connection: connection, Input: map[string]interface{}{"department_ref": departmentRef, "content": "请查看通知"}})
	if err != nil {
		t.Fatal(err)
	}
	messageRef := sent.Output["notification"].(map[string]interface{})["message_ref"].(string)
	status, err := adapter.Execute(context.Background(), integrations.ActionRequest{ActionID: ActionMessageStatusGet, Connection: connection, Input: map[string]interface{}{"message_ref": messageRef}})
	if err != nil {
		t.Fatal(err)
	}
	notification := status.Output["notification"].(map[string]interface{})
	if notification["delivery_status"] != "partially_delivered" || notification["delivered_count"] != 2 || notification["failed_count"] != 1 {
		t.Fatalf("department status = %#v", notification)
	}
}

func TestDepartmentSearchPreservesRawPageCompletenessAfterFiltering(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1.0/oauth2/accessToken":
			_, _ = w.Write([]byte(`{"accessToken":"token-1","expireIn":7200}`))
		case "/v1.0/contact/departments/search":
			_, _ = w.Write([]byte(`{"list":[2,0]}`))
		case "/topapi/v2/department/get":
			assertLegacyToken(t, r)
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","result":{"dept_id":2,"name":"研发部","parent_id":1}}`))
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
		ActionID:      ActionDepartmentSearch,
		Connection:    testConnection("connection-1"),
		Input:         map[string]interface{}{"query": "研发", "max_results": 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(result.Output["departments"].([]map[string]interface{})); got != 1 {
		t.Fatalf("filtered department count = %d, output=%#v", got, result.Output)
	}
	if hasMore, _ := result.Output["has_more"].(bool); !hasMore {
		t.Fatalf("full raw department page lost pagination evidence: %#v", result.Output)
	}
	assertDingTalkOutputContract(t, ActionDepartmentSearch, result.Output)
}

func TestExtendedReferencesAreConnectionBound(t *testing.T) {
	departmentRef := encodeDepartmentRef("connection-1", 2)
	if _, err := decodeDepartmentRef(departmentRef, "connection-2"); err == nil {
		t.Fatal("expected cross-connection department rejection")
	}
	roleRef := encodeRoleRef("connection-1", 7)
	if _, err := decodeRoleRef(roleRef, "connection-2"); err == nil {
		t.Fatal("expected cross-connection role rejection")
	}
	messageRef := encodeDepartmentMessageRef("connection-1", 8, 2)
	if _, err := decodeMessageRef(messageRef, "connection-2"); err == nil {
		t.Fatal("expected cross-connection department message rejection")
	}
}

func TestAttendanceRangeRejectsUnsafeWindows(t *testing.T) {
	now := time.Now().UTC()
	if _, _, err := attendanceWindow(now.Format(time.RFC3339), now.Add(8*24*time.Hour).Format(time.RFC3339)); err == nil {
		t.Fatal("expected range longer than seven days to be rejected")
	}
	if _, _, err := attendanceWindow(now.Format(time.RFC3339), now.Add(-time.Hour).Format(time.RFC3339)); err == nil {
		t.Fatal("expected reversed range to be rejected")
	}
}
