package workflow

import (
	"context"

	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
)

func buildChatMessagesResponse(messages []*conversation.AgentMessage, total int64, page, limit int) map[string]interface{} {
	return buildChatMessagesResponseWithContext(context.Background(), messages, total, page, limit)
}

func buildChatMessagesResponseWithContext(ctx context.Context, messages []*conversation.AgentMessage, total int64, page, limit int) map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(messages))
	for _, msg := range messages {
		item := map[string]interface{}{
			"id":              msg.ID.String(),
			"conversation_id": msg.ConversationID.String(),
			"query":           msg.Query,
			"answer":          msg.Answer,
			"status":          msg.Status,
			"created_at":      msg.CreatedAt.Unix(),
		}

		if msg.WorkflowRunID != nil {
			item["workflow_run_id"] = msg.WorkflowRunID.String()
		}
		if msg.ParentMessageID != nil {
			item["parent_message_id"] = msg.ParentMessageID.String()
		}
		if msg.InvokeFrom != nil {
			item["invoke_from"] = *msg.InvokeFrom
		}
		if inputs, err := msg.GetInputsAsMap(); err == nil {
			item["inputs"] = inputs
		}
		if metadata := conversationMessageMetadataWithQuestionAnswer(ctx, msg); len(metadata) > 0 {
			item["message_metadata"] = metadata
		}

		items = append(items, item)
	}

	hasMore := int64(page*limit) < total
	return map[string]interface{}{
		"data":     items,
		"page":     page,
		"limit":    limit,
		"total":    total,
		"has_more": hasMore,
	}
}
