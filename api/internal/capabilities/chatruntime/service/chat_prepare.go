package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/shared/titlegen"
	"github.com/zgiai/zgi/api/internal/modules/skills"
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
	attachments, err := s.resolveChatAttachmentReferences(ctx, scope, req.FileIDs)
	if err != nil {
		return nil, err
	}
	parts.Attachments = attachments
	if err := s.applyModelCapabilities(ctx, scope, parts); err != nil {
		return nil, err
	}
	if err := s.applySkillConfig(ctx, scope, caller, &config, parts); err != nil {
		return nil, err
	}
	conversation, err := s.resolveChatConversation(ctx, scope, caller, req, parts.Query)
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
	conversation.RuntimeStatus = runtimemodel.ConversationRuntimeStatusStreaming
	conversation.ActiveMessageID = &message.ID
	s.appendStreamEventBestEffort(ctx, message.ID, conversation.ID, streamEventMessageStart, messageStartPayload(conversation, message, false))

	return &PreparedChat{
		Conversation: conversation,
		Message:      message,
		LLMRequest:   llmRequest,
		Scope:        scope,
		Caller:       caller,
		RunConfig:    config,
		ParentID:     parentID,
		parts:        parts,
	}, nil
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
		conversation, err = s.repos.Conversation.GetByCallerScoped(ctx, message.ConversationID, scope.OrganizationID, scope.AccountID, normalizeCallerType(caller.Type), normalizeCallerID(caller.ID))
		if err != nil {
			return nil, mapRepoError(err)
		}
	} else {
		conversation, err = s.getConversation(ctx, scope, message.ConversationID)
		if err != nil {
			return nil, err
		}
	}
	req = applyRunConfigToRegenerateRequest(config, req)
	parts, err := normalizeRegenerateRequest(req, message)
	if err != nil {
		return nil, err
	}
	applyRunConfigToParts(config, parts)
	parts.Attachments = attachmentBundleFromMessageMetadata(message.Metadata)
	if err := s.applyModelCapabilities(ctx, scope, parts); err != nil {
		return nil, err
	}
	if err := s.applySkillConfig(ctx, scope, caller, &config, parts); err != nil {
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
		LLMRequest:   newLLMChatRequest(parts, contextResult.Messages),
		ReplaceRoot:  true,
		Scope:        scope,
		Caller:       caller,
		RunConfig:    config,
		parts:        parts,
	}, nil
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

func (s *service) getConversation(ctx context.Context, scope Scope, id uuid.UUID) (*runtimemodel.Conversation, error) {
	conversation, err := s.repos.Conversation.GetScoped(ctx, id, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	return conversation, nil
}

func (s *service) resolveChatConversation(ctx context.Context, scope Scope, caller Caller, req runtimedto.ChatRequest, query string) (*runtimemodel.Conversation, error) {
	if strings.TrimSpace(req.ConversationID) == "" {
		return s.createConversationForChat(ctx, scope, caller, query)
	}
	conversationID, err := uuid.Parse(strings.TrimSpace(req.ConversationID))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid conversation_id", ErrInvalidInput)
	}
	conversation, err := s.repos.Conversation.GetByCallerScoped(ctx, conversationID, scope.OrganizationID, scope.AccountID, normalizeCallerType(caller.Type), normalizeCallerID(caller.ID))
	if err != nil {
		return nil, mapRepoError(err)
	}
	return conversation, nil
}

func (s *service) createConversationForChat(ctx context.Context, scope Scope, caller Caller, query string) (*runtimemodel.Conversation, error) {
	fallbackTitle := generateTitle(query)
	conversation, err := s.CreateConversationForCaller(ctx, scope, caller, fallbackTitle)
	if err != nil {
		return nil, err
	}
	if s.titleGen == nil {
		return conversation, nil
	}
	s.generateConversationTitleAsync(ctx, scope, conversation, query, fallbackTitle)
	return conversation, nil
}

