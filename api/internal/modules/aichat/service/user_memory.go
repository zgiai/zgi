package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/usermemoryruntime"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/memory"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const (
	streamEventMemoryCreate = "memory_create"
	streamEventMemoryUpdate = "memory_update"
	streamEventMemoryDelete = "memory_delete"
)

func (s *service) runUserMemoryPreflight(
	ctx context.Context,
	persistCtx context.Context,
	prepared *PreparedChat,
	onEvent func(StreamEvent) error,
) (*adapter.Usage, error) {
	state, memorySvc, skipStatus := s.userMemoryPreflightState(ctx, prepared, s.llmClient != nil)
	if skipStatus != "" {
		if prepared != nil && prepared.parts != nil && prepared.parts.UseMemory {
			s.updateUserMemoryRuntimeMetadataBestEffort(persistCtx, prepared, map[string]interface{}{
				"planner_status": skipStatus,
				"planner_action": "none",
			})
		}
		return nil, nil
	}
	result := usermemoryruntime.RunPreflight(ctx, usermemoryruntime.PreflightRequest{
		LatestUserMessage: prepared.parts.Query,
		LLMRequest:        prepared.LLMRequest,
		State:             state,
		MemoryService:     memorySvc,
		AccountID:         userMemoryAccountID(prepared),
		MutationMetadata:  userMemoryMutationMetadata(prepared),
		LLMClient:         s.llmClient,
		AppContext:        newBillingAppContext(prepared),
		OnMutation: func(trace skills.SkillTrace, result map[string]interface{}) {
			s.emitUserMemoryMutationEvent(ctx, prepared, trace, result, onEvent)
		},
	})
	if len(result.MetadataUpdates) > 0 {
		s.updateUserMemoryRuntimeMetadataBestEffort(persistCtx, prepared, result.MetadataUpdates)
	}
	if len(result.Messages) > 0 {
		prepared.LLMRequest.Messages = result.Messages
	}
	return result.Usage, nil
}

func (s *service) userMemoryPreflightState(ctx context.Context, prepared *PreparedChat, llmConfigured bool) (*usermemoryruntime.State, usermemoryruntime.MemoryService, string) {
	if prepared == nil || prepared.parts == nil {
		return nil, nil, "skipped_scope"
	}
	if !prepared.parts.UseMemory {
		return nil, nil, "skipped_disabled"
	}
	memorySvc, ok := s.memoryService.(usermemoryruntime.MemoryService)
	if !ok || memorySvc == nil || !llmConfigured {
		return nil, nil, "skipped_scope"
	}
	accountID := userMemoryAccountID(prepared)
	if accountID == uuid.Nil {
		return nil, nil, "skipped_scope"
	}
	enabled, err := memorySvc.IsEnabled(ctx, accountID)
	if err != nil || !enabled {
		return nil, nil, "skipped_scope"
	}
	state, err := memorySvc.GetModelState(ctx, accountID)
	if err != nil {
		return nil, nil, "error_state"
	}
	return &usermemoryruntime.State{
		AccountID: accountID,
		Entries:   append([]memory.MemoryEntryResponse(nil), state.Entries...),
	}, memorySvc, ""
}

func userMemoryAccountID(prepared *PreparedChat) uuid.UUID {
	if prepared == nil {
		return uuid.Nil
	}
	if prepared.Scope.AccountID != uuid.Nil {
		return prepared.Scope.AccountID
	}
	if prepared.Conversation != nil {
		return prepared.Conversation.AccountID
	}
	return uuid.Nil
}

func userMemoryMutationMetadata(prepared *PreparedChat) memory.MutationMetadata {
	meta := memory.MutationMetadata{
		ActorType: memory.EventActorModel,
		Source:    memory.EventSourceAIChat,
	}
	if prepared != nil && prepared.Conversation != nil {
		id := prepared.Conversation.ID
		meta.SourceConversationID = &id
	}
	if prepared != nil && prepared.Message != nil {
		id := prepared.Message.ID
		meta.SourceMessageID = &id
	}
	return meta
}

func (s *service) updateUserMemoryRuntimeMetadataBestEffort(ctx context.Context, prepared *PreparedChat, updates map[string]interface{}) {
	if prepared == nil || prepared.Message == nil || len(updates) == 0 {
		return
	}
	metadata := copyStringAnyMap(prepared.Message.Metadata)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	contextControl := map[string]interface{}{}
	if prepared.parts != nil && prepared.parts.ContextControl != nil {
		contextControl = copyStringAnyMap(prepared.parts.ContextControl)
	}
	if existing, ok := metadata["context_control"].(map[string]interface{}); ok {
		for key, value := range existing {
			contextControl[key] = value
		}
	}
	userMemory := map[string]interface{}{}
	if existing, ok := contextControl["user_memory"].(map[string]interface{}); ok {
		userMemory = copyStringAnyMap(existing)
	}
	for key, value := range updates {
		if stringValue, ok := value.(string); ok && strings.TrimSpace(stringValue) == "" {
			continue
		}
		userMemory[key] = value
	}
	contextControl["user_memory"] = userMemory
	metadata["context_control"] = contextControl
	prepared.Message.Metadata = metadata
	if prepared.parts != nil {
		prepared.parts.ContextControl = contextControl
	}
	if s == nil || s.repos == nil || s.repos.Message == nil {
		return
	}
	_ = s.repos.Message.UpdateMetadata(ctx, prepared.Message.ID, metadata)
}

func (s *service) emitUserMemoryMutationEvent(ctx context.Context, prepared *PreparedChat, trace skills.SkillTrace, result map[string]interface{}, onEvent func(StreamEvent) error) {
	eventType := userMemoryMutationEventType(result)
	if eventType == "" || prepared == nil || prepared.Conversation == nil || prepared.Message == nil {
		return
	}
	payload := map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"memory_scope":    "account",
		"action":          result["action"],
		"entry_id":        result["entry_id"],
		"category":        result["category"],
		"memory_type":     result["memory_type"],
		"status":          trace.Status,
	}
	if content, ok := result["content"].(string); ok && strings.TrimSpace(content) != "" {
		content = strings.TrimSpace(content)
		payload["content"] = content
		payload["content_preview"] = truncateString(content, 160)
	}
	s.emitPreparedEvent(ctx, prepared, eventType, payload, onEvent)
}

func userMemoryMutationEventType(result map[string]interface{}) string {
	action, _ := result["action"].(string)
	switch strings.TrimSpace(action) {
	case usermemoryruntime.ActionCreate:
		return streamEventMemoryCreate
	case usermemoryruntime.ActionUpdate:
		return streamEventMemoryUpdate
	case usermemoryruntime.ActionDelete:
		return streamEventMemoryDelete
	default:
		return ""
	}
}

func truncateString(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}
