package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Explicit protocol fixtures, independent of operationRequest's dispatch map.
func TestAllJSONOperationsUseFixedRoutesAndSchemas(t *testing.T) {
	cases := []struct{ id, method, path, args, bodyContains string }{
		{"bot.get", "GET", "/bot/v3/info", `{}`, ""},
		{"tenant.get", "GET", "/tenant/v2/tenant/query", `{}`, ""},
		{"chat.list", "GET", "/im/v1/chats", `{"page_size":2}`, ""},
		{"chat.get", "GET", "/im/v1/chats/oc_chat", `{"chat_id":"oc_chat"}`, ""},
		{"chat.members.list", "GET", "/im/v1/chats/oc_chat/members", `{"chat_id":"oc_chat"}`, ""},
		{"message.list", "GET", "/im/v1/messages", `{"chat_id":"oc_chat"}`, ""},
		{"message.get", "GET", "/im/v1/messages/om_message", `{"message_id":"om_message"}`, ""},
		{"document.read", "GET", "/docx/v1/documents/doc_id/raw_content", `{"document_id":"doc_id"}`, ""},
		{"document.blocks.list", "GET", "/docx/v1/documents/doc_id/blocks", `{"document_id":"doc_id"}`, ""},
		{"file.metadata.get", "POST", "/drive/v1/metas/batch_query", `{"file_token":"doc_id","file_type":"docx"}`, `"request_docs":[{"doc_token":"doc_id","doc_type":"docx"}]`},
		{"wiki.node.get", "GET", "/wiki/v2/spaces/get_node", `{"token":"wik_id"}`, ""},
		{"comment.list", "GET", "/drive/v1/files/doc_id/comments", `{"file_token":"doc_id"}`, ""},
		{"message.send", "POST", "/im/v1/messages", `{"receive_id":"ou_user","receive_id_type":"open_id","text":"hello"}`, `"msg_type":"text"`},
		{"message.reply", "POST", "/im/v1/messages/om_message/reply", `{"message_id":"om_message","text":"hello"}`, `"msg_type":"text"`},
		{"document.blocks.append", "POST", "/docx/v1/documents/doc_id/blocks/blk_id/children", `{"document_id":"doc_id","block_id":"blk_id","text":"hello"}`, `"block_type":2`},
		{"document.blocks.update", "PATCH", "/docx/v1/documents/doc_id/blocks/blk_id", `{"document_id":"doc_id","block_id":"blk_id","text":"hello"}`, `"update_text_elements"`},
		{"comment.create", "POST", "/drive/v1/files/doc_id/comments", `{"file_token":"doc_id","text":"hello"}`, `"is_whole":true`},
		{"comment.reply", "POST", "/drive/v1/files/doc_id/comments/com_id/replies", `{"file_token":"doc_id","comment_id":"com_id","text":"hello"}`, `"text_run":{"text":"hello"}`},
		{"message.update", "PUT", "/im/v1/messages/om_message", `{"message_id":"om_message","text":"hello"}`, `"msg_type":"text"`},
		{"comment.update", "PUT", "/drive/v1/files/doc_id/comments/com_id/replies/rep_id", `{"file_token":"doc_id","comment_id":"com_id","reply_id":"rep_id","text":"hello"}`, `"text_run":{"text":"hello"}`},
		{"comment.delete", "DELETE", "/drive/v1/files/doc_id/comments/com_id/replies/rep_id", `{"file_token":"doc_id","comment_id":"com_id","reply_id":"rep_id"}`, ""},
		{"reaction.list", "GET", "/im/v1/messages/om_message/reactions", `{"message_id":"om_message"}`, ""},
		{"reaction.add", "POST", "/im/v1/messages/om_message/reactions", `{"message_id":"om_message","emoji_type":"SMILE"}`, `"emoji_type":"SMILE"`},
		{"reaction.remove", "DELETE", "/im/v1/messages/om_message/reactions/rea_id", `{"message_id":"om_message","reaction_id":"rea_id"}`, ""},
		{"pin.list", "GET", "/im/v1/pins", `{"chat_id":"oc_chat"}`, ""},
		{"pin.add", "POST", "/im/v1/pins", `{"message_id":"om_message"}`, `"message_id":"om_message"`},
		{"pin.remove", "DELETE", "/im/v1/pins/om_message", `{"message_id":"om_message"}`, ""},
		{"chat.create", "POST", "/im/v1/chats", `{"name":"Fixture","user_id_list":["ou_user"]}`, `"user_id_list":["ou_user"]`},
		{"chat.update", "PUT", "/im/v1/chats/oc_chat", `{"chat_id":"oc_chat","name":"Fixture"}`, `"name":"Fixture"`},
		{"card.create", "POST", "/cardkit/v1/cards", `{"text":"hello"}`, `"type":"card_json"`},
		{"card.update", "PUT", "/cardkit/v1/cards/card_id", `{"card_id":"card_id","sequence":1,"text":"hello"}`, `"sequence":1`},
		{"document.blocks.convert", "POST", "/docx/v1/documents/blocks/convert", `{"text":"hello"}`, `"content_type":"markdown"`},
		{"contact.basic.get", "GET", "/contact/v3/users/ou_user", `{"user_id":"ou_user"}`, ""},
		{"contact.search", "POST", "/directory/v1/employees/search", `{"query":"张三"}`, `"required_fields":["base_info.employee_id","base_info.name.name"]`},
		{"message.batch_send", "POST", "/message/v4/batch_send/", `{"open_ids":["ou_one","ou_two"],"text":"hello"}`, `"content":{"text":"hello"}`},
		{"message.system_send", "POST", "/im/v1/messages", `{"open_id":"ou_user","text":"新会话"}`, `"msg_type":"system"`},
	}
	definition, err := catalog.FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	if len(definition.Operations) != len(cases)+2 {
		t.Fatal("new operation missing protocol fixture")
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			calls := 0
			client, err := New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return fixtureResponse(`{"code":0,"tenant_access_token":"token-fixture","expire":7200}`), nil
				}
				if r.Method != tc.method || r.URL.Path != "/open-apis"+tc.path || r.URL.Host != "open.larksuite.com" {
					t.Fatalf("route: %s %s", r.Method, r.URL)
				}
				if r.Header.Get("Authorization") != "Bearer token-fixture" {
					t.Fatal("missing app token")
				}
				body, _ := io.ReadAll(r.Body)
				if tc.bodyContains != "" && !strings.Contains(string(body), tc.bodyContains) {
					t.Fatalf("payload %s", body)
				}
				switch tc.id {
				case "tenant.get":
					return fixtureResponse(tenantFixture), nil
				case "chat.members.list":
					return fixtureResponse(memberPageFixture(0, false, "")), nil
				case "contact.search":
					return fixtureResponse(`{"code":0,"data":{"employees":[],"page_response":{"has_more":false}}}`), nil
				case "message.list":
					if r.URL.Query().Get("container_id_type") != "chat" || r.URL.Query().Get("container_id") != "oc_chat" {
						t.Fatal("message scope")
					}
				case "wiki.node.get":
					if r.URL.Query().Get("token") != "wik_id" {
						t.Fatal("wiki scope")
					}
				case "message.system_send":
					if r.URL.Query().Get("receive_id_type") != "open_id" {
						t.Fatal("system recipient")
					}
				case "chat.list":
					if r.URL.Query().Get("page_size") != "2" {
						t.Fatal("pagination")
					}
				}
				return fixtureResponse(`{"code":0,"data":{"result":"safe","user":{"open_id":"ou_user","name":"Name","email":"private@example.invalid","mobile":"private"}},"bot":{"open_id":"ou_bot"}}`), nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			var args map[string]any
			_ = json.Unmarshal([]byte(tc.args), &args)
			input := fixtureInput()
			input.Values["region"] = "lark"
			response, err := client.InvokeCredentials(context.Background(), "feishu."+tc.id, input, args)
			if err != nil || calls != 2 {
				t.Fatalf("invoke %v (%d calls)", err, calls)
			}
			if client.schemas.Output("feishu."+tc.id, response.Value) != nil {
				t.Fatal("invalid output")
			}
			if tc.id == "contact.basic.get" {
				raw, _ := json.Marshal(response.Value)
				if strings.Contains(string(raw), "private") {
					t.Fatal("sensitive contact fields exposed")
				}
			}
			args["unexpected"] = "https://evil.invalid"
			before := calls
			if _, err := client.InvokeCredentials(context.Background(), "feishu."+tc.id, input, args); err == nil || calls != before {
				t.Fatal("unknown schema field reached network")
			}
		})
	}
}