func (s *service) generateConversationTitleAsync(ctx context.Context, scope Scope, conversation *runtimemodel.Conversation, query, fallbackTitle string) {
	if conversation == nil || s.titleGen == nil {
		return
	}
	conversationID := conversation.ID
	workspaceID := conversation.WorkspaceID
	go func() {
		titleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), titleGenerationTimeout)
		defer cancel()

		result, err := s.titleGen.Generate(titleCtx, titlegen.GenerateRequest{
			OrganizationID: scope.OrganizationID,
			AccountID:      scope.AccountID,
			WorkspaceID:    workspaceID,
			AppID:          conversationID.String(),
			AppType:        runtimemodel.MessageBillingReasonSourceAIChat,
			SessionID:      conversationID.String(),
			ConversationID: conversationID.String(),
			Messages:       []titlegen.Message{{Role: "user", Content: query}},
			FallbackTitle:  fallbackTitle,
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
		rendered, err := renderAIChatSystemPrompt()
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
	}
	parts.SkillIDs, parts.ToolSkillIDs = filterSkillsForModel(enabled, catalog, parts)
	if parts.UseMemory {
		memoryEnabled, err := s.isUserMemoryEnabled(ctx, scope.AccountID)
		if err != nil {
			return err
		}
		if memoryEnabled {
			appendUserMemorySkill(ctx, parts, catalog)
		}
	}
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

func renderAIChatSystemPrompt() (string, error) {
	tmpl, err := prompt.GetTemplate(prompt.AIChatSystem)
	if err != nil {
		return "", err
	}
	return tmpl.Render(nil)
}

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
	if !runConfigAllowsUserMemory(config) {
		parts.UseMemory = false
	}
}

func runConfigAllowsUserMemory(config RunConfig) bool {
	return config.UseMemory && !strings.EqualFold(strings.TrimSpace(config.BillingAppType), runtimemodel.ConversationCallerAgent)
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
		Query:       query,
		ModelName:   modelName,
		Provider:    provider,
		ProviderPtr: providerPtr,
		Parameters:  params,
		UseMemory:   req.UseMemory,
	}, nil
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

	return &chatRequestParts{
		Query:       query,
		ModelName:   modelName,
		Provider:    provider,
		ProviderPtr: providerPtr,
		Parameters:  params,
		UseMemory:   useMemory,
	}, nil
}

func replacementRootMessage(source *runtimemodel.Message, parts *chatRequestParts) *runtimemodel.Message {
	message := newStreamingMessage(source.ConversationID, nil, parts)
	message.ID = source.ID
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
	return &runtimemodel.Message{
		ConversationID:      conversationID,
		ParentID:            parentID,
		Query:               parts.Query,
		Status:              runtimemodel.MessageStatusStreaming,
		ModelProvider:       parts.ProviderPtr,
		ModelName:           parts.ModelName,
		BillingReasonSource: &billingReasonSource,
		ModelParameters:     parts.Parameters,
		Metadata:            streamingMessageMetadata(parts),
	}
}

