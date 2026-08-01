package feishu

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

func TestListMessagesUsesBoundedHistoryContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/open-apis/im/v1/messages" {
			t.Errorf("request = %s %s", request.Method, request.URL)
		}
		query := request.URL.Query()
		if query.Get("container_id_type") != "chat" || query.Get("container_id") != "oc_team" ||
			query.Get("start_time") != "1700000000" || query.Get("end_time") != "1700003600" ||
			query.Get("sort_type") != "ByCreateTimeDesc" || query.Get("page_size") != "2" ||
			query.Get("page_token") != "next-in" {
			t.Errorf("query = %s", request.URL.RawQuery)
		}
		writer.Header().Set("X-Tt-Logid", "messages-log")
		longText := strings.Repeat("中", 4500)
		payload := map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"has_more": true, "page_token": "next-out",
				"items": []interface{}{
					map[string]interface{}{
						"message_id": "om_1", "root_id": "", "parent_id": "", "thread_id": "", "chat_id": "oc_team",
						"msg_type": "text", "create_time": "1700000001", "update_time": "1700000002",
						"deleted": false, "updated": true,
						"sender": map[string]interface{}{"id": "ou_sender", "id_type": "open_id", "sender_type": "user"},
						"body":   map[string]interface{}{"content": mustJSON(t, map[string]string{"text": longText})},
					},
					map[string]interface{}{
						"message_id": "om_2", "chat_id": "oc_team", "msg_type": "image",
						"sender": map[string]interface{}{"id": "ou_sender", "sender_type": "user"},
						"body":   map[string]interface{}{"content": `{"image_key":"img_secret"}`},
					},
					map[string]interface{}{"message_id": "om_3", "chat_id": "oc_team", "msg_type": "text"},
				},
			},
		}
		if err := json.NewEncoder(writer).Encode(payload); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer server.Close()
	adapter := newTestAdapter(t, server)
	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: ActionListMessages, Connection: testUserConnection(),
		Input: map[string]interface{}{
			"chat_id": "oc_team", "start_time": int64(1_700_000_000), "end_time": int64(1_700_003_600),
			"sort_type": "newest_first", "page_size": 2, "page_token": "next-in",
		},
	})
	if err != nil || result == nil || result.ProviderRequestID != "messages-log" || result.ResultCount != 2 {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	messages, _ := result.Output["messages"].([]interface{})
	if len(messages) != 2 || result.Output["next_page_token"] != "next-out" || result.Output["has_more"] != true {
		t.Fatalf("output = %#v", result.Output)
	}
	first := messages[0].(map[string]interface{})
	second := messages[1].(map[string]interface{})
	if len([]rune(first["text"].(string))) != 4000 || second["text"] != "" {
		t.Fatalf("messages = %#v", messages)
	}
	if _, exposed := second["body"]; exposed {
		t.Fatal("raw upstream message body was exposed")
	}
}

func TestListMessagesRejectsInvalidInputsBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	adapter := newTestAdapter(t, server)
	tests := []map[string]interface{}{
		{"chat_id": "   "},
		{"chat_id": "oc_team", "start_time": int64(1_700_000_000)},
		{"chat_id": "oc_team", "start_time": int64(1_700_000_000), "end_time": int64(1_700_700_000)},
		{"chat_id": "oc_team", "page_size": 51},
		{"chat_id": "oc_team", "sort_type": "sideways"},
	}
	for _, input := range tests {
		_, err := adapter.Execute(context.Background(), integrations.ActionRequest{
			ActionID: ActionListMessages, Connection: testUserConnection(), Input: input,
		})
		if integrations.ErrorCode(err) != integrations.ErrorCodeInvalidInput {
			t.Fatalf("input = %#v, err = %v", input, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("network calls = %d", calls.Load())
	}
}

func TestListCalendarEventsUsesTimeRangeAndClipsOutput(t *testing.T) {
	calendarID := "feishu.cn_team@group.calendar.feishu.cn"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.EscapedPath() != "/open-apis/calendar/v4/calendars/feishu.cn_team@group.calendar.feishu.cn/events" {
			t.Errorf("request = %s %s", request.Method, request.URL)
		}
		query := request.URL.Query()
		if query.Get("start_time") != "1700000000" || query.Get("end_time") != "1700086400" ||
			query.Get("page_size") != "50" || query.Get("page_token") != "page-in" {
			t.Errorf("query = %s", request.URL.RawQuery)
		}
		writer.Header().Set("X-Tt-Logid", "events-log")
		_, _ = io.WriteString(writer, `{"code":0,"msg":"success","data":{"has_more":true,"page_token":"page-out","items":[{"event_id":"event-1","organizer_calendar_id":"`+calendarID+`","summary":"Planning","description":"`+strings.Repeat("d", 4500)+`","start_time":{"timestamp":"1700000000","timezone":"Asia/Shanghai"},"end_time":{"timestamp":"1700003600","timezone":"Asia/Shanghai"},"status":"confirmed","visibility":"default","free_busy_status":"busy","location":{"name":"Room","address":"Floor 1"},"app_link":"https://open.feishu.cn/calendar/event-1","recurrence":"","is_exception":false},{"event_id":"event-2"}]}}`)
	}))
	defer server.Close()
	adapter := newTestAdapter(t, server)
	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: ActionListEvents, Connection: testUserConnection(),
		Input: map[string]interface{}{
			"calendar_id": calendarID, "start_time": int64(1_700_000_000), "end_time": int64(1_700_086_400),
			"page_token": "page-in",
		},
	})
	if err != nil || result == nil || result.ProviderRequestID != "events-log" || result.ResultCount != 2 {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	events := result.Output["events"].([]interface{})
	event := events[0].(map[string]interface{})
	if len([]rune(event["description"].(string))) != 4000 || event["app_link"] == "" || result.Output["next_page_token"] != "page-out" {
		t.Fatalf("output = %#v", result.Output)
	}
}

func TestListCalendarEventsRejectsProviderInvalidPageSizeBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	adapter := newTestAdapter(t, server)
	_, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: ActionListEvents, Connection: testUserConnection(),
		Input: map[string]interface{}{
			"calendar_id": "feishu.cn_team@group.calendar.feishu.cn",
			"start_time":  int64(1_700_000_000), "end_time": int64(1_700_086_400),
			"page_size": 20,
		},
	})
	if integrations.ErrorCode(err) != integrations.ErrorCodeInvalidInput || calls.Load() != 0 {
		t.Fatalf("err = %v, calls = %d", err, calls.Load())
	}
}

func TestListCalendarEventsRejectsUnboundedRangeBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	adapter := newTestAdapter(t, server)
	_, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: ActionListEvents, Connection: testUserConnection(),
		Input: map[string]interface{}{
			"calendar_id": "feishu.cn_team@group.calendar.feishu.cn",
			"start_time":  int64(1_700_000_000), "end_time": int64(1_704_000_001),
		},
	})
	if integrations.ErrorCode(err) != integrations.ErrorCodeInvalidInput || calls.Load() != 0 {
		t.Fatalf("err = %v, calls = %d", err, calls.Load())
	}
}

func TestCreateCalendarEventUsesStableIdempotencyMarkerAndNormalizedBody(t *testing.T) {
	calendarID := "feishu.cn_team@group.calendar.feishu.cn"
	var calls atomic.Int32
	var firstKey string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		call := calls.Add(1)
		if request.Method != http.MethodPost || !strings.HasSuffix(request.URL.Path, "/events") {
			t.Errorf("request = %s %s", request.Method, request.URL)
		}
		key := request.URL.Query().Get("idempotency_key")
		if len(key) != 64 || request.URL.Query().Get("user_id_type") != "open_id" {
			t.Errorf("query = %s", request.URL.RawQuery)
		}
		if call == 1 {
			firstKey = key
		} else if key != firstKey {
			t.Errorf("idempotency key changed: %q != %q", key, firstKey)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body["summary"] != "Planning" || body["visibility"] != "private" || body["free_busy_status"] != "busy" {
			t.Errorf("body = %#v", body)
		}
		start := body["start_time"].(map[string]interface{})
		if start["timestamp"] != "1700000000" || start["timezone"] != "Asia/Shanghai" {
			t.Errorf("start_time = %#v", start)
		}
		if call == 1 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(writer, `{"code":0,"msg":"temporary"}`)
			return
		}
		writer.Header().Set("X-Tt-Logid", "create-event-log")
		_, _ = io.WriteString(writer, `{"code":0,"msg":"success","data":{"event":{"event_id":"event-1","organizer_calendar_id":"`+calendarID+`","summary":"Planning","description":"Agenda","start_time":{"timestamp":"1700000000","timezone":"Asia/Shanghai"},"end_time":{"timestamp":"1700003600","timezone":"Asia/Shanghai"},"status":"confirmed","visibility":"private","free_busy_status":"busy","location":{"name":"Room","address":"Floor 1"},"app_link":"https://open.feishu.cn/calendar/event-1","recurrence":"","is_exception":false}}}`)
	}))
	defer server.Close()
	adapter := newTestAdapter(t, server)
	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		OrganizationID: "org-1", ConnectionID: "connection-1", MessageID: "message-1",
		ActionID: ActionCreateEvent, Connection: testUserConnection(),
		Input: map[string]interface{}{
			"calendar_id": calendarID, "summary": "  Planning  ", "description": "Agenda",
			"start_time": int64(1_700_000_000), "end_time": int64(1_700_003_600), "timezone": "Asia/Shanghai",
			"visibility": "private", "free_busy_status": "busy", "need_notification": false,
			"location_name": "Room", "location_address": "Floor 1",
		},
	})
	if err != nil || result == nil || result.ProviderRequestID != "create-event-log" || result.ResultCount != 1 || result.AttemptCount != 2 || calls.Load() != 2 {
		t.Fatalf("result = %#v, calls = %d, err = %v", result, calls.Load(), err)
	}
	if result.Output["event"].(map[string]interface{})["event_id"] != "event-1" {
		t.Fatalf("output = %#v", result.Output)
	}
}

