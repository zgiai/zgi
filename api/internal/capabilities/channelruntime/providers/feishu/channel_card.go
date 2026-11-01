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
//
// A prompt that carries a structured card shows the operation it asks about,
// and once the decision is recorded its outcome replaces it
// (ChannelAdapter.Edit). A tap only reports that the reply was received: the
// reply may still be refused (another member of a group) or not yet decided,
// so the card and its buttons stay until the decision itself updates them.
// The card is shared (update_multi), so everyone in a group sees one state.

const (
	feishuCardReplyKey = "zgi_reply"
	feishuCardWaitKey  = "zgi_wait"
	feishuCardTypeKey  = "zgi_conversation_type"
	// A card larger than this goes as rich text without buttons.
	maxChannelCardBytes = 28 * 1024
	// Field values beyond this are shortened on the card; the full value
	// stays in the console.
	maxChannelCardFieldRunes = 1000
	maxChannelCardTitleRunes = 60
)

// feishuCardReplies are the only words a button may send.
var feishuCardReplies = map[string]bool{"确认": true, "取消": true, "同意": true, "拒绝": true}

// buildChannelCard renders a delivery with actions as an interactive card.
// It reports false when the card cannot carry them, and the delivery goes as
// rich text instead.
func buildChannelCard(request channelruntime.DeliveryRequest) (channelMessageBody, bool) {
	if len(request.Attachments) != 0 {
		return channelMessageBody{}, false
	}
	var card map[string]any
	if request.Card != nil {
		card = structuredChannelCard(request, request.Card, maxChannelCardFieldRunes)
		if card == nil {
			return channelMessageBody{}, false
		}
	} else {
		buttons := channelCardButtons(request.Actions, nil)
		if len(buttons) == 0 {
			return channelMessageBody{}, false
		}
		card = map[string]any{
			"config": map[string]any{"wide_screen_mode": true},
			"elements": []map[string]any{
				{"tag": "markdown", "content": channelCardLead(request.Mentions, request.Text)},
				{"tag": "action", "actions": buttons},
			},
		}
	}
	content, ok := marshalChannelCard(card)
	if !ok && request.Card != nil {
		// Long details are what make a card too large; shorter ones keep it a card.
		content, ok = marshalChannelCard(structuredChannelCard(request, request.Card, 200))
	}
	if !ok {
		return channelMessageBody{}, false
	}
	return channelMessageBody{
		ReceiveID: strings.TrimSpace(request.ExternalConversationID), MsgType: "interactive", Content: content,
		UUID: uuid.NewSHA1(request.DeliveryID, []byte("feishu:interactive")).String(),
	}, true
}

func marshalChannelCard(card map[string]any) (string, bool) {
	if card == nil {
		return "", false
	}
	data, err := json.Marshal(card)
	if err != nil || len(data) > maxChannelCardBytes {
		return "", false
	}
	return string(data), true
}

// channelCardLead is the card's opening text. Generated text never notifies
// anyone; only trusted mentions do.
func channelCardLead(mentions []channelruntime.DeliveryMention, text string) string {
	var content strings.Builder
	for _, mention := range channelPostMentions(mentions) {
		content.WriteString(`<at id=` + mention.UserID + `></at> `)
	}
	content.WriteString(feishuMentionTag.ReplaceAllStringFunc(strings.TrimSpace(text), html.EscapeString))
	return strings.TrimSpace(content.String())
}

func channelCardButtons(actions []channelruntime.DeliveryAction, card *channelruntime.DeliveryCard) []map[string]any {
	buttons := make([]map[string]any, 0, len(actions))
	for _, action := range actions {
		reply := strings.TrimSpace(action.Reply)
		if !feishuCardReplies[reply] || strings.TrimSpace(action.Wait) == "" {
			continue
		}
		kind := "default"
		if action.Primary {
			kind = "primary"
		}
		value := map[string]string{
			feishuCardReplyKey: reply, feishuCardWaitKey: strings.TrimSpace(action.Wait), feishuCardTypeKey: string(action.ConversationType),
		}
		button := map[string]any{
			"tag": "button", "type": kind,
			"text":  map[string]string{"tag": "plain_text", "content": bounded(firstNonEmpty(strings.TrimSpace(action.Label), reply), 20)},
			"value": value,
		}
		if card != nil {
			if card.HighRisk && action.Primary {
				kind = "danger"
				button["type"] = kind
				// A high-risk operation asks once more before it is sent.
				button["confirm"] = map[string]any{
					"title": map[string]string{"tag": "plain_text", "content": "确认执行这个操作？"},
					"text":  map[string]string{"tag": "plain_text", "content": bounded(card.Title, maxChannelCardTitleRunes)},
				}
			}
		}
		buttons = append(buttons, button)
	}
	return buttons
}