func streamingMessageMetadata(parts *chatRequestParts) map[string]interface{} {
	version := systemPromptVersion
	if strings.TrimSpace(parts.SystemPromptVersion) != "" {
		version = strings.TrimSpace(parts.SystemPromptVersion)
	}
	metadata := map[string]interface{}{
		"system_prompt_version": version,
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

func (s *service) appendAgentMemoryContext(ctx context.Context, scope Scope, parts *chatRequestParts, systemPrompt string) (string, map[string]interface{}, error) {
	if parts == nil || !parts.AgentMemoryEnabled || len(enabledAgentMemorySlots(parts.AgentMemorySlots)) == 0 {
		return systemPrompt, nil, nil
	}
	slots := enabledAgentMemorySlots(parts.AgentMemorySlots)
	metadata := map[string]interface{}{
		"agent_memory": map[string]interface{}{
			"enabled":        true,
			"available":      false,
			"injected":       false,
			"context_status": "skipped_scope",
		},
	}
	state := &AgentMemoryRuntimeState{
		Enabled:       true,
		UserScope:     strings.TrimSpace(parts.AgentMemoryUserScope),
		EnabledSlots:  slots,
		ContextStatus: "skipped_scope",
	}
	parts.AgentMemoryRuntimeState = state
	if s.agentMemoryService == nil || scope.WorkspaceID == nil || *scope.WorkspaceID == uuid.Nil {
		return systemPrompt, metadata, nil
	}
	agentID, err := uuid.Parse(strings.TrimSpace(parts.AgentMemoryAgentID))
	if err != nil || agentID == uuid.Nil {
		return systemPrompt, metadata, nil
	}
	state.AgentID = agentID
	values, err := s.agentMemoryService.ReadUserMemory(ctx, *scope.WorkspaceID, agentID, agentMemoryRuntimeSlots(slots), parts.AgentMemoryUserScope, scope.AccountID)
	if err != nil {
		state.ContextStatus = "error"
		state.ContextError = err.Error()
		metadata["agent_memory"] = map[string]interface{}{
			"enabled":        true,
			"available":      false,
			"injected":       false,
			"context_status": "error",
			"context_error":  err.Error(),
		}
		return systemPrompt, metadata, nil
	}
	state.SavedValues = append([]agentmemory.SlotValueResponse(nil), values...)
	state.ContextStatus = "success"
	rendered, injectedCount := renderAgentMemoryContext(values, agentMemoryContextBudgetChars)
	metadata["agent_memory"] = map[string]interface{}{
		"enabled":        true,
		"available":      injectedCount > 0,
		"injected":       strings.TrimSpace(rendered) != "",
		"value_count":    injectedCount,
		"context_status": "success",
	}
	if strings.TrimSpace(rendered) == "" {
		return systemPrompt, metadata, nil
	}
	return strings.TrimSpace(systemPrompt) + "\n\n" + rendered, metadata, nil
}

func appendAgentMemoryPolicy(systemPrompt string, parts *chatRequestParts) string {
	rendered := renderAgentMemoryPolicy(parts)
	if rendered == "" {
		return systemPrompt
	}
	base := strings.TrimSpace(systemPrompt)
	if base == "" {
		return rendered
	}
	return base + "\n\n" + rendered
}

func renderAgentMemoryPolicy(parts *chatRequestParts) string {
	if parts == nil || !parts.AgentMemoryEnabled {
		return ""
	}
	slots := enabledAgentMemorySlots(parts.AgentMemorySlots)
	if len(slots) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Agent memory is enabled for this agent.\n")
	b.WriteString("Available memory keys configured by the organizer:\n")
	for _, slot := range slots {
		b.WriteString("- ")
		b.WriteString(slot.Key)
		if description := strings.TrimSpace(slot.Description); description != "" {
			b.WriteString(": ")
			b.WriteString(description)
		}
		if slot.MaxChars > 0 {
			b.WriteString(" (max ")
			b.WriteString(strconv.Itoa(slot.MaxChars))
			b.WriteString(" chars)")
		}
		b.WriteString("\n")
	}
	b.WriteString("\nResponse style rules:\n")
	b.WriteString("- Do not describe internal memory operations to the user. Do not say you loaded memory, read memory, or called a tool.\n")
	b.WriteString("- Use saved memory naturally in the answer. Do not present a separate section saying memories were loaded.\n")
	b.WriteString("- Confirm a memory change only when the current message context contains an internal Agent memory success note for this turn.\n")
	b.WriteString("\nMemory management rules:\n")
	b.WriteString("- Saved Agent memory has already been injected into this system context when available. Use those saved values proactively without waiting for the user to remind you.\n")
	b.WriteString("- Agent memory writes and clears are handled by an internal memory planner before the final answer. Do not invent, simulate, or mention memory tools in the final answer.\n")
	b.WriteString("- If the user provides stable profile facts, preferences, standing instructions, or durable project context, answer normally and let the internal memory planner decide whether to persist it.\n")
	b.WriteString("- Choose keys by semantic fit: profile is only the user's own identity; preferences are response style, language, examples, tone, and formatting; standing_instructions are durable procedures, collaboration rules, interaction rules, user-addressing rules, and agent persona/roleplay instructions; project_context is ongoing project background.\n")
	b.WriteString("- Do not store agent identity, assistant persona, roleplay style, or what the user calls you in profile. Store those in standing_instructions when they are durable interaction rules, or preferences when they are only tone/style preferences.\n")
	b.WriteString("- Do not copy profile facts such as the user's real name, preferred name, job, or team into standing_instructions. If standing_instructions contains an addressing rule, keep it as the rule itself, not as a duplicate profile fact.\n")
	b.WriteString("- When the user changes their name, preferred address, job, or role, update profile only. Do not rewrite standing_instructions unless the user explicitly changes the collaboration rule, assistant persona, or addressing rule.\n")
	b.WriteString("- Do not infer project_context from a profile or job-role change. Update project_context only when the user describes an ongoing project, goal, workstream, or asks to change project background.\n")
	b.WriteString("- Do not save transient small talk, one-off events, secrets, credentials, payment data, government IDs, or other sensitive information. If asked to save sensitive information, politely decline.\n")
	b.WriteString("- Never say you remembered, recorded, updated, saved, cleared, or forgot memory unless an internal Agent memory success note says the change succeeded in this turn.\n")
	b.WriteString("- If there is no internal Agent memory success note, you may say you understand or will follow the user's request in this conversation, but do not claim any memory was saved or changed.\n")
	b.WriteString("- Do not say that you will remember something later. Either update memory successfully in this turn, or answer without claiming it was saved.\n")
	return b.String()
}

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func renderAgentMemoryContext(values []agentmemory.SlotValueResponse, budget int) (string, int) {
	if budget <= 0 || len(values) == 0 {
		return "", 0
	}
	var b strings.Builder
	b.WriteString("Saved Agent memory for the current user:\n")
	b.WriteString("Use these saved memories proactively when answering. If the user's latest message conflicts with saved memory, prefer the latest message and update Agent memory when appropriate.\n")
	b.WriteString("Saved standing_instructions are binding interaction rules for this user. Follow them in every reply, including greetings, casual chat, and short turns, unless the latest user message explicitly changes or overrides them.\n")
	b.WriteString("Important: standing_instructions have higher priority than ordinary small talk. Even short turns must follow saved identity, addressing, tone, and interaction rules.\n")
	count := 0
	for _, value := range values {
		content := strings.TrimSpace(value.Content)
		key := strings.TrimSpace(value.Key)
		if content == "" || key == "" {
			continue
		}
		entryLabel := agentMemoryContextEntryLabel(key)
		entry := "- " + entryLabel + ":\n" + indentAgentMemoryContent(content) + "\n"
		if b.Len()+len(entry) > budget {
			if count == 0 {
				prefix := "- " + entryLabel + ":\n"
				remaining := budget - b.Len() - len(prefix)
				if remaining > 0 {
					b.WriteString(prefix)
					b.WriteString(indentAgentMemoryContent(truncateString(content, remaining)))
					count++
				}
			}
			break
		}
		b.WriteString(entry)
		count++
	}
	if count == 0 {
		return "", 0
	}
	return strings.TrimSpace(b.String()), count
}

func agentMemoryContextEntryLabel(key string) string {
	if strings.EqualFold(strings.TrimSpace(key), "standing_instructions") {
		return "standing_instructions (binding interaction rules; follow every turn)"
	}
	return key
}
func indentAgentMemoryContent(content string) string {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	for i, line := range lines {
		lines[i] = "  " + strings.TrimSpace(line)
	}
	return strings.Join(lines, "\n")
}

func truncateString(value string, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxChars {
		return value
	}
	if maxChars <= 3 {
		return string(runes[:maxChars])
	}
	return string(runes[:maxChars-3]) + "..."
}
func agentMemoryRuntimeSlots(input []AgentMemorySlotConfig) []agentmemory.RuntimeSlot {
	slots := enabledAgentMemorySlots(input)
	out := make([]agentmemory.RuntimeSlot, 0, len(slots))
	for _, slot := range slots {
		out = append(out, agentmemory.RuntimeSlot{
			Key:         slot.Key,
			Description: slot.Description,
			MaxChars:    slot.MaxChars,
			Enabled:     slot.Enabled,
			SortOrder:   slot.SortOrder,
		})
	}
	return out
}

func enabledAgentMemorySlots(input []AgentMemorySlotConfig) []AgentMemorySlotConfig {
	normalized := normalizeAgentMemorySlots(input)
	if len(normalized) == 0 {
		return nil
	}
	out := make([]AgentMemorySlotConfig, 0, len(normalized))
	for _, slot := range normalized {
		if slot.Enabled {
			out = append(out, slot)
		}
	}
	return out
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

func appendUserMemorySkill(ctx context.Context, parts *chatRequestParts, catalog []skills.SkillDiscoveryMetadata) {
	_ = ctx
	if parts == nil {
		return
	}
	id := userMemorySkillID()
	for _, item := range catalog {
		if strings.EqualFold(strings.TrimSpace(item.ID), id) && item.Status != skills.SkillStatusInvalid {
			parts.SkillIDs = appendUniqueSkillID(parts.SkillIDs, id)
			parts.ToolSkillIDs = appendUniqueSkillID(parts.ToolSkillIDs, id)
			return
		}
	}
}

func appendUniqueSkillID(values []string, id string) []string {
	normalized := strings.ToLower(strings.TrimSpace(id))
	if normalized == "" {
		return values
	}
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), normalized) {
			return values
		}
	}
	return append(values, normalized)
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
