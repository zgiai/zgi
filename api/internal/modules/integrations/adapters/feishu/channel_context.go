package feishu

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const (
	feishuContextLookback = 7 * 24 * time.Hour
	feishuMemberCacheTTL  = 10 * time.Minute
)

type feishuMemberCacheEntry struct {
	Names     map[string]string
	ExpiresAt time.Time
}

// ReadContext implements channelruntime.ContextReader for server-bound Feishu
// group and direct conversations. The model cannot select the installation or
// chat.
func (adapter *ChannelAdapter) ReadContext(ctx context.Context, request channelruntime.ChannelContextRequest) (*channelruntime.ChannelContextPack, error) {
	if adapter == nil || (adapter.executor == nil && !adapter.directProviderAvailable()) || request.OrganizationID == uuid.Nil || request.WorkspaceID == uuid.Nil ||
		request.ActorID == uuid.Nil || request.InstallationID == uuid.Nil || request.ConnectionID == uuid.Nil ||
		request.RunID == uuid.Nil || strings.TrimSpace(request.ExternalConversationID) == "" ||
		strings.TrimSpace(request.CurrentMessageID) == "" {
		return nil, channelruntime.ErrInvalidWebhook
	}
	limit := request.Limit
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	pack := &channelruntime.ChannelContextPack{Status: channelruntime.ContextStatusComplete}
	input := map[string]interface{}{
		"chat_id":   request.ExternalConversationID,
		"page_size": limit,
		"sort_type": "newest_first",
		"end_time":  request.OccurredAt.Unix(),
	}
	// After marks the previous successful DigitalHuman delivery. It is a
	// selection-priority hint, not a provider query boundary: relevant nearby
	// messages can precede that reply and still be required by the current ask.
	input["start_time"] = request.OccurredAt.Add(-feishuContextLookback).Unix()
	if token := strings.TrimSpace(request.PageToken); token != "" {
		input["page_token"] = token
	}
	result, err := adapter.executeContextAction(ctx, request, ActionListMessages, input)
	if err != nil {
		return nil, err
	}
	pack.ProviderRequestIDs = appendRequestID(pack.ProviderRequestIDs, result.ProviderRequestID)
	pack.Messages = append(pack.Messages, contextMessagesFromOutput(result.Output)...)
	pack.NextPageToken = outputString(result.Output, "next_page_token")

	seen := make(map[string]struct{}, len(pack.Messages)+3)
	for _, message := range pack.Messages {
		seen[message.MessageID] = struct{}{}
	}
	for _, messageID := range []string{request.ParentMessageID, request.RootMessageID} {
		messageID = strings.TrimSpace(messageID)
		if messageID == "" || messageID == request.CurrentMessageID {
			continue
		}
		if _, ok := seen[messageID]; ok {
			continue
		}
		itemResult, itemErr := adapter.executeContextAction(ctx, request, ActionGetMessage, map[string]interface{}{"message_id": messageID})
		if itemErr != nil {
			pack.Status = channelruntime.ContextStatusPartial
			pack.ErrorCode = integrations.ErrorCode(itemErr)
			continue
		}
		pack.ProviderRequestIDs = appendRequestID(pack.ProviderRequestIDs, itemResult.ProviderRequestID)
		items := contextMessagesFromOutput(itemResult.Output)
		pack.Messages = append(pack.Messages, items...)
		for _, item := range items {
			seen[item.MessageID] = struct{}{}
		}
	}

	names, requestIDs, namesErr := adapter.chatMemberNames(ctx, request)
	for _, requestID := range requestIDs {
		pack.ProviderRequestIDs = appendRequestID(pack.ProviderRequestIDs, requestID)
	}
	if namesErr != nil {
		pack.Status = channelruntime.ContextStatusPartial
		if pack.ErrorCode == "" {
			pack.ErrorCode = integrations.ErrorCode(namesErr)
		}
	}
	for index := range pack.Messages {
		pack.Messages[index].Author.Label = strings.TrimSpace(names[pack.Messages[index].Author.ExternalID])
	}
	pack.CurrentAuthor = channelruntime.ChannelAuthor{
		ExternalID: request.CurrentSenderID,
		Label:      strings.TrimSpace(names[request.CurrentSenderID]),
	}
	pack.Messages = uniqueContextMessages(pack.Messages, request.CurrentMessageID, request.OccurredAt)
	return pack, nil
}

func (adapter *ChannelAdapter) executeContextAction(ctx context.Context, request channelruntime.ChannelContextRequest, actionID string, input map[string]interface{}) (*integrations.ActionResult, error) {
	if adapter.directProviderAvailable() {
		return adapter.executeDirectContextAction(ctx, request, actionID, input)
	}
	operationID := uuid.New().String()
	return adapter.executor.Execute(ctx, integrations.ActionRequest{
		OrganizationID:  request.OrganizationID.String(),
		WorkspaceID:     request.WorkspaceID.String(),
		UserID:          request.ActorID.String(),
		ConversationID:  request.RunID.String(),
		MessageID:       operationID,
		ConnectionID:    request.ConnectionID.String(),
		AppID:           request.Subject.ID.String(),
		OperationItemID: operationID,
		InvokeFrom:      tools.ToolInvokeFromAPI,
		IntegrationID:   IntegrationID,
		ActionID:        actionID,
		Input:           input,
	})
}

