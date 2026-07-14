package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/shared/titlegen"
	"github.com/zgiai/zgi/api/internal/prompt"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

func (s *service) PrepareChat(ctx context.Context, scope Scope, req runtimedto.ChatRequest) (*PreparedChat, error) {
	return s.PrepareConfiguredChat(ctx, scope, Caller{Type: runtimemodel.ConversationCallerAIChat}, RunConfig{
		BillingAppType: runtimemodel.MessageBillingReasonSourceAIChat,
	}, req)
}

func (s *service) PrepareConfiguredChat(ctx context.Context, scope Scope, caller Caller, config RunConfig, req runtimedto.ChatRequest) (*PreparedChat, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}

	req = applyRunConfigToChatRequest(config, req)
	parts, err := normalizeChatRequest(req)
	if err != nil {
		return nil, err
	}
	applyRunConfigToParts(config, parts)
	applyCallerRuntimeSurfacePolicy(caller, parts)
	attachments, err := s.resolveChatAttachmentReferences(ctx, scope, req.FileIDs)
	if err != nil {
		return nil, err
	}
	parts.Attachments = attachments
	if err := s.applyModelCapabilities(ctx, scope, parts); err != nil {
		return nil, err
	}
	applyProtocolToolsPolicy(caller, parts)
	applyManagedUserMemoryPolicy(caller, parts)
	if err := s.applySkillConfig(ctx, scope, caller, &config, parts); err != nil {
		return nil, err
	}
	conversation, err := s.resolveChatConversation(ctx, scope, caller, req, parts)
	if err != nil {
		return nil, err
	}
	if err := s.ensureConversationAllowsNewTurn(ctx, scope, conversation); err != nil {
		return nil, err
	}
	parentID, err := s.resolveParentMessage(ctx, scope, conversation, strings.TrimSpace(req.ParentID))
	if err != nil {
		return nil, err
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
	conversation.RuntimeStatus = runtimemodel.ConversationRuntimeStatusStreaming
	conversation.ActiveMessageID = &message.ID
	s.appendStreamEventBestEffort(ctx, message.ID, conversation.ID, streamEventMessageStart, messageStartPayload(conversation, message, false))

	prepared := &PreparedChat{
		Conversation: conversation,
		Message:      message,
		Scope:        scope,
		Caller:       caller,
		RunConfig:    config,
		ParentID:     parentID,
		parts:        parts,
	}
	s.refreshInitialPageContext(ctx, prepared)
	return prepared, nil
}

func (s *service) PrepareRootRegeneration(ctx context.Context, scope Scope, id uuid.UUID, req runtimedto.RegenerateMessageRequest) (*PreparedChat, error) {
	return s.prepareRootRegeneration(ctx, scope, Caller{Type: runtimemodel.ConversationCallerAIChat}, RunConfig{}, id, req, false)
}

func (s *service) PrepareConfiguredRootRegeneration(ctx context.Context, scope Scope, caller Caller, config RunConfig, id uuid.UUID, req runtimedto.RegenerateMessageRequest) (*PreparedChat, error) {
	return s.prepareRootRegeneration(ctx, scope, caller, config, id, req, true)
}