func TestCreateCalendarEventRejectsMissingMarkerAndWhitespaceTitle(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	adapter := newTestAdapter(t, server)
	baseInput := map[string]interface{}{
		"calendar_id": "feishu.cn_team@group.calendar.feishu.cn", "summary": "Planning",
		"start_time": int64(1_700_000_000), "end_time": int64(1_700_003_600),
	}
	_, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: ActionCreateEvent, Connection: testUserConnection(), Input: baseInput,
	})
	if integrations.ErrorCode(err) != integrations.ErrorCodeInvalidInput {
		t.Fatalf("missing marker err = %v", err)
	}
	baseInput["summary"] = "   "
	_, err = adapter.Execute(context.Background(), integrations.ActionRequest{
		MessageID: "message-1", ActionID: ActionCreateEvent, Connection: testUserConnection(), Input: baseInput,
	})
	if integrations.ErrorCode(err) != integrations.ErrorCodeInvalidInput || calls.Load() != 0 {
		t.Fatalf("whitespace title err = %v, calls = %d", err, calls.Load())
	}
	baseInput["summary"] = "Planning"
	baseInput["timezone"] = "Asia//Shanghai"
	_, err = adapter.Execute(context.Background(), integrations.ActionRequest{
		MessageID: "message-1", ActionID: ActionCreateEvent, Connection: testUserConnection(), Input: baseInput,
	})
	if integrations.ErrorCode(err) != integrations.ErrorCodeInvalidInput || calls.Load() != 0 {
		t.Fatalf("timezone err = %v, calls = %d", err, calls.Load())
	}
}

func TestExpandedActionsMapProviderAccessErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-Tt-Logid", "access-log")
		writer.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(writer, `{"code":99991661,"msg":"denied"}`)
	}))
	defer server.Close()
	adapter := newTestAdapter(t, server)
	_, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: ActionListMessages, Connection: testUserConnection(), Input: map[string]interface{}{"chat_id": "oc_team"},
	})
	if integrations.ErrorCode(err) != integrations.ErrorCodeAccessDenied {
		t.Fatalf("err = %v", err)
	}
	diagnostics := integrations.ProviderDiagnosticsFromError(err)
	if diagnostics.RequestID != "access-log" || diagnostics.HTTPStatus != http.StatusForbidden {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
}

func newTestAdapter(t *testing.T, server *httptest.Server) *Adapter {
	t.Helper()
	adapter, err := newForBaseURLs(server.Client(), server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func testUserConnection() *integrations.ResolvedConnection {
	return &integrations.ResolvedConnection{
		IntegrationID: IntegrationID, DriverID: DriverID, AuthMethodID: UserOAuthAuthMethodID,
		Credentials: map[string]string{"access_token": "user-token"}, Config: map[string]interface{}{"region": RegionCN},
	}
}

func mustJSON(t *testing.T, value interface{}) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
