package service

import (
	"context"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/modelprogress"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func (s *service) startPreparedModelProgress(
	ctx context.Context,
	prepared *PreparedChat,
	round int,
	messages []adapter.Message,
	onEvent func(StreamEvent) error,
) *modelprogress.Tracker {
	if s == nil || prepared == nil || prepared.Message == nil || prepared.Conversation == nil {
		return nil
	}
	if prepared.parts == nil || normalizeExecutionMode(prepared.parts.ExecutionMode) != executionModeDirectChat {
		return nil
	}
	model := ""
	if prepared.LLMRequest != nil {
		model = prepared.LLMRequest.Model
	}
	return modelprogress.Start(ctx, modelprogress.Options{
		ConversationID: prepared.Conversation.ID.String(),
		MessageID:      prepared.Message.ID.String(),
		Round:          round,
		Model:          model,
		Schedule:       s.modelProgressSchedule,
		Emit: func(payload map[string]interface{}) {
			_ = s.emitPreparedEvent(ctx, prepared, streamEventAgentProgress, payload, onEvent)
		},
	})
}