func (s *service) prepareRootRegeneration(ctx context.Context, scope Scope, caller Caller, config RunConfig, id uuid.UUID, req runtimedto.RegenerateMessageRequest, callerScoped bool) (*PreparedChat, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	message, err := s.repos.Message.GetScoped(ctx, id, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	var conversation *runtimemodel.Conversation
	if callerScoped {
		conversation, err = s.getConversationByCallerScoped(ctx, scope, caller, message.ConversationID)
		if err != nil {
			return nil, err
		}
	} else {
		conversation, err = s.getConversation(ctx, scope, message.ConversationID)
		if err != nil {
			return nil, err
		}
	}
	if err := s.ensureConversationAllowsNewTurn(ctx, scope, conversation); err != nil {
		return nil, err
	}
	req = applyRunConfigToRegenerateRequest(config, req)
	parts, err := normalizeRegenerateRequest(req, message)
	if err != nil {
		return nil, err
	}
	applyRunConfigToParts(config, parts)
	applyCallerRuntimeSurfacePolicy(caller, parts)
	if err := applyCanonicalConversationSurface(conversation, parts); err != nil {
		return nil, err
	}
	parts.Attachments = attachmentBundleFromMessageMetadata(message.Metadata)
	if err := s.applyModelCapabilities(ctx, scope, parts); err != nil {
		return nil, err
	}
	applyProtocolToolsPolicy(caller, parts)
	applyManagedUserMemoryPolicy(caller, parts)
	if err := s.applySkillConfig(ctx, scope, caller, &config, parts); err != nil {
		return nil, err
	}
	contextResult, err := s.buildUpstreamMessages(ctx, scope, nil, parts)
	if err != nil {
		return nil, err
	}
	parts.ContextControl = contextResult.Metadata
	llmRequest := newLLMChatRequest(parts, contextResult.Messages)
	preflight, err := s.runContextualPreparePreflights(ctx, scope, conversation, config, parts, llmRequest)
	if err != nil {
		return nil, err
	}
	replacement := replacementRootMessage(message, parts)
	if err := s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepos := repository.NewRepositories(tx)
		count, err := txRepos.Message.CountByConversation(ctx, conversation.ID)
		if err != nil {
			return err
		}
		if conversation.RuntimeStatus == runtimemodel.ConversationRuntimeStatusStreaming {
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
	conversation.RuntimeStatus = runtimemodel.ConversationRuntimeStatusStreaming
	conversation.ActiveMessageID = &message.ID
	s.resetStreamEventsBestEffort(ctx, message.ID)
	s.appendStreamEventBestEffort(ctx, message.ID, conversation.ID, streamEventMessageStart, messageStartPayload(conversation, replacement, true))
	return &PreparedChat{
		Conversation: conversation,
		Message:      replacement,
		LLMRequest:   llmRequest,
		ReplaceRoot:  true,
		Scope:        scope,
		Caller:       caller,
		RunConfig:    config,
		parts:        parts,

		UserMemoryPreflightDone:  preflight != nil && preflight.UserMemoryDone,
		UserMemoryPreflightUsage: contextualPreflightUserMemoryUsage(preflight),
	}, nil
}

func contextualPreflightUserMemoryUsage(preflight *contextualPreparePreflightResult) *adapter.Usage {
	if preflight == nil {
		return nil
	}
	return preflight.UserMemoryUsage
}

func (s *service) ensureMember(ctx context.Context, scope Scope) error {
	if scope.OrganizationID == uuid.Nil || scope.AccountID == uuid.Nil {
		return ErrUnauthorized
	}
	if scope.SkipAccessCheck {
		return nil
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

func (s *service) ensureConversationAllowsNewTurn(ctx context.Context, scope Scope, conversation *runtimemodel.Conversation) error {
	if conversation == nil || conversation.CurrentLeafMessageID == nil {
		return nil
	}
	if conversation.RuntimeStatus != runtimemodel.ConversationRuntimeStatusIdle {
		return nil
	}
	leafMessage, err := s.repos.Message.GetScoped(ctx, *conversation.CurrentLeafMessageID, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return mapRepoError(err)
	}
	if leafMessage.Status == runtimemodel.MessageStatusWaitingApproval {
		return ErrConversationWaitingApproval
	}
	if leafMessage.Status == runtimemodel.MessageStatusWaitingQuestion {
		return ErrConversationWaitingQuestion
	}
	if leafMessage.Status == runtimemodel.MessageStatusWaitingClientAction {
		return ErrConversationWaitingAction
	}
	return nil
}

func (s *service) getConversation(ctx context.Context, scope Scope, id uuid.UUID) (*runtimemodel.Conversation, error) {
	conversation, err := s.repos.Conversation.GetScoped(ctx, id, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	return conversation, nil
}

func (s *service) resolveChatConversation(ctx context.Context, scope Scope, caller Caller, req runtimedto.ChatRequest, parts *chatRequestParts) (*runtimemodel.Conversation, error) {
	if strings.TrimSpace(req.ConversationID) == "" {
		return s.createConversationForChat(ctx, scope, caller, parts)
	}
	conversationID, err := uuid.Parse(strings.TrimSpace(req.ConversationID))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid conversation_id", ErrInvalidInput)
	}
	conversation, err := s.getConversationByCallerScoped(ctx, scope, caller, conversationID)
	if err != nil {
		return nil, err
	}
	if err := applyCanonicalConversationSurface(conversation, parts); err != nil {
		return nil, err
	}
	return conversation, nil
}

func (s *service) createConversationForChat(ctx context.Context, scope Scope, caller Caller, parts *chatRequestParts) (*runtimemodel.Conversation, error) {
	query := ""
	surface := ""
	if parts != nil {
		query = parts.Query
		surface = parts.Surface
	}
	initialTitle := conversationTitleFallback(query, initialConversationTitle())
	conversation, err := s.createConversationForCaller(ctx, scope, caller, initialTitle, surface)
	if err != nil {
		return nil, err
	}
	if s.titleGen == nil || normalizeCallerType(caller.Type) == runtimemodel.ConversationCallerAgent {
		return conversation, nil
	}
	s.generateConversationTitleAsync(ctx, scope, conversation, parts, initialTitle)
	return conversation, nil
}

func (s *service) generateConversationTitleAsync(ctx context.Context, scope Scope, conversation *runtimemodel.Conversation, parts *chatRequestParts, initialTitle string) {
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
			AppType:           runtimemodel.MessageBillingReasonSourceAIChat,
			SessionID:         conversationID.String(),
			ConversationID:    conversationID.String(),
			Messages:          []titlegen.Message{{Role: "user", Content: query}},
			FallbackTitle:     initialTitle,
			PreferredProvider: preferredProvider,
			PreferredModel:    preferredModel,
			PreferredUseCase:  string(llmmodelmodel.UseCaseTextChat),
		})
		if err != nil {
			logger.WarnContext(titleCtx, "failed to generate aichat conversation title", "conversation_id", conversationID.String(), err)
			return
		}
		title := normalizeTitle(result.Title, initialTitle)
		if title == initialTitle {
			return
		}
		if err := s.repos.Conversation.UpdateScoped(titleCtx, conversationID, scope.OrganizationID, scope.AccountID, map[string]interface{}{"title": title}); err != nil {
			logger.WarnContext(titleCtx, "failed to update generated aichat conversation title", "conversation_id", conversationID.String(), err)
		}
	}()
}

func (s *service) resolveParentMessage(ctx context.Context, scope Scope, conversation *runtimemodel.Conversation, parentIDRaw string) (*uuid.UUID, error) {
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
	systemPrompt := strings.TrimSpace(parts.SystemPrompt)
	if systemPrompt == "" {
		rendered, err := renderAIChatSystemPrompt(parts.Surface)
		if err != nil {
			return nil, err
		}
		systemPrompt = rendered
	}
	systemPrompt, memoryMetadata, err := s.appendUserMemoryContext(ctx, scope, parts, systemPrompt)
	if err != nil {
		return nil, err
	}
	systemPrompt = appendAgentMemoryPolicy(systemPrompt, parts)
	systemPrompt, agentMemoryMetadata, err := s.appendAgentMemoryContext(ctx, scope, parts, systemPrompt)
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
			applyRecentAssetCandidatesFromBranch(parts, branch)
			applyRecentGeneratedArtifactsFromBranch(parts, branch)
			applyRecentOperationPlansFromBranch(parts, branch)
			result, err := s.buildTokenBudgetMessages(ctx, spec, parts, systemPrompt, branch)
			if err != nil {
				return nil, err
			}
			result.Metadata = mergeUserMemoryMetadata(result.Metadata, memoryMetadata)
			result.Metadata = mergeUserMemoryMetadata(result.Metadata, agentMemoryMetadata)
			return result, nil
		}
	}
	currentContent, contextMetadata := s.buildFallbackCurrentUserContent(parts)
	messages := []adapter.Message{{Role: "system", Content: systemPrompt}}
	contextMetadata = mergeUserMemoryMetadata(contextMetadata, memoryMetadata)
	contextMetadata = mergeUserMemoryMetadata(contextMetadata, agentMemoryMetadata)
	if parentID != nil && *parentID != uuid.Nil {
		branch, err := s.repos.Message.ListBranch(ctx, *parentID, maxContextMessages)
		if err != nil {
			return nil, err
		}
		if !shouldIsolateHistoryForCurrentTurn(parts) {
			for _, item := range branch {
				if item == nil {
					continue
				}
				userMessage, err := s.historicalUserMessage(ctx, item, parts.ModelSupportsVision)
				if err != nil {
					return nil, err
				}
				if userMessage != nil {
					messages = append(messages, *userMessage)
				}
				if isUsableAssistantHistoryStatus(item.Status) && strings.TrimSpace(item.Answer) != "" {
					messages = append(messages, adapter.Message{Role: "assistant", Content: item.Answer})
				}
			}
		}
		applyRecentOperationPlansFromBranch(parts, branch)
		if recentExecutionContext, recentExecutionMetadata := buildRecentExecutionContextMessageForRequest(parts, branch); recentExecutionContext != nil {
			messages = append(messages, *recentExecutionContext)
			if contextMetadata == nil {
				contextMetadata = map[string]interface{}{}
			}
			mergeRecentExecutionContextMetadata(contextMetadata, recentExecutionMetadata)
		}
		if continuationContext := buildContinuationTaskStateMessage(parts, branch); continuationContext != nil {
			messages = append(messages, *continuationContext)
			if contextMetadata == nil {
				contextMetadata = map[string]interface{}{}
			}
			contextMetadata["continuation_task_state_included"] = true
		}
		if turnBoundaryContext := currentTurnBoundaryMessage(parts); turnBoundaryContext != nil {
			messages = append(messages, *turnBoundaryContext)
		}
		applyRecentAssetCandidatesFromBranch(parts, branch)
		applyRecentGeneratedArtifactsFromBranch(parts, branch)
	}
	messages = append(messages, adapter.Message{Role: "user", Content: currentContent})
	return &contextBudgetResult{Messages: messages, Metadata: contextMetadata}, nil
}

