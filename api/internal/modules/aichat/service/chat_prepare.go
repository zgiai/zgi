package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	aichatdto "github.com/zgiai/zgi/api/internal/modules/aichat/dto"
	aichatmodel "github.com/zgiai/zgi/api/internal/modules/aichat/model"
	"github.com/zgiai/zgi/api/internal/modules/aichat/repository"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/shared/titlegen"
	"github.com/zgiai/zgi/api/internal/prompt"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

func (s *service) PrepareChat(ctx context.Context, scope Scope, req aichatdto.ChatRequest) (*PreparedChat, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}

	parts, err := normalizeChatRequest(req)
	if err != nil {
		return nil, err
	}
	attachments, err := s.resolveChatAttachmentReferences(ctx, scope, req.FileIDs)
	if err != nil {
		return nil, err
	}
	parts.Attachments = attachments
	if err := s.applyModelCapabilities(ctx, scope, parts); err != nil {
		return nil, err
	}
	if err := s.applyOrganizationSkillConfig(ctx, scope, parts); err != nil {
		return nil, err
	}
	conversation, err := s.resolveChatConversation(ctx, scope, req, parts)
	if err != nil {
		return nil, err
	}
	parentID, err := s.resolveParentMessage(ctx, scope, conversation, strings.TrimSpace(req.ParentID))
	if err != nil {
		return nil, err
	}
	var llmRequest *adapter.ChatRequest
	if parts.Attachments == nil || len(parts.Attachments.Files) == 0 {
		contextResult, err := s.buildUpstreamMessages(ctx, scope, parentID, parts)
		if err != nil {
			return nil, err
		}
		parts.ContextControl = contextResult.Metadata
		llmRequest = newLLMChatRequest(parts, contextResult.Messages)
	}

	message := newStreamingMessage(conversation.ID, parentID, parts)
	if err := s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepos := repository.NewRepositories(tx)
		if err := txRepos.Message.Create(ctx, message); err != nil {
			return err
		}
		if err := txRepos.Conversation.StartStreaming(ctx, conversation.ID, scope.OrganizationID, scope.AccountID, message.ID); err != nil {
			return err
		}
		return nil
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrConversationRunning
		}
		return nil, err
	}
	conversation.RuntimeStatus = aichatmodel.ConversationRuntimeStatusStreaming
	conversation.ActiveMessageID = &message.ID
	s.appendStreamEventBestEffort(ctx, message.ID, conversation.ID, streamEventMessageStart, messageStartPayload(conversation, message, false))

	return &PreparedChat{
		Conversation: conversation,
		Message:      message,
		LLMRequest:   llmRequest,
		Scope:        scope,
		ParentID:     parentID,
		parts:        parts,
	}, nil
}

func (s *service) PrepareRootRegeneration(ctx context.Context, scope Scope, id uuid.UUID, req aichatdto.RegenerateMessageRequest) (*PreparedChat, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	message, err := s.repos.Message.GetScoped(ctx, id, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	conversation, err := s.getConversation(ctx, scope, message.ConversationID)
	if err != nil {
		return nil, err
	}
	parts, err := normalizeRegenerateRequest(req, message)
	if err != nil {
		return nil, err
	}
	parts.Attachments = attachmentBundleFromMessageMetadata(message.Metadata)
	if err := s.applyModelCapabilities(ctx, scope, parts); err != nil {
		return nil, err
	}
	if err := s.applyOrganizationSkillConfig(ctx, scope, parts); err != nil {
		return nil, err
	}
	contextResult, err := s.buildUpstreamMessages(ctx, scope, nil, parts)
	if err != nil {
		return nil, err
	}
	parts.ContextControl = contextResult.Metadata
	replacement := replacementRootMessage(message, parts)
	if err := s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepos := repository.NewRepositories(tx)
		count, err := txRepos.Message.CountByConversation(ctx, conversation.ID)
		if err != nil {
			return err
		}
		if conversation.RuntimeStatus == aichatmodel.ConversationRuntimeStatusStreaming {
			return ErrConversationRunning
		}
		if !canReplaceOnlyRootMessage(conversation, message, count) {
			return ErrMessageReplaceNotAllowed
		}
		if err := txRepos.Message.ReplaceRootForStreaming(ctx, replacement); err != nil {
			return err
		}
		if err := txRepos.Conversation.StartStreaming(ctx, conversation.ID, scope.OrganizationID, scope.AccountID, message.ID); err != nil {
			return err
		}
		return nil
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrConversationRunning
		}
		return nil, err
	}
	conversation.RuntimeStatus = aichatmodel.ConversationRuntimeStatusStreaming
	conversation.ActiveMessageID = &message.ID
	s.resetStreamEventsBestEffort(ctx, message.ID)
	s.appendStreamEventBestEffort(ctx, message.ID, conversation.ID, streamEventMessageStart, messageStartPayload(conversation, replacement, true))
	return &PreparedChat{
		Conversation: conversation,
		Message:      replacement,
		LLMRequest:   newLLMChatRequest(parts, contextResult.Messages),
		ReplaceRoot:  true,
		Scope:        scope,
		parts:        parts,
	}, nil
}