func TestFeishuPermissionGroupsAreANDOfAlternatives(t *testing.T) {
	d, err := catalog.FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range d.Operations {
		t.Run(op.ID, func(t *testing.T) {
			granted := map[string]bool{}
			if scopesAllow(op.RequiredScopes, granted) {
				t.Fatal("empty grants authorize")
			}
			for _, group := range op.RequiredScopes {
				granted[group[0]] = true
			}
			if !scopesAllow(op.RequiredScopes, granted) {
				t.Fatal("required groups not accepted")
			}
			for i := range op.RequiredScopes {
				partial := map[string]bool{}
				for j, group := range op.RequiredScopes {
					if i != j {
						partial[group[0]] = true
					}
				}
				if scopesAllow(op.RequiredScopes, partial) {
					t.Fatal("missing AND group allowed")
				}
			}
		})
	}
}
func TestFeishuWriteUnknownIsNeverRetriedAndRedirectsAreRejected(t *testing.T) {
	for _, mode := range []string{"network", "redirect", "oversize", "malformed", "provider_code"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			client, _ := New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return fixtureResponse(`{"code":0,"tenant_access_token":"token-fixture","expire":7200}`), nil
				}
				switch mode {
				case "provider_code":
					return fixtureResponse(`{"code":987654321,"msg":"fixture-secret-not-real"}`), nil
				case "network":
					return nil, errors.New("fixture-secret-not-real")
				case "redirect":
					res := fixtureResponse(`{}`)
					res.StatusCode = 302
					res.Header.Set("Location", "https://evil.invalid")
					return res, nil
				case "oversize":
					return fixtureResponse(strings.Repeat("x", maxResponseBytes+1)), nil
				default:
					return fixtureResponse("null"), nil
				}
			})})
			_, err := client.InvokeCredentials(context.Background(), "feishu.message.send", fixtureInput(), map[string]any{"receive_id": "ou_user", "receive_id_type": "open_id", "text": "hello"})
			failure := providers.FailureOf(err)
			if err == nil || !failure.Unknown || calls != 2 || strings.Contains(err.Error(), "fixture-secret") {
				t.Fatalf("unknown/redaction %v %d", err, calls)
			}
		})
	}
}
func TestFeishuResponseBoundsAndSecretFiltering(t *testing.T) {
	client, _ := New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "tenant_access_token") {
			return fixtureResponse(`{"code":0,"tenant_access_token":"token-fixture","expire":7200}`), nil
		}
		raw, _ := json.Marshal(map[string]any{"code": 0, "data": map[string]any{"text": strings.Repeat("x", 18000), "app_secret": "hidden", "nested": map[string]any{"echo": "token-fixture fixture-secret-not-real"}}})
		return fixtureResponse(string(raw)), nil
	})})
	res, err := client.InvokeCredentials(context.Background(), "feishu.document.read", fixtureInput(), map[string]any{"document_id": "doc_id"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(res.Value)
	if res.Value["truncated"] != true || strings.Contains(string(raw), "token-fixture") || strings.Contains(string(raw), "fixture-secret-not-real") || strings.Contains(string(raw), "hidden") {
		t.Fatal("bounded redaction failed")
	}
}
func TestBatchSchemaRejectsDuplicatesAndUnboundedRecipients(t *testing.T) {
	client, _ := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid recipients reached provider")
		return nil, nil
	})})
	for _, ids := range [][]any{{"ou_a", "ou_a"}, make([]any, 21)} {
		_, err := client.InvokeCredentials(context.Background(), "feishu.message.batch_send", fixtureInput(), map[string]any{"open_ids": ids, "text": "hello"})
		if err == nil || providers.FailureOf(err).Code != core.ErrorCodeInvalidInput {
			t.Fatal(err)
		}
	}
}
