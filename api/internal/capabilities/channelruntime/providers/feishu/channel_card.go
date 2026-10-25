package feishu

import (
	"context"
	"encoding/json"
	"html"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
)

// A prompt waiting for an approval is sent as an interactive card whose
// buttons send its reply words, and a tap comes back as the card action
// callback over the same long connection. The app must subscribe to the
// card.action.trigger callback; without it the buttons do nothing, and the
// card still says what to reply.

const (
	feishuCardReplyKey = "zgi_reply"
	feishuCardWaitKey  = "zgi_wait"
	feishuCardTypeKey  = "zgi_conversation_type"
	// A card larger than this goes as rich text without buttons.
	maxChannelCardBytes = 28 * 1024
)

// feishuCardReplies are the only words a button may send.
var feishuCardReplies = map[string]bool{"确认": true, "取消": true, "同意": true, "拒绝": true}

// buildChannelCard renders a delivery with actions as an interactive card.
// It reports false when the card cannot carry them, and the delivery goes as
// rich text instead.
func buildChannelCard(request channelruntime.DeliveryRequest) (channelMessageBody, bool) {
	buttons := make([]map[string]any, 0, len(request.Actions))
	for _, action := range request.Actions {
		reply := strings.TrimSpace(action.Reply)
		if !feishuCardReplies[reply] || strings.TrimSpace(action.Wait) == "" {
			continue
		}
		kind := "default"
		if action.Primary {
			kind = "primary"
		}
		buttons = append(buttons, map[string]any{
			"tag": "button", "type": kind,
			"text": map[string]string{"tag": "plain_text", "content": bounded(firstNonEmpty(strings.TrimSpace(action.Label), reply), 20)},
			"value": map[string]string{
				feishuCardReplyKey: reply, feishuCardWaitKey: strings.TrimSpace(action.Wait), feishuCardTypeKey: string(action.ConversationType),
			},
		})
	}
	if len(buttons) == 0 || len(request.Attachments) != 0 {
		return channelMessageBody{}, false
	}
	// Generated text never notifies anyone; only trusted mentions do.
	var content strings.Builder
	for _, mention := range channelPostMentions(request.Mentions) {
		content.WriteString(`<at id=` + mention.UserID + `></at> `)
	}
	content.WriteString(feishuMentionTag.ReplaceAllStringFunc(strings.TrimSpace(request.Text), html.EscapeString))
	card, err := json.Marshal(map[string]any{
		"config":   map[string]any{"wide_screen_mode": true},
		"elements": []map[string]any{{"tag": "markdown", "content": content.String()}, {"tag": "action", "actions": buttons}},
	})
	if err != nil || len(card) > maxChannelCardBytes {
		return channelMessageBody{}, false
	}
	return channelMessageBody{
		ReceiveID: strings.TrimSpace(request.ExternalConversationID), MsgType: "interactive", Content: string(card),
		UUID: uuid.NewSHA1(request.DeliveryID, []byte("feishu:interactive")).String(),
	}, true
}

// cardActionMessage turns a tapped button into the reply it sends, from the
// tapping user, in the card's conversation.
func cardActionMessage(identity channelruntime.InstallationIdentity, appID string, event *callback.CardActionTriggerEvent, now time.Time) (*channelruntime.InboundMessage, bool) {
	if event == nil || event.EventV2Base == nil || event.EventV2Base.Header == nil || event.Event == nil ||
		event.Event.Action == nil || event.Event.Operator == nil || event.Event.Context == nil {
		return nil, false
	}
	header := event.EventV2Base.Header
	if header.AppID != appID || strings.TrimSpace(header.TenantKey) != identity.ExternalTenantID || strings.TrimSpace(header.EventID) == "" {
		return nil, false
	}
	value := func(key string) string {
		text, _ := event.Event.Action.Value[key].(string)
		return strings.TrimSpace(text)
	}
	reply, wait := value(feishuCardReplyKey), value(feishuCardWaitKey)
	chatID, senderID := strings.TrimSpace(event.Event.Context.OpenChatID), strings.TrimSpace(event.Event.Operator.OpenID)
	if !feishuCardReplies[reply] || wait == "" || !validFeishuToken(chatID) || !validFeishuToken(senderID) {
		return nil, false
	}
	conversationType, wake := channelruntime.ConversationTypeDirect, channelruntime.WakeReasonDirectMessage
	if value(feishuCardTypeKey) == string(channelruntime.ConversationTypeGroup) {
		// A tap answers the bot's own card, so a group needs no mention.
		conversationType, wake = channelruntime.ConversationTypeGroup, channelruntime.WakeReasonReplyToSelf
	}
	eventID := strings.TrimSpace(header.EventID)
	return &channelruntime.InboundMessage{
		Provider: IntegrationID, ProviderEventID: eventID, ProviderMessageID: "card_action:" + eventID,
		ExternalTenantID: identity.ExternalTenantID, ExternalConversationID: chatID,
		ParentMessageID: strings.TrimSpace(event.Event.Context.OpenMessageID), ConversationType: conversationType,
		SenderExternalID: senderID, Text: reply, WakeReason: wake, OccurredAt: now, ActionWait: wait,
	}, true
}

func (receiver *WebSocketReceiver) handleCardAction(ctx context.Context, identity channelruntime.InstallationIdentity, appID string, event *callback.CardActionTriggerEvent, handle channelruntime.InboundMessageHandler) (*callback.CardActionTriggerResponse, error) {
	message, ok := cardActionMessage(identity, appID, event, time.Now().UTC())
	if !ok {
		return &callback.CardActionTriggerResponse{Toast: &callback.Toast{Type: "error", Content: "这个按钮已失效，请直接回复。"}}, nil
	}
	messageCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	if err := handle(messageCtx, message); err != nil {
		return &callback.CardActionTriggerResponse{Toast: &callback.Toast{Type: "error", Content: "没有提交成功，请直接回复「" + message.Text + "」。"}}, nil
	}
	return &callback.CardActionTriggerResponse{Toast: &callback.Toast{Type: "success", Content: "已提交「" + message.Text + "」"}}, nil
}
