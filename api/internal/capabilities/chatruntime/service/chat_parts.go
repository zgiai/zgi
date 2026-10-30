package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
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
	return req
}

func applyRunConfigToParts(config RunConfig, parts *chatRequestParts) {
	if parts == nil {
		return
	}
	parts.SystemPrompt = strings.TrimSpace(config.SystemPrompt)
	parts.SystemPromptVersion = strings.TrimSpace(config.SystemPromptVersion)
	parts.TurnContext = strings.TrimSpace(config.TurnContext)
	parts.CurrentTimeProvided = config.CurrentTimeProvided
	parts.UserTimezone = strings.TrimSpace(config.UserTimezone)
	parts.ConfiguredSkillIDs = normalizedSkillIDs(config.EnabledSkillIDs)
	parts.KnowledgeDatasetIDs = normalizedSkillIDs(config.KnowledgeDatasetIDs)
	parts.KnowledgeRetrievalConfig = copyStringAnyMap(config.KnowledgeRetrievalConfig)
	parts.AgentMemoryEnabled = config.AgentMemoryEnabled
	parts.AgentMemoryAutoExtractionEnabled = config.AgentMemoryAutoExtractionEnabled
	parts.AgentMemorySlots = normalizeAgentMemorySlots(config.AgentMemorySlots)
	parts.AgentMemoryUserScope = strings.TrimSpace(config.AgentMemoryUserScope)
	parts.ConnectorTargetScope = config.ConnectorTargetScope
	parts.ConnectorTargetVersionID = config.ConnectorTargetVersionID
	parts.ConnectorToolsDisabled = config.ConnectorToolsDisabled
	parts.ConnectorDelegationDisabled = config.ConnectorDelegationDisabled
	parts.ConnectorTargetType = config.ConnectorTargetType
	parts.ConnectorRunID = config.ConnectorRunID
	parts.ConnectorInteractive = config.ConnectorInteractive
	parts.AgentMemoryConfigScope = strings.TrimSpace(config.AgentMemoryConfigScope)
	parts.AgentMemoryConfigRevision = strings.TrimSpace(config.AgentMemoryConfigRevision)
	parts.AgentMemoryInstructions = strings.TrimSpace(config.AgentMemoryInstructions)
	parts.AgentMemoryAgentID = strings.TrimSpace(config.BillingAppID)
	parts.RuntimeTools = append([]skillloop.RuntimeTool(nil), config.RuntimeTools...)
	parts.AttachmentReadAllowed = config.AttachmentReadAuthorizer != nil
	parts.BillingSource = strings.TrimSpace(config.BillingAppType)
}

func applyCallerRuntimeSurfacePolicy(caller Caller, parts *chatRequestParts) {
	if parts == nil {
		return
	}
	parts.Surface = normalizeRuntimeSurfaceForCaller(caller, parts.Surface)
	if (caller.Type == runtimemodel.ConversationCallerAIChat || caller.Type == runtimemodel.ConversationCallerAgent) &&
		(caller.Source == "" || caller.Source == runtimemodel.ConversationSourceConsole) {
		parts.AttachmentReadAllowed = true
	}
}

// applyPersistedConversationSurface keeps a conversation on the runtime surface
// selected when it was created. Later requests may carry stale surface hints,
// but they cannot change the conversation's runtime capabilities. Conversations
// that predate surface metadata keep their legacy behavior and are not migrated.
func applyPersistedConversationSurface(conversation *runtimemodel.Conversation, parts *chatRequestParts) {
	if conversation == nil || parts == nil || conversation.Metadata == nil {
		return
	}
	persisted := strings.TrimSpace(stringMetadataValue(conversation.Metadata["surface"]))
	if persisted == "" {
		return
	}
	parts.Surface = normalizeAIChatSurface(persisted)
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
			Name:        strings.TrimSpace(slot.Name),
			Description: strings.TrimSpace(slot.Description),
			MaxChars:    maxChars,
			Enabled:     slot.Enabled,
			SortOrder:   sortOrder,
			ReadOnly:    slot.ReadOnly,
			ManualOnly:  slot.ManualOnly,
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
		ConversationID:      strings.TrimSpace(req.ConversationID),
		Query:               query,
		Surface:             surface,
		RuntimeContext:      runtimeContext,
		RawOperationContext: copyStringAnyMap(req.OperationContext),
		OperationContext:    operationContext,
		OperationLedger:     operationLedger,
		ModelName:           modelName,
		Provider:            provider,
		ProviderPtr:         providerPtr,
		RoutingStrategy:     normalizeRoutingStrategy(req.RoutingStrategy),
		Parameters:          params,
		SourceMessageID:     cloneUUIDPointer(req.SourceMessageID),
		HistoryPolicy:       normalizeConversationHistoryPolicy(req.HistoryPolicy),
	}, nil
}