func (s *service) applyModelCapabilities(ctx context.Context, scope Scope, parts *chatRequestParts) error {
	if parts == nil {
		return nil
	}
	if s.modelSpecResolver == nil {
		return fmt.Errorf("resolve AI Chat model capabilities: resolver is unavailable")
	}
	spec, ok, err := s.modelSpecResolver.Resolve(ctx, scope.OrganizationID, parts.Provider, parts.ModelName)
	if err != nil {
		return fmt.Errorf("resolve AI Chat model capabilities: %w", err)
	}
	if !ok {
		return fmt.Errorf("resolve AI Chat model capabilities: model %s/%s was not found", parts.Provider, parts.ModelName)
	}
	if !spec.SupportsFunctionCalling() {
		return fmt.Errorf("resolve AI Chat model capabilities: model %s/%s does not support function calling", parts.Provider, parts.ModelName)
	}
	parts.ModelSupportsVision = spec.SupportsVision()
	parts.FunctionCallingKnown = true
	parts.ModelSupportsFunctionCalling = spec.SupportsFunctionCalling()
	parts.FunctionCallingAssumed = false
	parts.ModelCapabilityStatus = "resolved"
	parts.ModelCapabilityError = ""
	return nil
}

func applyManagedUserMemoryPolicy(caller Caller, parts *chatRequestParts) {
	if parts == nil {
		return
	}
	if normalizeCallerType(caller.Type) == runtimemodel.ConversationCallerAgent {
		parts.UseMemory = false
		return
	}
	parts.UseMemory = parts.FunctionCallingKnown && parts.ModelSupportsFunctionCalling
}

