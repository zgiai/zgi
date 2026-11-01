package feishu

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
)

type renderedCard struct {
	Config struct {
		UpdateMulti bool `json:"update_multi"`
	} `json:"config"`
	Header struct {
		Template string `json:"template"`
		Title    struct {
			Content string `json:"content"`
		} `json:"title"`
	} `json:"header"`
	Elements []struct {
		Tag     string `json:"tag"`
		Content string `json:"content"`
		Text    struct {
			Tag     string `json:"tag"`
			Content string `json:"content"`
		} `json:"text"`
		Fields []struct {
			Text struct {
				Tag     string `json:"tag"`
				Content string `json:"content"`
			} `json:"text"`
		} `json:"fields"`
		Actions []struct {
			Type    string            `json:"type"`
			Value   map[string]string `json:"value"`
			Confirm json.RawMessage   `json:"confirm"`
		} `json:"actions"`
	} `json:"elements"`
}

func approvalCardRequest() channelruntime.DeliveryRequest {
	return channelruntime.DeliveryRequest{
		DeliveryID: uuid.New(), ExternalConversationID: "oc_chat", Text: "需要你确认。",
		Actions: []channelruntime.DeliveryAction{
			{Label: "确认", Reply: "确认", Wait: "wait-1", Primary: true},
			{Label: "取消", Reply: "取消", Wait: "wait-1"},
		},
		Card: &channelruntime.DeliveryCard{
			Title: "飞书 / Lark · 发送文本消息", HighRisk: true, Summary: "发送文本消息。",
			Fields: []channelruntime.DeliveryCardField{
				{Label: "使用账号", Value: "feishu:cli_bot"},
				{Label: "内容", Value: "**不要渲染** <at id=all></at>", Truncated: true},
			},
			State: channelruntime.DeliveryCardPending,
		},
	}
}

func decodeCard(t *testing.T, content string) renderedCard {
	t.Helper()
	var card renderedCard
	if err := json.Unmarshal([]byte(content), &card); err != nil {
		t.Fatalf("card content = %s, %v", content, err)
	}
	return card
}

func TestStructuredApprovalCardShowsTheOperationAndAsksOnceMoreWhenHighRisk(t *testing.T) {
	body, ok := buildChannelCard(approvalCardRequest())
	if !ok || body.MsgType != "interactive" {
		t.Fatalf("card = %#v, %v", body, ok)
	}
	card := decodeCard(t, body.Content)
	if !card.Config.UpdateMulti || card.Header.Template != "red" || card.Header.Title.Content != "需要你确认｜飞书 / Lark · 发送文本消息" {
		t.Fatalf("header = %#v, config = %#v", card.Header, card.Config)
	}
	var values []string
	var buttons int
	for _, element := range card.Elements {
		for _, field := range element.Fields {
			if field.Text.Tag != "plain_text" {
				t.Fatalf("a detail is rendered as %q, want plain text", field.Text.Tag)
			}
			values = append(values, field.Text.Content)
		}
		for _, action := range element.Actions {
			buttons++
			if action.Value[feishuCardReplyKey] == "确认" && (action.Type != "danger" || len(action.Confirm) == 0) {
				t.Fatalf("high-risk confirm button = %#v", action)
			}
		}
	}
	joined := strings.Join(values, "\n")
	if !strings.Contains(joined, "使用账号\nfeishu:cli_bot") || !strings.Contains(joined, "内容（已截断）\n**不要渲染** <at id=all></at>") {
		t.Fatalf("details = %q", joined)
	}
	if buttons != 2 {
		t.Fatalf("buttons = %d, want 2", buttons)
	}
}

func TestDecidedApprovalCardShowsItsOutcomeWithoutButtons(t *testing.T) {
	request := approvalCardRequest()
	request.Card.State, request.Card.Outcome = channelruntime.DeliveryCardCompleted, "已确认并执行完成。"
	body, ok := buildChannelCard(request)
	if !ok {
		t.Fatal("decided card was not rendered")
	}
	card := decodeCard(t, body.Content)
	if card.Header.Template != "green" || !strings.HasPrefix(card.Header.Title.Content, "已执行｜") {
		t.Fatalf("header = %#v", card.Header)
	}
	found := false
	for _, element := range card.Elements {
		if len(element.Actions) != 0 {
			t.Fatal("a decided card still has buttons")
		}
		if element.Text.Content == "已确认并执行完成。" {
			found = true
		}
	}
	if !found {
		t.Fatalf("outcome missing: %s", body.Content)
	}
}