func (s *service) ensureMember(ctx context.Context, scope Scope) error {
	if scope.OrganizationID == uuid.Nil || scope.AccountID == uuid.Nil {
		return ErrUnauthorized
	}
	ok, err := s.repos.Access.IsOrganizationMember(ctx, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrPermissionDenied
	}
	return nil
}

func (s *service) resolveWorkspaceID(ctx context.Context, scope Scope) (*uuid.UUID, error) {
	if scope.WorkspaceID != nil && *scope.WorkspaceID != uuid.Nil {
		return scope.WorkspaceID, nil
	}
	return s.repos.Access.GetCurrentWorkspaceID(ctx, scope.AccountID)
}

func (s *service) getConversation(ctx context.Context, scope Scope, id uuid.UUID) (*aichatmodel.Conversation, error) {
	conversation, err := s.repos.Conversation.GetScoped(ctx, id, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	return conversation, nil
}

func (s *service) resolveChatConversation(ctx context.Context, scope Scope, req aichatdto.ChatRequest, parts *chatRequestParts) (*aichatmodel.Conversation, error) {
	if strings.TrimSpace(req.ConversationID) == "" {
		return s.createConversationForChat(ctx, scope, parts)
	}
	conversationID, err := uuid.Parse(strings.TrimSpace(req.ConversationID))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid conversation_id", ErrInvalidInput)
	}
	return s.getConversation(ctx, scope, conversationID)
}

func (s *service) createConversationForChat(ctx context.Context, scope Scope, parts *chatRequestParts) (*aichatmodel.Conversation, error) {
	fallbackTitle := initialConversationTitle()
	conversation, err := s.CreateConversation(ctx, scope, fallbackTitle)
	if err != nil {
		return nil, err
	}
	if s.titleGen == nil {
		return conversation, nil
	}
	s.generateConversationTitleAsync(ctx, scope, conversation, parts, fallbackTitle)
	return conversation, nil
}

func (s *service) generateConversationTitleAsync(ctx context.Context, scope Scope, conversation *aichatmodel.Conversation, parts *chatRequestParts, fallbackTitle string) {
	if conversation == nil || s.titleGen == nil {
		return
	}
	query := ""
	preferredProvider := ""
	preferredModel := ""
	if parts != nil {
		query = parts.Query
		preferredProvider = parts.Provider
		preferredModel = parts.ModelName
	}
	conversationID := conversation.ID
	workspaceID := conversation.WorkspaceID
	go func() {
		titleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), titleGenerationTimeout)
		defer cancel()

		result, err := s.titleGen.Generate(titleCtx, titlegen.GenerateRequest{
			OrganizationID:    scope.OrganizationID,
			AccountID:         scope.AccountID,
			WorkspaceID:       workspaceID,
			AppID:             conversationID.String(),
			AppType:           aichatmodel.MessageBillingReasonSourceAIChat,
			SessionID:         conversationID.String(),
			ConversationID:    conversationID.String(),
			Messages:          []titlegen.Message{{Role: "user", Content: query}},
			FallbackTitle:     fallbackTitle,
			PreferredProvider: preferredProvider,
			PreferredModel:    preferredModel,
			PreferredUseCase:  string(llmmodelmodel.UseCaseTextChat),
		})
		if err != nil {
			logger.WarnContext(titleCtx, "failed to generate aichat conversation title", "conversation_id", conversationID.String(), err)
			return
		}
		title := normalizeTitle(result.Title, fallbackTitle)
		if title == fallbackTitle {
			return
		}
		if err := s.repos.Conversation.UpdateScoped(titleCtx, conversationID, scope.OrganizationID, scope.AccountID, map[string]interface{}{"title": title}); err != nil {
			logger.WarnContext(titleCtx, "failed to update generated aichat conversation title", "conversation_id", conversationID.String(), err)
		}
	}()
}

