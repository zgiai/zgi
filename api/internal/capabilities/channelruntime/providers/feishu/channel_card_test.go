package feishu

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
)

func TestApprovalPromptIsACardWithReplyButtons(t *testing.T) {
	request := channelruntime.DeliveryRequest{
		DeliveryID: uuid.New(), ExternalConversationID: "oc_chat", Text: "需要你确认任务「晨报」。<at user_id=\"ou_other\">x</at>",
		Mentions: []channelruntime.DeliveryMention{{ExternalID: "ou_asker", DisplayName: "康源"}},
		Actions: []channelruntime.DeliveryAction{
			{Label: "确认", Reply: "确认", Wait: "wait-1", Primary: true, ConversationType: channelruntime.ConversationTypeGroup},
			{Label: "取消", Reply: "取消", Wait: "wait-1", ConversationType: channelruntime.ConversationTypeGroup},
			{Label: "删库", Reply: "删库", Wait: "wait-1"},
		},
	}
	body, ok := buildChannelCard(request)
	if !ok || body.MsgType != "interactive" {
		t.Fatalf("card = %#v, %v", body, ok)
	}
	var card struct {
		Elements []struct {
			Tag     string `json:"tag"`
			Content string `json:"content"`
			Actions []struct {
				Type  string            `json:"type"`
				Value map[string]string `json:"value"`
			} `json:"actions"`
		} `json:"elements"`
	}
	if err := json.Unmarshal([]byte(body.Content), &card); err != nil || len(card.Elements) != 2 {
		t.Fatalf("card content = %s, %v", body.Content, err)
	}
	if text := card.Elements[0].Content; !strings.HasPrefix(text, "<at id=ou_asker></at> ") || strings.Contains(text, "<at user_id") {
		t.Fatalf("card text = %q", text)
	}
	buttons := card.Elements[1].Actions
	if len(buttons) != 2 || buttons[0].Type != "primary" || buttons[0].Value[feishuCardReplyKey] != "确认" ||
		buttons[0].Value[feishuCardWaitKey] != "wait-1" || buttons[1].Value[feishuCardTypeKey] != "group" {
		t.Fatalf("buttons = %#v", buttons)
	}
	request.Actions = nil
	if _, ok := buildChannelCard(request); ok {
		t.Fatal("a message without actions became a card")
	}
}

func TestTappingACardButtonRepliesInItsConversation(t *testing.T) {
	identity := channelruntime.InstallationIdentity{ExternalTenantID: "tenant-1", ExternalSelfID: "ou_bot"}
	event := func(appID, tenant, reply string) *callback.CardActionTriggerEvent {
		return &callback.CardActionTriggerEvent{
			EventV2Base: &larkevent.EventV2Base{Header: &larkevent.EventHeader{EventID: "evt-1", AppID: appID, TenantKey: tenant}},
			Event: &callback.CardActionTriggerRequest{
				Operator: &callback.Operator{OpenID: "ou_asker"},
				Action:   &callback.CallBackAction{Value: map[string]interface{}{feishuCardReplyKey: reply, feishuCardWaitKey: "wait-1", feishuCardTypeKey: "group"}},
				Context:  &callback.Context{OpenChatID: "oc_chat", OpenMessageID: "om_card"},
			},
		}
	}
	now := time.Now().UTC()
	message, ok := cardActionMessage(identity, "cli_app", event("cli_app", "tenant-1", "确认"), now)
	if !ok || message.Text != "确认" || message.ActionWait != "wait-1" || message.SenderExternalID != "ou_asker" ||
		message.ExternalConversationID != "oc_chat" || message.ConversationType != channelruntime.ConversationTypeGroup ||
		message.WakeReason != channelruntime.WakeReasonReplyToSelf || message.ProviderMessageID != "card_action:evt-1" {
		t.Fatalf("tap = %#v, %v", message, ok)
	}
	for name, other := range map[string]*callback.CardActionTriggerEvent{
		"another app":    event("cli_other", "tenant-1", "确认"),
		"another tenant": event("cli_app", "tenant-2", "确认"),
		"another word":   event("cli_app", "tenant-1", "删库"),
	} {
		if _, ok := cardActionMessage(identity, "cli_app", other, now); ok {
			t.Fatalf("%s was accepted", name)
		}
	}
}