func applyProtocolToolsPolicy(caller Caller, parts *chatRequestParts) {
	if parts == nil {
		return
	}
	parts.ProtocolToolsEnabled = normalizeCallerType(caller.Type) == runtimemodel.ConversationCallerAgent &&
		parts.FunctionCallingKnown && parts.ModelSupportsFunctionCalling && !parts.FunctionCallingAssumed
}

func (s *service) applyOrganizationSkillConfig(ctx context.Context, scope Scope, parts *chatRequestParts) error {
	return s.applySkillConfig(ctx, scope, Caller{Type: runtimemodel.ConversationCallerAIChat}, nil, parts)
}

func (s *service) applySkillConfig(ctx context.Context, scope Scope, caller Caller, config *RunConfig, parts *chatRequestParts) error {
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
	callerType := normalizeCallerType(caller.Type)
	var enabled []string
	if callerType == runtimemodel.ConversationCallerAgent {
		enabled = effectiveAgentSkillIDs(parts.ConfiguredSkillIDs, catalog, config)
	} else {
		orgEnabled, err := s.effectiveOrganizationSkillIDs(ctx, scope.OrganizationID, catalog)
		if err != nil {
			return err
		}
		if parts.ConfiguredSkillIDs == nil {
			defaultEnabled, _, err := s.effectiveAccountSkillPreferenceIDs(ctx, scope, callerType, catalog, orgEnabled)
			if err != nil {
				return err
			}
			enabled = defaultEnabled
		} else {
			enabled = effectiveSkillIDsForCaller(parts.ConfiguredSkillIDs, catalog, orgEnabled, callerType, config)
		}
		enabled = filterAIChatSkillIDsForSurface(enabled, parts)
		trustedCapabilities := s.trustedContextualAIChatSkillCapabilities(ctx, scope, parts)
		enabled = addContextualAIChatSkillIDsWithCapabilities(enabled, orgEnabled, catalog, parts, trustedCapabilities)
	}
	parts.SkillIDs, parts.ToolSkillIDs = filterSkillsForModel(enabled, catalog, parts)
	if len(parts.SkillIDs) == 0 {
		parts.SkillMode = skillModeDisabled
		return nil
	}
	parts.SkillMode = skillModeAuto
	return nil
}

