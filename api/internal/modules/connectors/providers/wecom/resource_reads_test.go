package wecom

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
)

func readInput() providers.CredentialInput {
	return providers.CredentialInput{Values: map[string]string{"bot_id": "fixture-bot", "secret": "fixture-secret"}, ResourceScope: providers.ResourceScope{OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ConnectionID: uuid.New(), CredentialVersion: 1, BindingID: uuid.New(), BindingRevision: 1, Target: core.TargetRef{Type: core.TargetTypeChat, ID: uuid.NewString()}, LogicalOperationID: uuid.NewString()}}
}
func firstResource(t *testing.T, result providers.Response) string {
	t.Helper()
	return result.Value["data"].(map[string]any)["items"].([]any)[0].(map[string]any)["resource_handle"].(string)
}

func TestSevenResourceReadsUseScopedHandlesAndOfficialParameters(t *testing.T) {
	requests := 0
	c, err := NewWithHandleKey(&http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		requests++
		if strings.Contains(r.URL.Path, "get_cli_config") {
			return response(map[string]any{"token": "derived-fixture"}), nil
		}
		var wire map[string]string
		if json.NewDecoder(r.Body).Decode(&wire) != nil || len(wire) != 1 {
			t.Fatal("business wrapper")
		}
		var body map[string]any
		if json.Unmarshal([]byte(wire["payload"]), &body) != nil {
			t.Fatal("payload JSON")
		}
		var data map[string]any
		switch r.URL.Path {
		case "/cli/doc/search":
			if len(stringsInput(body["keywords"])) != 1 {
				t.Fatal(body)
			}
			data = map[string]any{"docs": []any{map[string]any{"docid": "doc-123", "doc_name": "文档"}}, "has_more": true, "next_cursor": "private-cursor"}
		case "/cli/doc/contents/get":
			if body["docid"] != "doc-123" || body["content_type"] != "markdown" {
				t.Fatal(body)
			}
			data = map[string]any{"file_path": map[string]any{"file_name": "ignored.md", "content": "# 文档正文"}, "name": "文档"}
		case "/cli/calendar/schedules/list":
			if len(body) != 2 || body["begin_time"] == nil || body["end_time"] == nil {
				t.Fatal("invented pagination", body)
			}
			data = map[string]any{"schedule_list": []any{map[string]any{"schedule_id": "schedule1", "subject": "日程", "timezone": map[string]any{"timezone_id": "Asia/Shanghai", "timezone_offset": 28800}}}}
		case "/cli/calendar/schedules/get":
			if len(body) != 1 || stringsInput(body["schedule_ids"])[0] != "schedule1" {
				t.Fatal(body)
			}
			data = map[string]any{"schedule_list_count": 0}
		case "/cli/calendar/schedules/search":
			if body["organizer"] != "user1" {
				t.Fatal(body)
			}
			data = map[string]any{"schedules": []any{map[string]any{"schedule_id": "schedule1", "subject": "日程"}}}
		case "/cli/todo/list":
			data = map[string]any{"items": []any{map[string]any{"todo_id": "todo1", "title": "待办", "deadline": nil}}}
		case "/cli/todo/get":
			items := body["items"].([]any)
			if len(items) != 1 || items[0].(map[string]any)["todo_id"] != "todo1" {
				t.Fatal(body)
			}
			data = map[string]any{"items": []any{map[string]any{"todo_id": "todo1", "success": false, "errmsg": "private upstream details"}}}
		default:
			t.Fatal(r.URL.Path)
		}
		return response(envelope(data)), nil
	})}, true, nil, strings.Repeat("k", 32))
	if err != nil {
		t.Fatal(err)
	}
	in := readInput()
	docs, err := c.InvokeCredentials(context.Background(), "wecom.documents.search", in, map[string]any{"keywords": []any{"主题"}})
	if err != nil {
		t.Fatal(err)
	}
	doc := firstResource(t, docs)
	content, err := c.InvokeCredentials(context.Background(), "wecom.documents.get", in, map[string]any{"resource_handle": doc})
	if err != nil || content.Value["data"].(map[string]any)["content"] != "# 文档正文" {
		t.Fatal(err)
	}
	schedules, err := c.InvokeCredentials(context.Background(), "wecom.calendar.list", in, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	agenda, err := c.InvokeCredentials(context.Background(), "wecom.calendar.get", in, map[string]any{"resource_handles": []any{firstResource(t, schedules)}})
	if err != nil || len(agenda.Value["data"].(map[string]any)["not_returned"].([]string)) != 1 {
		t.Fatal(err)
	}
	if _, err = c.InvokeCredentials(context.Background(), "wecom.calendar.search", in, map[string]any{"organizer": "user1"}); err != nil {
		t.Fatal(err)
	}
	todos, err := c.InvokeCredentials(context.Background(), "wecom.todos.list", in, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.InvokeCredentials(context.Background(), "wecom.todos.get", in, map[string]any{"resource_handles": []any{firstResource(t, todos)}})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result.Value)
	if strings.Contains(string(raw), "private") || result.Value["data"].(map[string]any)["items"].([]any)[0].(map[string]any)["success"] != false {
		t.Fatal("partial result masked or raw error leaked")
	}
	before := requests
	for _, change := range []func(*providers.CredentialInput){func(x *providers.CredentialInput) { x.ResourceScope.OwnerAccountID = uuid.New() }, func(x *providers.CredentialInput) { x.ResourceScope.OrganizationID = uuid.New() }, func(x *providers.CredentialInput) { x.ResourceScope.ConnectionID = uuid.New() }, func(x *providers.CredentialInput) { x.ResourceScope.CredentialVersion++ }, func(x *providers.CredentialInput) { x.ResourceScope.Target.ID = uuid.NewString() }} {
		other := in
		change(&other)
		if _, err = c.openResource(other, doc, "document", ""); err == nil {
			t.Fatal("scope crossed")
		}
	}
	if requests != before {
		t.Fatal("handle validation contacted provider")
	}
	cursor := docs.Value["data"].(map[string]any)["next_cursor"].(string)
	if _, err = c.InvokeCredentials(context.Background(), "wecom.documents.search", in, map[string]any{"keywords": []any{"changed"}, "cursor": cursor}); err == nil {
		t.Fatal("cursor changed query")
	}
}