func (adapter *ChannelAdapter) executeDirectContextAction(
	ctx context.Context,
	request channelruntime.ChannelContextRequest,
	actionID string,
	input map[string]interface{},
) (*integrations.ActionResult, error) {
	connection, cleanup, err := adapter.resolveDirectConnection(ctx, request.OrganizationID, request.ConnectionID)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	region, err := feishuRegion(connection)
	if err != nil {
		return nil, err
	}
	token, tokenMeta, err := adapter.provider.tenantAccessToken(ctx, connection, region)
	if err != nil {
		return feishuActionResult(nil, tokenMeta, 0), err
	}
	var output map[string]interface{}
	var meta responseMeta
	switch actionID {
	case ActionListMessages:
		output, meta, err = adapter.provider.listMessages(ctx, region, token, feishuConnectionAppID(connection), input)
	case ActionGetMessage:
		output, meta, err = adapter.provider.getMessage(ctx, region, token, feishuConnectionAppID(connection), input)
	case ActionListChatMembers:
		output, meta, err = adapter.provider.listChatMembers(ctx, region, token, input)
	default:
		return nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu channel context action is not supported", nil)
	}
	meta.Attempts += tokenMeta.Attempts
	return feishuActionResult(output, meta, outputCountForChannelAction(actionID, output)), err
}

func outputCountForChannelAction(actionID string, output map[string]interface{}) int {
	switch actionID {
	case ActionListMessages, ActionGetMessage:
		return outputCount(output, "messages")
	case ActionListChatMembers:
		return outputCount(output, "members")
	default:
		return 0
	}
}

func (adapter *ChannelAdapter) chatMemberNames(ctx context.Context, request channelruntime.ChannelContextRequest) (map[string]string, []string, error) {
	cacheKey := request.InstallationID.String() + ":" + request.ExternalConversationID
	now := adapter.now()
	adapter.memberCacheMu.Lock()
	if cached, ok := adapter.memberCache[cacheKey]; ok && now.Before(cached.ExpiresAt) {
		names := cloneNames(cached.Names)
		adapter.memberCacheMu.Unlock()
		return names, nil, nil
	}
	adapter.memberCacheMu.Unlock()

	names := make(map[string]string)
	requestIDs := make([]string, 0, 2)
	pageToken := ""
	for page := 0; page < 10; page++ {
		input := map[string]interface{}{"chat_id": request.ExternalConversationID, "page_size": 100}
		if pageToken != "" {
			input["page_token"] = pageToken
		}
		result, err := adapter.executeContextAction(ctx, request, ActionListChatMembers, input)
		if err != nil {
			return nil, requestIDs, err
		}
		requestIDs = appendRequestID(requestIDs, result.ProviderRequestID)
		for _, raw := range outputItems(result.Output, "members") {
			member, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			id, name := outputString(member, "member_id"), outputString(member, "name")
			if id != "" && name != "" {
				names[id] = name
			}
		}
		pageToken = outputString(result.Output, "next_page_token")
		hasMore, _ := result.Output["has_more"].(bool)
		if !hasMore || pageToken == "" {
			break
		}
	}
	adapter.memberCacheMu.Lock()
	adapter.memberCache[cacheKey] = feishuMemberCacheEntry{Names: cloneNames(names), ExpiresAt: now.Add(feishuMemberCacheTTL)}
	adapter.memberCacheMu.Unlock()
	return names, requestIDs, nil
}

func contextMessagesFromOutput(output map[string]interface{}) []channelruntime.ChannelContextMessage {
	items := outputItems(output, "messages")
	messages := make([]channelruntime.ChannelContextMessage, 0, len(items))
	for _, raw := range items {
		message, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		occurredAt := parseFeishuTimestamp(outputString(message, "create_time"))
		messages = append(messages, channelruntime.ChannelContextMessage{
			MessageID:       outputString(message, "message_id"),
			RootMessageID:   outputString(message, "root_id"),
			ParentMessageID: outputString(message, "parent_id"),
			ThreadID:        outputString(message, "thread_id"),
			Author: channelruntime.ChannelAuthor{
				ExternalID: outputString(message, "sender_id"),
				IsSelf:     outputBool(message, "sender_is_self"),
			},
			Text:        outputString(message, "text"),
			MessageType: outputString(message, "message_type"),
			OccurredAt:  occurredAt,
			Edited:      outputBool(message, "updated"),
			Deleted:     outputBool(message, "deleted"),
		})
	}
	return messages
}

func uniqueContextMessages(messages []channelruntime.ChannelContextMessage, currentMessageID string, occurredAt time.Time) []channelruntime.ChannelContextMessage {
	byID := make(map[string]channelruntime.ChannelContextMessage, len(messages))
	for _, message := range messages {
		id := strings.TrimSpace(message.MessageID)
		if id == "" || id == currentMessageID || message.Deleted || message.OccurredAt.After(occurredAt) {
			continue
		}
		byID[id] = message
	}
	result := make([]channelruntime.ChannelContextMessage, 0, len(byID))
	for _, message := range byID {
		result = append(result, message)
	}
	sort.SliceStable(result, func(left, right int) bool {
		return result[left].OccurredAt.Before(result[right].OccurredAt)
	})
	return result
}

func outputItems(output map[string]interface{}, key string) []interface{} {
	if output == nil {
		return nil
	}
	items, _ := output[key].([]interface{})
	return items
}

func outputString(output map[string]interface{}, key string) string {
	if output == nil {
		return ""
	}
	value, _ := output[key].(string)
	return strings.TrimSpace(value)
}

func outputBool(output map[string]interface{}, key string) bool {
	value, _ := output[key].(bool)
	return value
}

func parseFeishuTimestamp(value string) time.Time {
	raw, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return time.Time{}
	}
	if raw > 10_000_000_000 {
		return time.UnixMilli(raw).UTC()
	}
	return time.Unix(raw, 0).UTC()
}

func appendRequestID(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}

func cloneNames(source map[string]string) map[string]string {
	copy := make(map[string]string, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}