func (s *service) resolveParentMessage(ctx context.Context, scope Scope, conversation *aichatmodel.Conversation, parentIDRaw string) (*uuid.UUID, error) {
	if conversation == nil {
		return nil, ErrConversationMissing
	}
	if parentIDRaw == "" && conversation.CurrentLeafMessageID != nil {
		parentID := *conversation.CurrentLeafMessageID
		return &parentID, nil
	}
	if parentIDRaw == "" {
		return nil, nil
	}
	parentID, err := uuid.Parse(parentIDRaw)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid parent_id", ErrInvalidInput)
	}
	parent, err := s.repos.Message.GetScoped(ctx, parentID, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	if parent.ConversationID != conversation.ID {
		return nil, fmt.Errorf("%w: parent message belongs to another conversation", ErrInvalidInput)
	}
	return &parentID, nil
}

func (s *service) buildUpstreamMessages(ctx context.Context, scope Scope, parentID *uuid.UUID, parts *chatRequestParts) (*contextBudgetResult, error) {
	systemPrompt, err := renderAIChatSystemPrompt()
	if err != nil {
		return nil, err
	}
	systemPrompt, memoryMetadata, err := s.appendUserMemoryContext(ctx, scope, parts, systemPrompt)
	if err != nil {
		return nil, err
	}
	if s.modelSpecResolver != nil {
		spec, ok, err := s.modelSpecResolver.Resolve(ctx, scope.OrganizationID, parts.Provider, parts.ModelName)
		if err != nil {
			return nil, err
		}
		if ok {
			parts.ModelSupportsVision = spec.SupportsVision()
		}
		if ok && spec.ContextWindow > 0 {
			branch, err := s.loadContextBranch(ctx, parentID, maxContextCandidateMessages)
			if err != nil {
				return nil, err
			}
			result, err := s.buildTokenBudgetMessages(ctx, spec, parts, systemPrompt, branch)
			if err != nil {
				return nil, err
			}
			result.Metadata = mergeUserMemoryMetadata(result.Metadata, memoryMetadata)
			return result, nil
		}
	}
	currentContent, contextMetadata := s.buildFallbackCurrentUserContent(parts)
	messages := []adapter.Message{{Role: "system", Content: systemPrompt}}
	contextMetadata = mergeUserMemoryMetadata(contextMetadata, memoryMetadata)
	if parentID != nil && *parentID != uuid.Nil {
		branch, err := s.repos.Message.ListBranch(ctx, *parentID, maxContextMessages)
		if err != nil {
			return nil, err
		}
		for _, item := range branch {
			if item == nil {
				continue
			}
			userMessage := s.historicalUserMessage(ctx, item, parts.ModelSupportsVision)
			if userMessage != nil {
				messages = append(messages, *userMessage)
			}
			if isUsableAssistantHistoryStatus(item.Status) && strings.TrimSpace(item.Answer) != "" {
				messages = append(messages, adapter.Message{Role: "assistant", Content: item.Answer})
			}
		}
		if recentExecutionContext, recentExecutionMetadata := buildRecentExecutionContextMessage(branch); recentExecutionContext != nil {
			messages = append(messages, *recentExecutionContext)
			if contextMetadata == nil {
				contextMetadata = map[string]interface{}{}
			}
			mergeRecentExecutionContextMetadata(contextMetadata, recentExecutionMetadata)
		}
	}
	messages = append(messages, adapter.Message{Role: "user", Content: currentContent})
	return &contextBudgetResult{Messages: messages, Metadata: contextMetadata}, nil
}

func (s *service) applyModelCapabilities(ctx context.Context, scope Scope, parts *chatRequestParts) error {
	if parts == nil || s.modelSpecResolver == nil {
		return nil
	}
	spec, ok, err := s.modelSpecResolver.Resolve(ctx, scope.OrganizationID, parts.Provider, parts.ModelName)
	if err != nil {
		return err
	}
	parts.ModelSupportsVision = ok && spec.SupportsVision()
	parts.FunctionCallingKnown = ok
	parts.ModelSupportsFunctionCalling = ok && spec.SupportsFunctionCalling()
	return nil
}

