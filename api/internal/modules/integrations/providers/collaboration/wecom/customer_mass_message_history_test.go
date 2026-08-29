package wecom

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/integrations/core"
)

func TestCustomerMassMessageHistoryDiscoversOpaqueTasks(t *testing.T) {
	var historyCalls atomic.Int32
	var captured map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/cgi-bin/service/get_corp_token":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "access_token": "token", "expires_in": 7200})
		case "/cgi-bin/externalcontact/get_groupmsg_list_v2":
			historyCalls.Add(1)
			_ = json.NewDecoder(request.Body).Decode(&captured)
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{
				"errcode":     0,
				"requestid":   "history-request",
				"next_cursor": "next-history-page",
				"group_msg_list": []map[string]interface{}{
					{
						"msgid": "provider-enterprise-task", "create_time": "1787000000", "create_type": 0,
						"text":        map[string]interface{}{"content": "企业创建内容"},
						"attachments": []map[string]interface{}{{"msgtype": "image", "image": map[string]interface{}{"media_id": "secret-media", "pic_url": "https://secret.example/image"}}},
					},
					{
						"msgid": "provider-member-task", "creator": "alice", "create_time": 1787000100, "create_type": 1,
						"text":        map[string]interface{}{"content": "成员创建内容"},
						"attachments": []map[string]interface{}{{"msgtype": "link", "link": map[string]interface{}{"url": "https://secret.example/link"}}},
					},
				},
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
	connection := testWeComConnection("history-connection", "history-corp", "1000002")
	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		IntegrationID: IntegrationID,
		ActionID:      ActionCustomerMassMessageHistoryList,
		Connection:    connection,
		Input: map[string]interface{}{
			"chat_type": "single", "start_time": float64(1787000000), "end_time": float64(1787003600),
			"filter_type": float64(2), "limit": float64(100), "cursor": "history-cursor",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertWeComOutputContract(t, ActionCustomerMassMessageHistoryList, result.Output)
	if historyCalls.Load() != 1 || captured["chat_type"] != "single" || captured["filter_type"] != float64(2) || captured["limit"] != float64(100) || captured["cursor"] != "history-cursor" {
		t.Fatalf("history request=%#v calls=%d", captured, historyCalls.Load())
	}
	if result.ProviderRequestID != "history-request" || result.Output["next_cursor"] != "next-history-page" || result.Output["is_last"] != false {
		t.Fatalf("history result=%#v", result)
	}
	records := result.Output["records"].([]map[string]interface{})
	if len(records) != 2 || records[0]["source"] != "enterprise" || records[1]["source"] != "member" {
		t.Fatalf("history records=%#v", records)
	}
	if _, exists := records[0]["creator_ref"]; exists {
		t.Fatalf("API-created enterprise record unexpectedly exposed a creator: %#v", records[0])
	}
	if _, exists := records[1]["creator_ref"]; !exists {
		t.Fatalf("member-created record did not contain an opaque creator: %#v", records[1])
	}
	creds, err := parseCredentials(connection)
	if err != nil {
		t.Fatal(err)
	}
	for index, providerID := range []string{"provider-enterprise-task", "provider-member-task"} {
		taskRef := records[index]["task_ref"].(string)
		if strings.Contains(taskRef, providerID) {
			t.Fatalf("provider msgid leaked in task_ref: %q", taskRef)
		}
		if decoded, decodeErr := adapter.decodeCustomerTaskRef(taskRef, creds); decodeErr != nil || decoded != providerID {
			t.Fatalf("decoded task=%q err=%v, want %q", decoded, decodeErr, providerID)
		}
	}
	serialized, _ := json.Marshal(result.Output)
	if strings.Contains(string(serialized), "secret-media") || strings.Contains(string(serialized), "secret.example") {
		t.Fatalf("unsafe attachment details leaked: %s", serialized)
	}
}

func TestCustomerMassMessageHistoryValidatesRangeAndCreatorBinding(t *testing.T) {
	var historyCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/cgi-bin/service/get_corp_token":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "access_token": "token", "expires_in": 7200})
		case "/cgi-bin/externalcontact/get_groupmsg_list_v2":
			historyCalls.Add(1)
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{
				"errcode": 0,
				"group_msg_list": []map[string]interface{}{{
					"msgid": "provider-task", "creator": "bob", "create_time": 1787000000, "create_type": 1,
				}},
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
	connection := testWeComConnection("history-connection", "history-corp", "1000002")
	creds, err := parseCredentials(connection)
	if err != nil {
		t.Fatal(err)
	}
	creatorRef, err := adapter.encodeSubjectRef(creds, SubjectTypeMember, SubjectIDKindUserID, "alice", "", "")
	if err != nil {
		t.Fatal(err)
	}

	_, err = adapter.Execute(context.Background(), integrations.ActionRequest{
		IntegrationID: IntegrationID, ActionID: ActionCustomerMassMessageHistoryList, Connection: connection,
		Input: map[string]interface{}{
			"chat_type": "single", "start_time": float64(1),
			"end_time": float64(1 + maxCustomerMassMessageHistoryRangeSeconds + 1),
		},
	})
	if integrations.ErrorCode(err) != integrations.ErrorCodeInvalidInput || historyCalls.Load() != 0 {
		t.Fatalf("invalid range error=%v calls=%d", err, historyCalls.Load())
	}

	_, err = adapter.Execute(context.Background(), integrations.ActionRequest{
		IntegrationID: IntegrationID, ActionID: ActionCustomerMassMessageHistoryList, Connection: connection,
		Input: map[string]interface{}{
			"chat_type": "single", "start_time": float64(1787000000), "end_time": float64(1787003600),
			"creator_ref": creatorRef, "filter_type": float64(1),
		},
	})
	if integrations.ErrorCode(err) != integrations.ErrorCodeResponseInvalid || historyCalls.Load() != 1 {
		t.Fatalf("creator mismatch error=%v calls=%d", err, historyCalls.Load())
	}
}

func TestCustomerMassMessageStatusProjectionOmitsUnavailableSendTime(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/cgi-bin/service/get_corp_token":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"errcode": 0, "access_token": "token", "expires_in": 7200})
		case "/cgi-bin/externalcontact/get_groupmsg_task":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{
				"errcode": 0,
				"task_list": []map[string]interface{}{
					{"userid": "alice", "status": 0},
					{"userid": "bob", "status": 2, "send_time": 1787000200},
				},
			})
		case "/cgi-bin/externalcontact/get_groupmsg_send_result":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{
				"errcode": 0,
				"send_list": []map[string]interface{}{
					{"userid": "alice", "external_userid": "external-0", "status": 0},
					{"userid": "alice", "external_userid": "external-1", "status": 1, "send_time": 1787000300},
					{"userid": "alice", "external_userid": "external-2", "status": 2, "send_time": 999},
					{"userid": "alice", "external_userid": "external-3", "status": 3},
				},
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
	connection := testWeComConnection("status-connection", "status-corp", "1000002")
	creds, err := parseCredentials(connection)
	if err != nil {
		t.Fatal(err)
	}
	taskRef, _ := adapter.encodeCustomerTaskRef(creds, "provider-task")
	senderRef, _ := adapter.encodeSubjectRef(creds, SubjectTypeMember, SubjectIDKindUserID, "alice", "", "")

	taskResult, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		IntegrationID: IntegrationID, ActionID: ActionCustomerMassMessageTaskList, Connection: connection,
		Input: map[string]interface{}{"task_ref": taskRef},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertWeComOutputContract(t, ActionCustomerMassMessageTaskList, taskResult.Output)
	tasks := taskResult.Output["tasks"].([]map[string]interface{})
	if tasks[0]["status"] != "pending_member_send" || tasks[0]["raw_status"] != 0 {
		t.Fatalf("pending task=%#v", tasks[0])
	}
	if _, exists := tasks[0]["sent_at"]; exists {
		t.Fatalf("pending task fabricated sent_at: %#v", tasks[0])
	}
	if tasks[1]["status"] != "sent" || tasks[1]["raw_status"] != 2 || tasks[1]["sent_at"] != int64(1787000200) {
		t.Fatalf("sent task=%#v", tasks[1])
	}

	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		IntegrationID: IntegrationID, ActionID: ActionCustomerMassMessageResultList, Connection: connection,
		Input: map[string]interface{}{"task_ref": taskRef, "sender_ref": senderRef},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertWeComOutputContract(t, ActionCustomerMassMessageResultList, result.Output)
	results := result.Output["results"].([]map[string]interface{})
	wantStatuses := []string{"pending", "sent", "failed_customer_not_friend", "failed_duplicate_mass_message"}
	for index, want := range wantStatuses {
		if results[index]["status"] != want || results[index]["raw_status"] != index {
			t.Fatalf("result[%d]=%#v want status=%q raw=%d", index, results[index], want, index)
		}
		_, hasSentAt := results[index]["sent_at"]
		if hasSentAt != (index == 1) {
			t.Fatalf("result[%d] sent_at presence=%v: %#v", index, hasSentAt, results[index])
		}
	}
	if results[1]["sent_at"] != int64(1787000300) {
		t.Fatalf("sent result=%#v", results[1])
	}
}