// A tap is a received reply, not a decision: another member of a group may
// be refused, and the decision may still be pending. The card and its buttons
// stay until the recorded decision edits them.
func TestTappingACardKeepsItUntilTheDecisionIsRecorded(t *testing.T) {
	identity := channelruntime.InstallationIdentity{ExternalTenantID: "tenant-1"}
	event := &callback.CardActionTriggerEvent{
		EventV2Base: &larkevent.EventV2Base{Header: &larkevent.EventHeader{EventID: "evt-1", AppID: "cli_app", TenantKey: "tenant-1"}},
		Event: &callback.CardActionTriggerRequest{
			Operator: &callback.Operator{OpenID: "ou_member"},
			Action:   &callback.CallBackAction{Value: map[string]interface{}{feishuCardReplyKey: "取消", feishuCardWaitKey: "wait-1", feishuCardTypeKey: "group"}},
			Context:  &callback.Context{OpenChatID: "oc_chat", OpenMessageID: "om_card"},
		},
	}
	receiver := &WebSocketReceiver{}
	accept := func(_ context.Context, _ *channelruntime.InboundMessage) error { return nil }
	response, err := receiver.handleCardAction(t.Context(), identity, "cli_app", event, accept)
	if err != nil || response.Card != nil || response.Toast == nil || response.Toast.Type != "success" {
		t.Fatalf("a received tap replaced the card: %#v, %v", response, err)
	}
	failing := func(_ context.Context, _ *channelruntime.InboundMessage) error { return io.ErrUnexpectedEOF }
	failed, _ := receiver.handleCardAction(t.Context(), identity, "cli_app", event, failing)
	if failed.Card != nil || failed.Toast == nil || failed.Toast.Type != "error" {
		t.Fatalf("a failed submit replaced the card: %#v", failed)
	}
}

func TestEditPatchesTheDeliveredCard(t *testing.T) {
	var patched string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = writer.Write([]byte(`{"code":0,"msg":"success","tenant_access_token":"tenant-token","expire":7200}`))
		case "/open-apis/im/v1/messages/om_card":
			if request.Method != http.MethodPatch {
				t.Errorf("method = %s", request.Method)
			}
			body, _ := io.ReadAll(request.Body)
			var envelope struct {
				Content string `json:"content"`
			}
			_ = json.Unmarshal(body, &envelope)
			patched = envelope.Content
			_, _ = writer.Write([]byte(`{"code":0,"msg":"success","data":{}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	connectionID := uuid.New()
	adapter, err := NewDirectChannelAdapter(newTestAdapter(t, server), &channelConnectionResolverStub{connection: directChannelConnection(connectionID)})
	if err != nil {
		t.Fatal(err)
	}
	if !adapter.Capabilities().Edit {
		t.Fatal("a direct Feishu adapter cannot edit")
	}
	request := approvalCardRequest()
	request.OrganizationID, request.WorkspaceID, request.InstallationID = uuid.New(), uuid.New(), connectionID
	request.Subject = runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman, ID: uuid.New()}
	request.Card.State, request.Card.Outcome = channelruntime.DeliveryCardCancelled, "已取消，这个操作没有执行。"
	receipt, err := adapter.Edit(t.Context(), request, "om_card")
	if err != nil || receipt.ProviderMessageID != "om_card" {
		t.Fatalf("Edit() = %#v, %v", receipt, err)
	}
	if card := decodeCard(t, patched); !strings.HasPrefix(card.Header.Title.Content, "已取消｜") {
		t.Fatalf("patched card = %s", patched)
	}
	request.Card = nil
	if _, err := adapter.Edit(t.Context(), request, "om_card"); err == nil {
		t.Fatal("a prompt without a card was edited")
	}
}