func (s *service) applyOrganizationSkillConfig(ctx context.Context, scope Scope, parts *chatRequestParts) error {
	if parts == nil {
		return nil
	}
	if s.skillRuntime == nil {
		parts.SkillMode = skillModeDisabled
		parts.SkillIDs = nil
		parts.ToolSkillIDs = nil
		logger.WarnContext(ctx, "aichat skills disabled because skill runtime is not configured",
			"organization_id", scope.OrganizationID.String(),
		)
		return nil
	}
	if !parts.FunctionCallingKnown || !parts.ModelSupportsFunctionCalling {
		parts.SkillMode = skillModeDisabled
		parts.SkillIDs = nil
		parts.ToolSkillIDs = nil
		logger.DebugContext(ctx, "aichat skills skipped because model function calling is unsupported or unknown",
			"organization_id", scope.OrganizationID.String(),
			"provider", parts.Provider,
			"model", parts.ModelName,
			"function_calling_known", parts.FunctionCallingKnown,
			"supports_function_calling", parts.ModelSupportsFunctionCalling,
		)
		return nil
	}
	catalog, err := s.catalogSkillMetadata(ctx, scope.OrganizationID)
	if err != nil {
		return err
	}
	catalog = visibleSkillMetadata(catalog)
	enabled, err := s.effectiveOrganizationSkillIDs(ctx, scope.OrganizationID, catalog)
	if err != nil {
		return err
	}
	parts.SkillIDs, parts.ToolSkillIDs = filterSkillsForModel(enabled, catalog, parts)
	if len(parts.SkillIDs) == 0 {
		parts.SkillMode = skillModeDisabled
		return nil
	}
	parts.SkillMode = skillModeAuto
	return nil
}

func (s *service) loadContextBranch(ctx context.Context, parentID *uuid.UUID, maxDepth int) ([]*aichatmodel.Message, error) {
	if parentID == nil || *parentID == uuid.Nil {
		return []*aichatmodel.Message{}, nil
	}
	return s.repos.Message.ListBranch(ctx, *parentID, maxDepth)
}

func renderAIChatSystemPrompt() (string, error) {
	tmpl, err := prompt.GetTemplate(prompt.AIChatSystem)
	if err != nil {
		return "", err
	}
	return tmpl.Render(map[string]interface{}{
		"Surface": "work_chat",
	})
}

func isUsableAssistantHistoryStatus(status string) bool {
	return status == aichatmodel.MessageStatusCompleted || status == aichatmodel.MessageStatusStopped
}

