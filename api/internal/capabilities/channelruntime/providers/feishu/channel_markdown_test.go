package feishu

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
)

func markdownDeliveryRequest() channelruntime.DeliveryRequest {
	return channelruntime.DeliveryRequest{
		OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ActorID: uuid.New(), InstallationID: uuid.New(),
		ConnectionID: uuid.New(), RunID: uuid.New(), DeliveryID: uuid.New(), ExternalConversationID: "oc_markdown",
		Text: "## 工作结果\n\n**已完成**，使用 `my_var`。\n\n- 第一步\n- [x] 第二步\n\n> 引用\n\n```go\nx := 1 * 2\n```\n\n| 名称 | 状态 |\n| --- | --- |\n| 测试 | 成功 |\n\n[下载报告](https://example.com/report?a=1&b=2)",
	}
}

func TestChannelMarkdownPostAndTrustedMentions(t *testing.T) {
	request := markdownDeliveryRequest()
	request.Mentions = []channelruntime.DeliveryMention{
		{ExternalID: "ou_sender", DisplayName: "康源"},
		{ExternalID: "ou_sender", DisplayName: "重复"},
		{ExternalID: "all", DisplayName: "所有人"},
		{ExternalID: `invalid"id`},
	}
	body, err := buildChannelMessage(request, false)
	if err != nil {
		t.Fatal(err)
	}
	if body.MsgType != "post" || body.ReceiveID != request.ExternalConversationID || body.UUID == "" {
		t.Fatalf("body = %#v", body)
	}
	var content map[string]feishuPost
	if err := json.Unmarshal([]byte(body.Content), &content); err != nil {
		t.Fatal(err)
	}
	rows := content["zh_cn"].Content
	if len(rows) != 2 || len(rows[0]) != 1 || rows[0][0].Tag != "at" || rows[0][0].UserID != "ou_sender" ||
		len(rows[1]) != 1 || rows[1][0].Tag != "md" || rows[1][0].Text != request.Text {
		t.Fatalf("post rows = %#v", rows)
	}
	replayed, _ := buildChannelMessage(request, false)
	if body.UUID != replayed.UUID || body.Content != replayed.Content {
		t.Fatal("repeated preparation must be deterministic")
	}
	request.Mentions = nil
	body, _ = buildChannelMessage(request, false)
	if strings.Contains(body.Content, `"tag":"at"`) {
		t.Fatal("private chat must not add mentions")
	}
}

func TestChannelMarkdownCannotForgeMentions(t *testing.T) {
	request := markdownDeliveryRequest()
	request.Text = `<at user_id="all">everyone</at> <AT user_id="ou_other">other</AT>`
	request.Mentions = []channelruntime.DeliveryMention{{ExternalID: "ou_sender", DisplayName: `<at user_id="all">`}}
	for _, plain := range []bool{false, true} {
		body, err := buildChannelMessage(request, plain)
		if err != nil {
			t.Fatal(err)
		}
		if plain {
			var content struct{ Text string }
			_ = json.Unmarshal([]byte(body.Content), &content)
			if strings.Count(content.Text, `<at user_id=`) != 1 || strings.Contains(content.Text, `<at user_id="all">`) {
				t.Fatalf("forged text mentions: %s", content.Text)
			}
		} else {
			var content map[string]feishuPost
			_ = json.Unmarshal([]byte(body.Content), &content)
			markdown := content["zh_cn"].Content[1][0].Text
			if strings.Contains(strings.ToLower(markdown), "<at") || !strings.Contains(markdown, "&lt;") {
				t.Fatalf("forged Markdown mentions: %s", markdown)
			}
		}
	}
}

func TestChannelMarkdownPlainFallbackPreservesContent(t *testing.T) {
	plain := feishuPlainText(markdownDeliveryRequest().Text)
	for _, want := range []string{"工作结果", "已完成", "my_var", "• 第一步", "☑ 第二步", "引用", "x := 1 * 2", "名称 | 状态", "测试 | 成功", "下载报告 (https://example.com/report?a=1&b=2)"} {
		if !strings.Contains(plain, want) {
			t.Errorf("plain result missing %q: %s", want, plain)
		}
	}
	for _, unwanted := range []string{"##", "**", "```", "[下载报告]"} {
		if strings.Contains(plain, unwanted) {
			t.Errorf("plain result contains formatting %q: %s", unwanted, plain)
		}
	}
}

func TestChannelMarkdownSizeUsesEncodedBytesWithoutTruncation(t *testing.T) {
	request := markdownDeliveryRequest()
	request.Text = strings.Repeat("正文", 9990) + "\n\n📎 报告.html\nhttps://example.com/download?token=a&download=1"
	body, err := buildChannelMessage(request, false)
	if err != nil {
		t.Fatal(err)
	}
	if body.MsgType != "text" {
		t.Fatalf("long Chinese text must respect post byte limit: %s", body.MsgType)
	}
	var content struct{ Text string }
	_ = json.Unmarshal([]byte(body.Content), &content)
	if html.UnescapeString(content.Text) != request.Text {
		t.Fatal("long result or attachment link was truncated")
	}
	wire, _ := json.Marshal(body)
	if len(wire) > maxChannelTextBytes {
		t.Fatalf("text request exceeds byte budget: %d", len(wire))
	}
	request.Text = strings.Repeat("\"\\<>&", 1500)
	body, err = buildChannelMessage(request, false)
	if err != nil {
		t.Fatal(err)
	}
	wire, _ = json.Marshal(body)
	if body.MsgType == "post" && len(wire) > maxChannelPostBytes {
		t.Fatal("escaped JSON was not included in size budget")
	}
	request.Text = strings.Repeat("中", maxChannelTextBytes)
	if _, err := buildChannelMessage(request, false); err == nil {
		t.Fatal("oversized text must fail explicitly, not truncate")
	}
}