func normalizeConversationHistoryPolicy(policy *runtimedto.ConversationHistoryPolicy) *runtimedto.ConversationHistoryPolicy {
	if policy == nil {
		return nil
	}
	mode := strings.ToLower(strings.TrimSpace(policy.Mode))
	if mode != runtimedto.ConversationHistoryModeSupplemental && mode != runtimedto.ConversationHistoryModeIsolated {
		return nil
	}
	maxTurns := min(max(policy.MaxTurns, 0), maxContextCandidateMessages)
	excluded := make([]string, 0, min(len(policy.ExcludedTexts), maxContextCandidateMessages))
	seen := make(map[string]struct{}, cap(excluded))
	for _, value := range policy.ExcludedTexts {
		value = normalizedHistoryContent(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		excluded = append(excluded, value)
		if len(excluded) >= maxContextCandidateMessages {
			break
		}
	}
	return &runtimedto.ConversationHistoryPolicy{Mode: mode, MaxTurns: maxTurns, ExcludedTexts: excluded}
}

func normalizedHistoryContent(value string) string {
	return strings.Join(strings.Fields(value), " ")
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
	routingStrategy := normalizeRoutingStrategy(stringMetadataValue(message.Metadata["routing_strategy"]))
	if req.RoutingStrategy != nil {
		routingStrategy = normalizeRoutingStrategy(*req.RoutingStrategy)
	}

	runtimeContext := normalizeRuntimeContext(req.RuntimeContext)
	surface := normalizeAIChatSurface(regenerateRequestSurface(req, message))
	operationContext, operationLedger := normalizeOperationContext(req.OperationContext)

	return &chatRequestParts{
		ConversationID:      message.ConversationID.String(),
		Query:               query,
		Surface:             surface,
		RuntimeContext:      runtimeContext,
		RawOperationContext: copyStringAnyMap(req.OperationContext),
		OperationContext:    operationContext,
		OperationLedger:     operationLedger,
		ModelName:           modelName,
		Provider:            provider,
		ProviderPtr:         providerPtr,
		RoutingStrategy:     routingStrategy,
		Parameters:          params,
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
	if protocol := strings.TrimSpace(stringMetadataValue(source.Metadata["native_skill_protocol"])); protocol != "" {
		message.Metadata["native_skill_protocol"] = protocol
	}
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
		SourceMessageID:     cloneUUIDPointer(parts.SourceMessageID),
	}
}

func cloneUUIDPointer(value *uuid.UUID) *uuid.UUID {
	if value == nil || *value == uuid.Nil {
		return nil
	}
	cloned := *value
	return &cloned
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
		"routing_strategy":      normalizeRoutingStrategy(parts.RoutingStrategy),
	}
	if mode := normalizeExecutionMode(parts.ExecutionMode); mode != "" {
		metadata["execution_mode"] = mode
		if useCase := executionModeModelUseCase(mode); useCase != "" {
			metadata["model_use_case"] = useCase
		}
	}
	if reason := strings.TrimSpace(parts.ExecutionRouteReason); reason != "" {
		metadata["execution_route_reason"] = reason
	}
	if parts.ProtocolToolsEnabled {
		metadata["protocol_tools_enabled"] = true
	}
	if parts.AgentMemoryToolsEnabled {
		metadata["agent_memory_tools_enabled"] = true
	}
	if parts.ProtocolToolsEnabled || parts.AgentMemoryToolsEnabled || (parts.SkillMode != "" && parts.SkillMode != skillModeDisabled) {
		metadata["has_trace"] = false
		metadata["skill_call_count"] = 0
		metadata["skill_names"] = []interface{}{}
		metadata["tool_call_count"] = 0
		metadata["tool_names"] = []interface{}{}
		metadata["guardrail_count"] = 0
		if parts.SkillMode != "" {
			metadata["skill_mode"] = parts.SkillMode
		}
		metadata["configured_skill_ids"] = append([]string(nil), parts.SkillIDs...)
	}
	if parts.ContextControl != nil {
		metadata["context_control"] = parts.ContextControl
	}
	if status := strings.TrimSpace(parts.ModelCapabilityStatus); status != "" || parts.FunctionCallingAssumed {
		modelCapabilities := map[string]interface{}{
			"function_calling_known":     parts.FunctionCallingKnown,
			"function_calling_supported": parts.ModelSupportsFunctionCalling,
			"function_calling_assumed":   parts.FunctionCallingAssumed,
		}
		if status != "" {
			modelCapabilities["status"] = status
		}
		if errMessage := strings.TrimSpace(parts.ModelCapabilityError); errMessage != "" {
			modelCapabilities["error"] = errMessage
		}
		metadata["model_capabilities"] = modelCapabilities
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
	// The snapshot is stored once, as turn_start_context. Messages from before
	// also carry it as turn_initial_context_snapshot, which is still read
	// (ADR 0040).
	if snapshot := turnInitialContextSnapshot(parts); snapshot != nil {
		metadata[turnStartContextKey] = snapshot
	}
	if parts.ModelTurnIntent != nil {
		metadata["model_turn_intent"] = parts.ModelTurnIntent
		if contract := modelTurnIntentTaskContract(parts.ModelTurnIntent); len(contract) > 0 {
			metadata["turn_task_contract"] = contract
		}
	} else if strings.TrimSpace(parts.ModelTurnIntentError) != "" {
		metadata["model_turn_intent_error"] = strings.TrimSpace(parts.ModelTurnIntentError)
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
	if snapshot := consoleAgentsContextSnapshot(parts); snapshot != nil {
		metadata[consoleAgentsContextSnapshotKey] = snapshot
	}
	if parts.Attachments != nil && len(parts.Attachments.Files) > 0 {
		metadata["files"] = parts.Attachments.metadataFiles()
		metadata["file_count"] = len(parts.Attachments.Files)
	}
	return metadata
}

func normalizeExecutionMode(value string) string {
	switch strings.TrimSpace(value) {
	case executionModeNativeAgentLoop:
		return executionModeNativeAgentLoop
	case executionModeNativeToolLoop:
		return executionModeNativeToolLoop
	case executionModeAgentLoop:
		return executionModeAgentLoop
	case executionModeLegacyToolChat:
		return executionModeLegacyToolChat
	case executionModeDirectChat:
		return executionModeDirectChat
	default:
		return ""
	}
}

func restoreExecutionModeFromMetadata(parts *chatRequestParts, metadata map[string]interface{}) {
	if parts == nil {
		return
	}
	parts.ExecutionMode = normalizeExecutionMode(stringMetadataValue(metadata["execution_mode"]))
	parts.ExecutionRouteReason = executionRoutePersistedMode
	if parts.ExecutionMode == "" {
		// Messages created before execution mode was persisted already entered the
		// Agent loop. Keep their continuation semantics instead of switching the
		// in-flight turn to a different runtime after an upgrade.
		parts.ExecutionMode = executionModeAgentLoop
	}
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
	if isAutonomousSubjectCaller(caller.Type) {
		return aiChatSurfaceExternalPageChat
	}
	return normalizeAIChatSurface(surface)
}

func isContextualAIChatSurface(value string) bool {
	return normalizeAIChatSurface(value) == aiChatSurfaceContextualSidebar
}
