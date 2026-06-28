package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

func applyRunConfigToChatRequest(config RunConfig, req runtimedto.ChatRequest) runtimedto.ChatRequest {
	if strings.TrimSpace(req.Model) == "" {
		req.Model = strings.TrimSpace(config.Model)
	}
	if strings.TrimSpace(req.Provider) == "" {
		req.Provider = strings.TrimSpace(config.ModelProvider)
	}
	if req.Parameters == nil && config.ModelParameters != nil {
		req.Parameters = copyStringAnyMap(config.ModelParameters)
	}
	if runConfigAllowsUserMemory(config) {
		req.UseMemory = true
	}
	return req
}

func applyRunConfigToRegenerateRequest(config RunConfig, req runtimedto.RegenerateMessageRequest) runtimedto.RegenerateMessageRequest {
	if req.Model == nil || strings.TrimSpace(*req.Model) == "" {
		if model := strings.TrimSpace(config.Model); model != "" {
			req.Model = &model
		}
	}
	if req.Provider == nil || strings.TrimSpace(*req.Provider) == "" {
		if provider := strings.TrimSpace(config.ModelProvider); provider != "" {
			req.Provider = &provider
		}
	}
	if req.Parameters == nil && config.ModelParameters != nil {
		req.Parameters = copyStringAnyMap(config.ModelParameters)
	}
	if runConfigAllowsUserMemory(config) && req.UseMemory == nil {
		useMemory := true
		req.UseMemory = &useMemory
	}
	return req
}

func applyRunConfigToParts(config RunConfig, parts *chatRequestParts) {
	if parts == nil {
		return
	}
	parts.SystemPrompt = strings.TrimSpace(config.SystemPrompt)
	parts.SystemPromptVersion = strings.TrimSpace(config.SystemPromptVersion)
	parts.ConfiguredSkillIDs = normalizedSkillIDs(config.EnabledSkillIDs)
	parts.KnowledgeDatasetIDs = normalizedSkillIDs(config.KnowledgeDatasetIDs)
	parts.KnowledgeRetrievalConfig = copyStringAnyMap(config.KnowledgeRetrievalConfig)
	parts.AgentMemoryEnabled = config.AgentMemoryEnabled
	parts.AgentMemorySlots = normalizeAgentMemorySlots(config.AgentMemorySlots)
	parts.AgentMemoryUserScope = strings.TrimSpace(config.AgentMemoryUserScope)
	parts.AgentMemoryAgentID = strings.TrimSpace(config.BillingAppID)
	parts.BillingSource = strings.TrimSpace(config.BillingAppType)
	if runConfigDisablesUserMemory(config) {
		parts.UseMemory = false
	}
}

func applyCallerRuntimeSurfacePolicy(caller Caller, parts *chatRequestParts) {
	if parts == nil {
		return
	}
	parts.Surface = normalizeRuntimeSurfaceForCaller(caller, parts.Surface)
}

func runConfigAllowsUserMemory(config RunConfig) bool {
	return config.UseMemory && !strings.EqualFold(strings.TrimSpace(config.BillingAppType), runtimemodel.ConversationCallerAgent)
}

func runConfigDisablesUserMemory(config RunConfig) bool {
	return strings.EqualFold(strings.TrimSpace(config.BillingAppType), runtimemodel.ConversationCallerAgent)
}

func normalizeAgentMemorySlots(input []AgentMemorySlotConfig) []AgentMemorySlotConfig {
	if len(input) == 0 {
		return nil
	}
	out := make([]AgentMemorySlotConfig, 0, len(input))
	seen := map[string]struct{}{}
	for i, slot := range input {
		key := strings.ToLower(strings.TrimSpace(slot.Key))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		maxChars := slot.MaxChars
		if maxChars <= 0 {
			maxChars = 1000
		}
		sortOrder := slot.SortOrder
		if sortOrder == 0 {
			sortOrder = i
		}
		out = append(out, AgentMemorySlotConfig{
			Key:         key,
			Description: strings.TrimSpace(slot.Description),
			MaxChars:    maxChars,
			Enabled:     slot.Enabled,
			SortOrder:   sortOrder,
		})
	}
	return out
}