func TestChannelMarkdownDeliveryFallbackAndAmbiguity(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		response string
		fallback bool
		unknown  bool
	}{
		{"content rejected", 400, `{"code":230001,"error":{"field_violations":[{"field":"content"}]}}`, true, false},
		{"documented ext rejection", 400, `{"code":230001,"msg":"Your request contains an invalid request parameter, ext=invalid message content. validate content fail"}`, true, false},
		{"content node rejected", 400, `{"code":230001,"error":{"field_violations":[{"field":"content.zh_cn"}]}}`, true, false},
		{"post expanded beyond limit", 400, `{"code":230025}`, true, false},
		{"other invalid argument", 400, `{"code":230001,"error":{"field_violations":[{"field":"receive_id"}]}}`, false, false},
		{"unspecified bad request", 400, `{"code":230001}`, false, false},
		{"missing permission", 400, `{"code":230027}`, false, false},
		{"not in chat", 400, `{"code":230002}`, false, false},
		{"DLP refusal", 400, `{"code":230028}`, false, false},
		{"rate limited", 429, `{"code":230020}`, false, false},
		{"card error is not post error", 400, `{"code":230099}`, false, false},
		{"gateway failure", 502, `bad gateway`, false, true},
		{"invalid successful response", 200, `<html>oops</html>`, false, true},
		{"missing message ID", 200, `{"code":0,"data":{}}`, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sends []channelMessageBody
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
					_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"tenant-token","expire":7200}`))
					return
				}
				var body channelMessageBody
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				sends = append(sends, body)
				if len(sends) == 1 {
					w.WriteHeader(tt.status)
					_, _ = w.Write([]byte(tt.response))
					return
				}
				_, _ = w.Write([]byte(`{"code":0,"data":{"message_id":"om_plain"}}`))
			}))
			defer server.Close()
			request := markdownDeliveryRequest()
			adapter, err := NewDirectChannelAdapter(newTestAdapter(t, server), &channelConnectionResolverStub{connection: directChannelConnection(request.ConnectionID)})
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := adapter.Deliver(t.Context(), request)
			server.Close() // Join handlers before inspecting the recorded requests.
			if tt.fallback {
				if err != nil || receipt == nil || receipt.ProviderMessageID != "om_plain" || len(sends) != 2 || sends[1].MsgType != "text" {
					t.Fatalf("fallback: receipt=%#v, error=%v, sends=%#v", receipt, err, sends)
				}
				if sends[0].UUID == sends[1].UUID {
					t.Fatal("different representations require different provider idempotency keys")
				}
			} else if err == nil || len(sends) != 1 || errors.Is(err, channelruntime.ErrDeliveryOutcomeUnknown) != tt.unknown {
				t.Fatalf("unexpected retry or outcome classification: sends=%d err=%v", len(sends), err)
			}
			if sends[0].MsgType != "post" {
				t.Fatal("normal channel reply must use post")
			}
		})
	}
}

func TestNormalizedPostHistory(t *testing.T) {
	for _, raw := range []string{
		`{"title":"结果","content":[[{"tag":"at","user_id":"ou_sender","user_name":"康源"}],[{"tag":"md","text":"**正文**"}],[{"tag":"a","text":"文件","href":"https://example.com/report"}]]}`,
		`{"zh_cn":{"title":"结果","content":[[{"tag":"at","user_id":"ou_sender","user_name":"康源"}],[{"tag":"md","text":"**正文**"}],[{"tag":"a","text":"文件","href":"https://example.com/report"}]]},"en_us":{"content":[[{"tag":"text","text":"do not duplicate"}]]}}`,
	} {
		if got := normalizedMessageText("post", raw); got != "结果\n@康源 \n**正文**\n文件 (https://example.com/report)" {
			t.Errorf("post history = %q", got)
		}
	}
	if got := normalizedMessageText("post", "not JSON"); got != "" {
		t.Fatalf("invalid post = %q", got)
	}
	raw := fmt.Sprintf(`{"content":[[{"tag":"md","text":%q}]]}`, strings.Repeat("中", 5000))
	if len([]rune(normalizedMessageText("post", raw))) > 4000 {
		t.Fatal("history must remain bounded")
	}
}

func TestChannelMarkdownFallbackFailureDoesNotSendAgain(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		t.Run(fmt.Sprintf("disconnect=%t", disconnect), func(t *testing.T) {
			var sends atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
					_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"tenant-token","expire":7200}`))
					return
				}
				attempt := sends.Add(1)
				if attempt == 2 && disconnect {
					connection, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					_ = connection.Close()
					return
				}
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"code":230025}`))
			}))
			defer server.Close()
			request := markdownDeliveryRequest()
			adapter, err := NewDirectChannelAdapter(newTestAdapter(t, server), &channelConnectionResolverStub{connection: directChannelConnection(request.ConnectionID)})
			if err != nil {
				t.Fatal(err)
			}
			_, err = adapter.Deliver(t.Context(), request)
			if err == nil || sends.Load() != 2 || errors.Is(err, channelruntime.ErrDeliveryOutcomeUnknown) != disconnect {
				t.Fatalf("fallback failure: sends=%d, err=%v", sends.Load(), err)
			}
		})
	}
}