func TestDocumentLongTaskWireModes(t *testing.T) {
	for _, mode := range []int{0, 1} {
		t.Run(string(rune('0'+mode)), func(t *testing.T) {
			calls := 0
			c, _ := NewWithHandleKey(&http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
				calls++
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if calls == 1 {
					return response(map[string]any{"results_json": `{"taskid":"private-task","poll_mode":` + string(rune('0'+mode)) + `}`}), nil
				}
				if r.Header.Get("Range") != "" {
					t.Fatal("range header")
				}
				if mode == 0 {
					if r.URL.Path != "/cli/task/query" || body["method"] != "PollClawLongTask" || body["payload"] != `{"taskid":"private-task"}` {
						t.Fatal("wrong mode 0 envelope", body)
					}
				} else {
					if r.URL.Path != "/cli/doc/contents/get" || len(body) != 0 || r.Header.Get("X-Long-Poll-TaskId") != "private-task" {
						t.Fatal("wrong mode 1 envelope")
					}
				}
				inner, _ := json.Marshal(map[string]any{"result": `{"content":"done"}`, "long_task_poll": map[string]any{"done": true}})
				return response(map[string]any{"results_json": string(inner)}), nil
			})}, true, nil, strings.Repeat("k", 32))
			token := "fixture"
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			result, err := c.readBusiness(ctx, "/doc/contents/get", readInput(), &token, map[string]any{"docid": "doc-123"})
			if err != nil || result["content"] != "done" || calls != 2 {
				t.Fatal(result, err, calls)
			}
		})
	}
}

func TestDocumentContentIsInMemoryAndBounded(t *testing.T) {
	result, err := documentContent(map[string]any{"file_path": "C:/not-a-real-file"}, "text")
	if err != nil || result["content"] != "C:/not-a-real-file" {
		t.Fatal("interpreted content as path")
	}
	result, err = documentContent(map[string]any{"content": strings.Repeat("中", 100000)}, "markdown")
	if err != nil || result["truncated"] != true || len(result["content"].(string)) > 256<<10 {
		t.Fatal("text bound")
	}
	if _, err = documentContent(map[string]any{"file_path": map[string]any{"content": "encoded", "content_encoding": "unsupported"}}, "text"); err == nil {
		t.Fatal("unknown encoding accepted")
	}
	bad := `{"errcode":1,"content":"not successful"}`
	if _, err = decodeBusiness(cliEnvelope{Result: &bad}); err == nil {
		t.Fatal("business error ignored")
	}
}

func TestLongTaskCancellationAndMalformedPollNeverRestart(t *testing.T) {
	for _, scenario := range []string{"cancel", "missing_done", "changed_task", "business_error"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			c, _ := NewWithHandleKey(&http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					if scenario == "cancel" {
						cancel()
					}
					return response(map[string]any{"results_json": `{"taskid":"task-1","poll_mode":0}`}), nil
				}
				if r.URL.Path != "/cli/task/query" {
					t.Fatal("initial document request repeated")
				}
				inner := `{"result":"{}","long_task_poll":{}}`
				if scenario == "changed_task" {
					inner = `{"taskid":"other","result":"{}","long_task_poll":{"done":true}}`
				}
				if scenario == "business_error" {
					inner = `{"result":"{\"errcode\":99}","long_task_poll":{"done":true}}`
				}
				return response(map[string]any{"results_json": inner}), nil
			})}, true, nil, strings.Repeat("k", 32))
			token := "fixture"
			_, err := c.readBusiness(ctx, "/doc/contents/get", readInput(), &token, map[string]any{"docid": "doc"})
			if err == nil || calls > 2 {
				t.Fatal("unsafe long task result", err, calls)
			}
			if scenario == "cancel" && calls != 1 {
				t.Fatal("polled after cancellation")
			}
		})
	}
}