func (s *service) loadContextBranch(ctx context.Context, parentID *uuid.UUID, maxDepth int) ([]*runtimemodel.Message, error) {
	if parentID == nil || *parentID == uuid.Nil {
		return []*runtimemodel.Message{}, nil
	}
	return s.repos.Message.ListBranch(ctx, *parentID, maxDepth)
}

func renderAIChatSystemPrompt(surface string) (string, error) {
	tmpl, err := prompt.GetTemplate(prompt.AIChatSystem)
	if err != nil {
		return "", err
	}
	return tmpl.Render(map[string]interface{}{
		"Surface": normalizeAIChatSurface(surface),
	})
}

func (s *service) appendUserMemoryContext(ctx context.Context, scope Scope, parts *chatRequestParts, systemPrompt string) (string, map[string]interface{}, error) {
	if parts == nil || !parts.UseMemory {
		return systemPrompt, nil, nil
	}
	if s.memoryService == nil {
		return systemPrompt, map[string]interface{}{"user_memory": map[string]interface{}{"enabled": true, "available": false}}, nil
	}
	if manager, ok := s.memoryService.(interface {
		EnsureRuntimeEnabled(context.Context, uuid.UUID) error
	}); ok {
		if err := manager.EnsureRuntimeEnabled(ctx, scope.AccountID); err != nil {
			return "", nil, err
		}
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