// structuredChannelCard renders an operation card: what it is, the details
// to check, and either the buttons that decide it or how it ended.
func structuredChannelCard(request channelruntime.DeliveryRequest, card *channelruntime.DeliveryCard, fieldRunes int) map[string]any {
	title := bounded(card.Title, maxChannelCardTitleRunes)
	if title == "" {
		return nil
	}
	pending := card.Pending()
	var buttons []map[string]any
	if pending {
		buttons = channelCardButtons(request.Actions, card)
		if len(buttons) == 0 {
			return nil
		}
	}
	elements := make([]map[string]any, 0, len(card.Fields)+5)
	if lead := channelCardLead(request.Mentions, card.Summary); lead != "" {
		elements = append(elements, map[string]any{"tag": "markdown", "content": lead})
	}
	if len(card.Fields) > 0 {
		elements = append(elements, map[string]any{"tag": "hr"})
	}
	for _, field := range card.Fields {
		label, value := bounded(field.Label, 64), strings.TrimSpace(field.Value)
		if label == "" || value == "" {
			continue
		}
		truncated := field.Truncated
		if runes := []rune(value); len(runes) > fieldRunes {
			value, truncated = string(runes[:fieldRunes]), true
		}
		if truncated {
			label += "（已截断）"
		}
		// Values are plain text: an operation's content is shown, never rendered.
		elements = append(elements, map[string]any{
			"tag": "div",
			"fields": []map[string]any{
				{"is_short": false, "text": map[string]string{"tag": "plain_text", "content": label}},
				{"is_short": false, "text": map[string]string{"tag": "plain_text", "content": value}},
			},
		})
	}
	if notice := strings.TrimSpace(card.Notice); notice != "" {
		elements = append(elements, map[string]any{
			"tag": "note", "elements": []map[string]string{{"tag": "plain_text", "content": notice}},
		})
	}
	if pending {
		elements = append(elements, map[string]any{"tag": "action", "actions": buttons})
	} else if outcome := strings.TrimSpace(card.Outcome); outcome != "" {
		elements = append(elements, map[string]any{"tag": "hr"}, map[string]any{
			"tag": "div", "text": map[string]string{"tag": "plain_text", "content": outcome},
		})
	}
	return map[string]any{
		"config":   map[string]any{"wide_screen_mode": true, "update_multi": true},
		"header":   channelCardHeader(card.State, card.HighRisk, title),
		"elements": elements,
	}
}

// channelCardHeader names the card's state before the operation, colored by
// where the decision stands.
func channelCardHeader(state channelruntime.DeliveryCardState, highRisk bool, title string) map[string]any {
	prefix, template := "需要你确认", "orange"
	if highRisk {
		template = "red"
	}
	switch state {
	case channelruntime.DeliveryCardCompleted:
		prefix, template = "已执行", "green"
	case channelruntime.DeliveryCardFailed:
		prefix, template = "执行失败", "red"
	case channelruntime.DeliveryCardUnknown:
		prefix, template = "结果待核实", "yellow"
	case channelruntime.DeliveryCardCancelled:
		prefix, template = "已取消", "grey"
	}
	return map[string]any{
		"template": template,
		"title":    map[string]string{"tag": "plain_text", "content": prefix + "｜" + title},
	}
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
	// Received is not decided: the card changes only when the decision is
	// recorded, so a refused or pending reply never shows as an outcome.
	return &callback.CardActionTriggerResponse{Toast: &callback.Toast{Type: "success", Content: "已提交「" + message.Text + "」"}}, nil
}