func normalizeChatRequest(req aichatdto.ChatRequest) (*chatRequestParts, error) {
	query := strings.TrimSpace(req.Query)
	runtimeContext := normalizeRuntimeContext(req.RuntimeContext)
	operationContext := copyStringAnyMap(req.OperationContext)
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
		RuntimeContext:      runtimeContext,
		RawOperationContext: operationContext,
		OperationContext:    operationContext,
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

func normalizeRegenerateRequest(req aichatdto.RegenerateMessageRequest, message *aichatmodel.Message) (*chatRequestParts, error) {
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

	return &chatRequestParts{
		Query:       query,
		ModelName:   modelName,
		Provider:    provider,
		ProviderPtr: providerPtr,
		Parameters:  params,
		UseMemory:   useMemory,
	}, nil
}

func replacementRootMessage(source *aichatmodel.Message, parts *chatRequestParts) *aichatmodel.Message {
	message := newStreamingMessage(source.ConversationID, nil, parts)
	message.ID = source.ID
	message.CreatedAt = source.CreatedAt
	message.UpdatedAt = time.Now()
	return message
}

func canReplaceOnlyRootMessage(conversation *aichatmodel.Conversation, message *aichatmodel.Message, messageCount int64) bool {
	if conversation == nil || message == nil {
		return false
	}
	if conversation.RuntimeStatus == aichatmodel.ConversationRuntimeStatusStreaming {
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

func newStreamingMessage(conversationID uuid.UUID, parentID *uuid.UUID, parts *chatRequestParts) *aichatmodel.Message {
	billingReasonSource := aichatmodel.MessageBillingReasonSourceAIChat
	return &aichatmodel.Message{
		ConversationID:      conversationID,
		ParentID:            parentID,
		Query:               parts.Query,
		Status:              aichatmodel.MessageStatusStreaming,
		ModelProvider:       parts.ProviderPtr,
		ModelName:           parts.ModelName,
		BillingReasonSource: &billingReasonSource,
		ModelParameters:     parts.Parameters,
		Metadata:            streamingMessageMetadata(parts),
	}
}

func streamingMessageMetadata(parts *chatRequestParts) map[string]interface{} {
	metadata := map[string]interface{}{
		"system_prompt_version": systemPromptVersion,
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
	if parts.Attachments != nil && len(parts.Attachments.Files) > 0 {
		metadata["files"] = parts.Attachments.metadataFiles()
		metadata["file_count"] = len(parts.Attachments.Files)
	}
	return metadata
}

func (s *service) appendUserMemoryContext(ctx context.Context, scope Scope, parts *chatRequestParts, systemPrompt string) (string, map[string]interface{}, error) {
	if parts == nil || !parts.UseMemory {
		return systemPrompt, nil, nil
	}
	if s.memoryService == nil {
		return systemPrompt, map[string]interface{}{"user_memory": map[string]interface{}{"enabled": true, "available": false}}, nil
	}
	enabled, err := s.memoryService.IsEnabled(ctx, scope.AccountID)
	if err != nil {
		return "", nil, err
	}
	if !enabled {
		return systemPrompt, map[string]interface{}{"user_memory": map[string]interface{}{"enabled": false, "available": false}}, nil
	}
	rendered, err := s.memoryService.RenderContext(ctx, scope.AccountID, userMemoryContextBudgetChars)
	if err != nil {
		return "", nil, err
	}
	metadata := map[string]interface{}{
		"user_memory": map[string]interface{}{
			"enabled":   true,
			"available": strings.TrimSpace(rendered) != "",
		},
	}
	if strings.TrimSpace(rendered) == "" {
		return systemPrompt, metadata, nil
	}
	return strings.TrimSpace(systemPrompt) + "\n\n" + rendered, metadata, nil
}

func (s *service) isUserMemoryEnabled(ctx context.Context, accountID uuid.UUID) (bool, error) {
	if s.memoryService == nil {
		return false, nil
	}
	return s.memoryService.IsEnabled(ctx, accountID)
}

func boolMetadata(metadata map[string]interface{}, key string) bool {
	if metadata == nil {
		return false
	}
	value, ok := metadata[key]
	if !ok {
		return false
	}
	typed, ok := value.(bool)
	return ok && typed
}

func mergeUserMemoryMetadata(metadata map[string]interface{}, memoryMetadata map[string]interface{}) map[string]interface{} {
	if len(memoryMetadata) == 0 {
		return metadata
	}
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	for key, value := range memoryMetadata {
		metadata[key] = value
	}
	return metadata
}

func newLLMChatRequest(parts *chatRequestParts, messages []adapter.Message) *adapter.ChatRequest {
	req := &adapter.ChatRequest{
		Provider: parts.Provider,
		Model:    parts.ModelName,
		Messages: messages,
		Stream:   true,
	}
	applyModelParameters(req, parts.Parameters)
	return req
}

func normalizeModelParameters(input map[string]interface{}) (map[string]interface{}, error) {
	output := make(map[string]interface{})
	for key, value := range input {
		switch key {
		case "temperature", "top_p", "presence_penalty", "frequency_penalty":
			v, ok := floatValue(value)
			if !ok {
				return nil, fmt.Errorf("%w: %s must be a number", ErrInvalidModelParam, key)
			}
			output[key] = v
		case "max_tokens", "seed":
			v, ok := intValue(value)
			if !ok {
				return nil, fmt.Errorf("%w: %s must be an integer", ErrInvalidModelParam, key)
			}
			output[key] = v
		case "stop":
			v, ok := stringSliceValue(value)
			if !ok {
				return nil, fmt.Errorf("%w: stop must be a string array", ErrInvalidModelParam)
			}
			output[key] = v
		default:
			return nil, fmt.Errorf("%w: unsupported parameter %s", ErrInvalidModelParam, key)
		}
	}
	return output, nil
}

func applyModelParameters(req *adapter.ChatRequest, params map[string]interface{}) {
	if value, ok := params["temperature"].(float64); ok {
		req.Temperature = &value
	}
	if value, ok := params["top_p"].(float64); ok {
		req.TopP = &value
	}
	if value, ok := params["presence_penalty"].(float64); ok {
		req.PresencePenalty = &value
	}
	if value, ok := params["frequency_penalty"].(float64); ok {
		req.FrequencyPenalty = &value
	}
	if value, ok := params["max_tokens"].(int); ok {
		req.MaxTokens = &value
	}
	if value, ok := params["seed"].(int); ok {
		req.Seed = &value
	}
	if value, ok := params["stop"].([]string); ok {
		req.Stop = value
	}
}
