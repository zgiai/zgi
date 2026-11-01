package digitalhuman

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// ChannelConnectorPreviewReader reads the details a pending connector
// confirmation was frozen with, as the console shows them. The requester
// check is the status read's own.
type ChannelConnectorPreviewReader interface {
	ChannelConnectorConfirmationPreview(ctx context.Context, organizationID, ownerAccountID, receipt uuid.UUID) ([]channelruntime.DeliveryCardField, error)
}

// channelCardProviders render a prompt's card and edit it as the decision
// lands. Every other channel sends the prompt's text.
var channelCardProviders = map[string]bool{"feishu": true}

const (
	channelConnectorGroupDetailsNotice = "为保护操作内容，群聊只显示操作名称。详细内容请在与数字人的私聊或 ZGI 控制台查看。"
	channelConnectorNoDetailsNotice    = "详细内容请在 ZGI 控制台查看。"
	maxChannelCardFields               = 12
)

// channelConnectorCard describes the operation a run waits on for a channel
// that renders cards. A group sees only what the operation is: its details
// can carry message or command content not meant for every member.
func (s *RuntimeService) channelConnectorCard(ctx context.Context, run *Run, answer string, metadata map[string]interface{}) *channelruntime.DeliveryCard {
	if run == nil || run.WaitingReceipt == nil {
		return nil
	}
	var event invocation.Event
	if json.Unmarshal(run.InvocationEvent, &event) != nil || event.Origin.Kind != invocation.OriginChannel ||
		!channelCardProviders[strings.ToLower(strings.TrimSpace(event.Origin.Provider))] {
		return nil
	}
	confirmation := connectorConfirmationRecord(jsonNormalizedMap(metadata), run.WaitingReceipt.String())
	if confirmation == nil {
		return nil
	}
	title := strings.Trim(localizedChannelText(confirmation["connector_name_i18n"], confirmation["connector_name"])+" · "+
		localizedChannelText(confirmation["action_name_i18n"], confirmation["action_name"]), " ·")
	if title == "" {
		return nil
	}
	card := &channelruntime.DeliveryCard{
		Title:    title,
		HighRisk: strings.EqualFold(strings.TrimSpace(stringValue(confirmation["risk_classification"])), "high"),
		Summary:  strings.TrimSpace(answer),
		State:    channelruntime.DeliveryCardPending,
	}
	if card.Summary == "" {
		consequence, _ := confirmation["consequence"].(map[string]interface{})
		card.Summary = localizedChannelText(consequence["description_i18n"], consequence["description"])
	}
	if contextString(event.Context, "conversation_type") == string(channelruntime.ConversationTypeGroup) {
		card.Notice = channelConnectorGroupDetailsNotice
		return card
	}
	reader, ok := s.channelConnectors.(ChannelConnectorPreviewReader)
	if !ok || run.ActorID == nil || *run.ActorID == uuid.Nil {
		card.Notice = channelConnectorNoDetailsNotice
		return card
	}
	fields, err := reader.ChannelConnectorConfirmationPreview(ctx, run.OrganizationID, *run.ActorID, *run.WaitingReceipt)
	if err != nil {
		logger.WarnContext(ctx, "failed to read DigitalHuman channel connector preview", "run_id", run.ID.String(), err)
	}
	if len(fields) == 0 {
		card.Notice = channelConnectorNoDetailsNotice
		return card
	}
	if len(fields) > maxChannelCardFields {
		fields = fields[:maxChannelCardFields]
	}
	card.Fields = fields
	return card
}

// channelCardDetailsText is a card's details as text, for the prompt's text
// form, which a channel shows when it cannot render the card.
func channelCardDetailsText(card *channelruntime.DeliveryCard) string {
	if card == nil {
		return ""
	}
	lines := []string{"操作：" + card.Title}
	for _, field := range card.Fields {
		value := field.Value
		if field.Truncated {
			value += "（已截断）"
		}
		lines = append(lines, field.Label+"："+value)
	}
	return strings.Join(lines, "\n")
}

// connectorConfirmationRecord finds, in a waiting message's metadata, the
// confirmation the run waits on: the record naming its receipt.
func connectorConfirmationRecord(value interface{}, receipt string) map[string]interface{} {
	return findConnectorConfirmation(value, strings.TrimSpace(receipt), 0)
}

func findConnectorConfirmation(value interface{}, receipt string, depth int) map[string]interface{} {
	if receipt == "" || depth > 12 {
		return nil
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		if stringValue(typed["route_receipt"]) == receipt && strings.TrimSpace(stringValue(typed["connector_name"])) != "" {
			return typed
		}
		for _, child := range typed {
			if found := findConnectorConfirmation(child, receipt, depth+1); found != nil {
				return found
			}
		}
	case []interface{}:
		for _, child := range typed {
			if found := findConnectorConfirmation(child, receipt, depth+1); found != nil {
				return found
			}
		}
	}
	return nil
}

// localizedChannelText is the Simplified Chinese text of a localized value,
// as channel prompts are written, or its default text.
func localizedChannelText(localized interface{}, fallback interface{}) string {
	if values, ok := localized.(map[string]interface{}); ok {
		if text := strings.TrimSpace(stringValue(values["zh-Hans"])); text != "" {
			return text
		}
	}
	return strings.TrimSpace(stringValue(fallback))
}

// channelConnectorCardState is the card state a decided confirmation shows,
// with the line that says how it ended.
func channelConnectorCardState(state string, summary string) (channelruntime.DeliveryCardState, string) {
	switch state {
	case "completed":
		if summary = strings.TrimSpace(summary); summary != "" {
			return channelruntime.DeliveryCardCompleted, "已确认并执行完成。\n" + summary
		}
		return channelruntime.DeliveryCardCompleted, "已确认并执行完成。"
	case "cancelled":
		return channelruntime.DeliveryCardCancelled, "已取消，这个操作没有执行。"
	case "failed":
		return channelruntime.DeliveryCardFailed, "已确认，但执行失败。请到 ZGI 控制台的运行记录查看原因。"
	default:
		return channelruntime.DeliveryCardUnknown, "已确认，但执行结果暂时无法确定。请到 ZGI 控制台的运行记录核实，不要重复执行。"
	}
}

type channelPromptUpdateStore interface {
	EnqueueRunChannelPromptUpdate(context.Context, uuid.UUID, string, channelruntime.DeliveryCardState, string, time.Time) error
}

// updateChannelConnectorPrompt shows a decided confirmation on the card that
// asked for it, wherever it was decided: a tap, a typed reply, the console or
// the API. It is best effort; the decision is already recorded.
func (s *RuntimeService) updateChannelConnectorPrompt(ctx context.Context, runID, messageID uuid.UUID, receipt, state, summary string) {
	store, ok := s.store.(channelPromptUpdateStore)
	if !ok || runID == uuid.Nil || messageID == uuid.Nil || strings.TrimSpace(receipt) == "" {
		return
	}
	cardState, outcome := channelConnectorCardState(state, summary)
	wait := channelWaitKey(messageID, runtimemodel.MessageStatusWaitingApproval, strings.TrimSpace(receipt))
	if err := store.EnqueueRunChannelPromptUpdate(context.WithoutCancel(ctx), runID, "waiting:"+wait, cardState, outcome, s.now()); err != nil {
		logger.WarnContext(ctx, "failed to update DigitalHuman channel connector prompt", "run_id", runID.String(), err)
	}
}