func normalizedSkillIDs(input []string) []string {
	if input == nil {
		return nil
	}
	out := make([]string, 0, len(input))
	seen := map[string]struct{}{}
	for _, raw := range input {
		id := strings.ToLower(strings.TrimSpace(raw))
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func isUsableAssistantHistoryStatus(status string) bool {
	return status == runtimemodel.MessageStatusCompleted || status == runtimemodel.MessageStatusStopped
}

func normalizeChatRequest(req runtimedto.ChatRequest) (*chatRequestParts, error) {
	query := strings.TrimSpace(req.Query)
	surface := normalizeAIChatSurface(req.Surface)
	runtimeContext := normalizeRuntimeContext(req.RuntimeContext)
	operationContext, operationLedger := normalizeOperationContext(req.OperationContext)
	modelName := strings.TrimSpace(req.Model)
	if query == "" || modelName == "" {
		return nil, fmt.Errorf("%w: query and model are required", ErrInvalidInput)
	}
	params, err := normalizeModelParameters(req.Parameters)
	if err != nil {
		return nil, err
	}
	provider := strings.TrimSpace(req.Provider)
	var providerPtr *string
	if provider != "" {
		providerPtr = &provider
	}
	return &chatRequestParts{
		Query:               query,
		Surface:             surface,
		RuntimeContext:      runtimeContext,
		RawOperationContext: copyStringAnyMap(req.OperationContext),
		OperationContext:    operationContext,
		OperationLedger:     operationLedger,
		ModelName:           modelName,
		Provider:            provider,
		ProviderPtr:         providerPtr,
		Parameters:          params,
		UseMemory:           req.UseMemory,
	}, nil
}

func normalizeRuntimeContext(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= runtimeContextMaxRunes {
		return value
	}
	return strings.TrimSpace(string(runes[:runtimeContextMaxRunes]))
}

func normalizeRegenerateRequest(req runtimedto.RegenerateMessageRequest, message *runtimemodel.Message) (*chatRequestParts, error) {
	query := strings.TrimSpace(message.Query)
	if req.Query != nil {
		query = strings.TrimSpace(*req.Query)
	}
	modelName := strings.TrimSpace(message.ModelName)
	if req.Model != nil {
		modelName = strings.TrimSpace(*req.Model)
	}
	if query == "" || modelName == "" {
		return nil, fmt.Errorf("%w: query and model are required", ErrInvalidInput)
	}

	params := copyStringAnyMap(message.ModelParameters)
	if params == nil {
		params = map[string]interface{}{}
	}
	if req.Parameters != nil {
		var err error
		params, err = normalizeModelParameters(req.Parameters)
		if err != nil {
			return nil, err
		}
	}

	provider := ""
	if message.ModelProvider != nil {
		provider = strings.TrimSpace(*message.ModelProvider)
	}
	if req.Provider != nil {
		provider = strings.TrimSpace(*req.Provider)
	}
	var providerPtr *string
	if provider != "" {
		providerPtr = &provider
	}

	useMemory := boolMetadata(message.Metadata, "use_memory")
	if req.UseMemory != nil {
		useMemory = *req.UseMemory
	}
	runtimeContext := normalizeRuntimeContext(req.RuntimeContext)
	surface := normalizeAIChatSurface(regenerateRequestSurface(req, message))
	operationContext, operationLedger := normalizeOperationContext(req.OperationContext)

	return &chatRequestParts{
		Query:               query,
		Surface:             surface,
		RuntimeContext:      runtimeContext,
		RawOperationContext: copyStringAnyMap(req.OperationContext),
		OperationContext:    operationContext,
		OperationLedger:     operationLedger,
		ModelName:           modelName,
		Provider:            provider,
		ProviderPtr:         providerPtr,
		Parameters:          params,
		UseMemory:           useMemory,
	}, nil
}

func regenerateRequestSurface(req runtimedto.RegenerateMessageRequest, message *runtimemodel.Message) string {
	if strings.TrimSpace(req.Surface) != "" {
		return req.Surface
	}
	if message == nil {
		return ""
	}
	return stringMetadataValue(message.Metadata["surface"])
}

func replacementRootMessage(source *runtimemodel.Message, parts *chatRequestParts) *runtimemodel.Message {
	message := newStreamingMessage(source.ConversationID, nil, parts)
	message.ID = source.ID
	message.Metadata = withOperationPlanTaskID(message.Metadata, source.ID.String())
	message.CreatedAt = source.CreatedAt
	message.UpdatedAt = time.Now()
	return message
}

func canReplaceOnlyRootMessage(conversation *runtimemodel.Conversation, message *runtimemodel.Message, messageCount int64) bool {
	if conversation == nil || message == nil {
		return false
	}
	if conversation.RuntimeStatus == runtimemodel.ConversationRuntimeStatusStreaming {
		return false
	}
	if message.ParentID != nil {
		return false
	}
	if messageCount != 1 {
		return false
	}
	return conversation.CurrentLeafMessageID != nil && *conversation.CurrentLeafMessageID == message.ID
}

func newStreamingMessage(conversationID uuid.UUID, parentID *uuid.UUID, parts *chatRequestParts) *runtimemodel.Message {
	billingReasonSource := runtimemodel.MessageBillingReasonSourceAIChat
	if strings.TrimSpace(parts.BillingSource) != "" {
		billingReasonSource = strings.TrimSpace(parts.BillingSource)
	}
	messageID := uuid.New()
	return &runtimemodel.Message{
		ID:                  messageID,
		ConversationID:      conversationID,
		ParentID:            parentID,
		Query:               parts.Query,
		Status:              runtimemodel.MessageStatusStreaming,
		ModelProvider:       parts.ProviderPtr,
		ModelName:           parts.ModelName,
		BillingReasonSource: &billingReasonSource,
		ModelParameters:     parts.Parameters,
		Metadata:            streamingMessageMetadataWithTaskID(parts, messageID.String()),
	}
}

func streamingMessageMetadata(parts *chatRequestParts) map[string]interface{} {
	return streamingMessageMetadataWithTaskID(parts, "")
}

func streamingMessageMetadataWithTaskID(parts *chatRequestParts, taskID string) map[string]interface{} {
	version := systemPromptVersion
	if strings.TrimSpace(parts.SystemPromptVersion) != "" {
		version = strings.TrimSpace(parts.SystemPromptVersion)
	}
	metadata := map[string]interface{}{
		"system_prompt_version": version,
		"surface":               normalizeAIChatSurface(parts.Surface),
	}
	if parts.SkillMode != "" && parts.SkillMode != skillModeDisabled {
		metadata["has_trace"] = false
		metadata["skill_call_count"] = 0
		metadata["skill_names"] = []interface{}{}
		metadata["tool_call_count"] = 0
		metadata["tool_names"] = []interface{}{}
		metadata["guardrail_count"] = 0
		metadata["skill_mode"] = parts.SkillMode
		metadata["configured_skill_ids"] = parts.SkillIDs
	}
	if parts.ContextControl != nil {
		metadata["context_control"] = parts.ContextControl
	}
	if parts.UseMemory {
		metadata["use_memory"] = true
	}
	if parts.RuntimeContext != "" {
		metadata["runtime_context"] = map[string]interface{}{
			"included":   true,
			"char_count": len([]rune(parts.RuntimeContext)),
		}
	}
	if parts.OperationLedger != nil {
		metadata["operation_ledger"] = copyStringAnyMap(parts.OperationLedger)
	}
	if strategy := contextualAIChatTurnStrategyFromParts(parts); strategy != nil {
		metadata["turn_strategy"] = strategy
		if plan := operationPlanFromTurnStrategy(taskID, parts, strategy); len(plan) > 0 {
			metadata["operation_plan"] = plan
		}
	}
	if snapshot := consoleFilesContextSnapshot(parts); snapshot != nil {
		metadata[consoleFilesContextSnapshotKey] = snapshot
	}
	if parts.Attachments != nil && len(parts.Attachments.Files) > 0 {
		metadata["files"] = parts.Attachments.metadataFiles()
		metadata["file_count"] = len(parts.Attachments.Files)
	}
	return metadata
}

func normalizeAIChatSurface(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case aiChatSurfaceContextualSidebar, "contextual-sidebar", "sidebar", "contextual":
		return aiChatSurfaceContextualSidebar
	case aiChatSurfaceExternalPageChat, "external-page-chat", "external", "page-chat", "webapp", "agent-webapp":
		return aiChatSurfaceExternalPageChat
	case aiChatSurfaceWorkChat, "work-chat", "work", "aichat", "":
		return aiChatSurfaceWorkChat
	default:
		return aiChatSurfaceWorkChat
	}
}

func normalizeRuntimeSurfaceForCaller(caller Caller, surface string) string {
	if normalizeCallerType(caller.Type) == runtimemodel.ConversationCallerAgent {
		return aiChatSurfaceExternalPageChat
	}
	return normalizeAIChatSurface(surface)
}

func isContextualAIChatSurface(value string) bool {
	return normalizeAIChatSurface(value) == aiChatSurfaceContextualSidebar
}
