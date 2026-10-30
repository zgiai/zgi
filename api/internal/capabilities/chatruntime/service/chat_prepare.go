package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/contextmgr"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/prompt"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

type temporaryFileClaimer interface {
	ClaimTemporaryFilesInTx(ctx context.Context, tx *gorm.DB, fileIDs []string, ownerID, usedBy string) error
}

func (s *service) PrepareChat(ctx context.Context, scope Scope, req runtimedto.ChatRequest) (*PreparedChat, error) {
	return s.PrepareConfiguredChat(ctx, scope, Caller{Type: runtimemodel.ConversationCallerAIChat}, RunConfig{
		BillingAppType: runtimemodel.MessageBillingReasonSourceAIChat,
	}, req)
}

func (s *service) PrepareConfiguredChat(ctx context.Context, scope Scope, caller Caller, config RunConfig, req runtimedto.ChatRequest) (*PreparedChat, error) {
	config, parts, conversation, parentID, err := s.prepareConfiguredChatDefinition(ctx, scope, caller, config, req)
	if err != nil {
		return nil, err
	}

	message := newStreamingMessage(conversation.ID, parentID, parts)
	temporaryFileIDs := temporaryAttachmentIDs(parts.Attachments)
	if err := s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepos := repository.NewRepositories(tx)
		if err := txRepos.Message.Create(ctx, message); err != nil {
			return err
		}
		if len(temporaryFileIDs) > 0 {
			if claimer, ok := s.fileService.(temporaryFileClaimer); ok {
				ownerID := scope.AccountID.String()
				if scope.ExternalUserID != nil && *scope.ExternalUserID != uuid.Nil {
					ownerID = scope.ExternalUserID.String()
				}
				if err := claimer.ClaimTemporaryFilesInTx(ctx, tx, temporaryFileIDs, ownerID, message.ID.String()); err != nil {
					return err
				}
			}
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
		Conversation:    conversation,
		Message:         message,
		Scope:           scope,
		Caller:          caller,
		RunConfig:       config,
		ParentID:        parentID,
		parts:           parts,
		taskTokenBudget: newTaskTokenBudget(req.MaxTaskTokens),
	}
	s.refreshInitialPageContext(ctx, prepared)
	return prepared, nil
}

func (s *service) prepareConfiguredChatDefinition(ctx context.Context, scope Scope, caller Caller, config RunConfig, req runtimedto.ChatRequest) (RunConfig, *chatRequestParts, *runtimemodel.Conversation, *uuid.UUID, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return RunConfig{}, nil, nil, nil, err
	}
	if req.MaxTaskTokens < 0 || req.MaxTaskTokens > maxTaskTokenBudget {
		return RunConfig{}, nil, nil, nil, fmt.Errorf("%w: max_task_tokens must be between 0 and %d", ErrInvalidInput, maxTaskTokenBudget)
	}
	if req.MaxTaskTokens > 0 && !taskTokenBudgetCallerAllowed(caller, req) {
		return RunConfig{}, nil, nil, nil, fmt.Errorf("%w: max_task_tokens requires an external API or scheduled digital employee", ErrInvalidInput)
	}
	initialConnections, err := initialChatConnections(caller, req)
	if err != nil {
		return RunConfig{}, nil, nil, nil, err
	}
	req = applyRunConfigToChatRequest(config, req)
	parts, err := normalizeChatRequest(req)
	if err != nil {
		return RunConfig{}, nil, nil, nil, err
	}
	applyRunConfigToParts(config, parts)
	applyCallerRuntimeSurfacePolicy(caller, parts)
	if len(req.FileIDs) > 0 && config.FileUploadEnabled != nil && !*config.FileUploadEnabled {
		return RunConfig{}, nil, nil, nil, fmt.Errorf("%w: file upload is disabled", ErrPermissionDenied)
	}
	attachments, err := s.resolveChatAttachmentReferences(ctx, scope, req.FileIDs, config.FileUploadPolicy)
	if err != nil {
		return RunConfig{}, nil, nil, nil, err
	}
	parts.Attachments = attachments
	if err := s.applyExistingConversationSurfaceForChat(ctx, scope, caller, req, parts); err != nil {
		return RunConfig{}, nil, nil, nil, err
	}
	if err := s.applyModelCapabilities(ctx, scope, caller, parts); err != nil {
		return RunConfig{}, nil, nil, nil, err
	}
	applyProtocolToolsPolicy(caller, parts)
	applyCallerMemoryPolicy(caller, parts)
	if hasInitialChatConnections(req) {
		conversation, err := s.createChatWithInitialConnections(ctx, scope, caller, parts, initialConnections, req.InitialConnectorDefaultsRevision)
		if err != nil {
			return RunConfig{}, nil, nil, nil, err
		}
		// Skill discovery happens below: first-turn choices must already be
		// committed and addressed by the real conversation ID at that boundary.
		req.ConversationID = conversation.ID.String()
		parts.ConversationID = req.ConversationID
		parts.RequiredInitialConnectorBindings = initialConnections
	}
	if err := s.applySkillConfig(ctx, scope, caller, &config, parts); err != nil {
		return RunConfig{}, nil, nil, nil, err
	}
	applyAgentMemoryToolsPolicy(parts)
	if err := finalizeExecutionMode(caller, parts); err != nil {
		return RunConfig{}, nil, nil, nil, err
	}
	logExecutionRoute(ctx, caller, parts)
	conversation, err := s.resolveChatConversation(ctx, scope, caller, req, parts)
	if err != nil {
		return RunConfig{}, nil, nil, nil, err
	}
	if err := s.ensureConversationAllowsNewTurn(ctx, scope, conversation, config.runtimeInputFollowUp); err != nil {
		return RunConfig{}, nil, nil, nil, err
	}
	parentID, err := s.resolveParentMessage(ctx, scope, conversation, strings.TrimSpace(req.ParentID))
	if err != nil {
		return RunConfig{}, nil, nil, nil, err
	}
	return config, parts, conversation, parentID, nil
}

// Scheduled employees without Linux execution keep the console conversation
// source. The server-owned exact run correlation, rather than that source
// alone, admits their bounded model request.
func taskTokenBudgetCallerAllowed(caller Caller, request runtimedto.ChatRequest) bool {
	return normalizeConversationSource(caller.Source) == runtimemodel.ConversationSourceExternalAPI ||
		(caller.Type == runtimemodel.ConversationCallerDigitalHuman && caller.RunID != nil && *caller.RunID != uuid.Nil && request.OperationContext["digital_human_run_id"] == caller.RunID.String())
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
	var err error
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
	if err := ensureConversationWorkspaceScope(scope, conversation); err != nil {
		return nil, err
	}
	if err := s.ensureConversationAllowsNewTurn(ctx, scope, conversation, config.runtimeInputFollowUp); err != nil {
		return nil, err
	}
	req = applyRunConfigToRegenerateRequest(config, req)
	parts, err := normalizeRegenerateRequest(req, message)
	if err != nil {
		return nil, err
	}
	applyRunConfigToParts(config, parts)
	applyCallerRuntimeSurfacePolicy(caller, parts)
	applyPersistedConversationSurface(conversation, parts)
	attachments, err := s.resolveChatAttachmentReferences(ctx, scope, attachmentFileIDsFromMessageMetadata(message.Metadata), config.FileUploadPolicy)
	if err != nil {
		return nil, err
	}
	parts.Attachments = attachments
	if err := s.applyRootRegenerationModelCapabilities(ctx, scope, caller, message, parts); err != nil {
		return nil, err
	}
	applyProtocolToolsPolicy(caller, parts)
	applyCallerMemoryPolicy(caller, parts)
	if err := s.applySkillConfig(ctx, scope, caller, &config, parts); err != nil {
		return nil, err
	}
	applyAgentMemoryToolsPolicy(parts)
	if err := finalizeExecutionMode(caller, parts); err != nil {
		return nil, err
	}
	logExecutionRoute(ctx, caller, parts)
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

func conversationWaiting(cause error, message *runtimemodel.Message) error {
	waiting := &ConversationWaitingError{Cause: cause, MessageID: message.ID, Status: string(message.Status)}
	if message.Status == runtimemodel.MessageStatusWaitingQuestion {
		request := governanceMapFromAny(message.Metadata["user_input_request"])
		waiting.RequestID = strings.TrimSpace(stringFromAny(request["request_id"]))
	}
	return waiting
}

func (s *service) ensureConversationAllowsNewTurn(ctx context.Context, scope Scope, conversation *runtimemodel.Conversation, allowRuntimeInputFollowUp bool) error {
	if conversation != nil &&
		conversation.RuntimeStatus == runtimemodel.ConversationRuntimeStatusStreaming &&
		conversation.ActiveMessageID != nil {
		activeMessage, err := s.repos.Message.GetScoped(ctx, *conversation.ActiveMessageID, scope.OrganizationID, scope.AccountID)
		if err != nil {
			return mapRepoError(err)
		}
		switch activeMessage.Status {
		case runtimemodel.MessageStatusCompleted, runtimemodel.MessageStatusStopped, runtimemodel.MessageStatusError:
			if err := s.repos.Conversation.ClearActiveMessage(ctx, conversation.ID, activeMessage.ID); err != nil {
				return err
			}
			conversation.RuntimeStatus = runtimemodel.ConversationRuntimeStatusIdle
			conversation.ActiveMessageID = nil
		}
	}
	if conversation == nil || conversation.CurrentLeafMessageID == nil {
		return nil
	}
	if conversation.RuntimeStatus != runtimemodel.ConversationRuntimeStatusIdle {
		return nil
	}
	if conversation.CurrentLeafMessageID != nil {
		leafMessage, err := s.repos.Message.GetScoped(ctx, *conversation.CurrentLeafMessageID, scope.OrganizationID, scope.AccountID)
		if err != nil {
			return mapRepoError(err)
		}
		if leafMessage.Status == runtimemodel.MessageStatusWaitingApproval {
			return conversationWaiting(ErrConversationWaitingApproval, leafMessage)
		}
		if leafMessage.Status == runtimemodel.MessageStatusWaitingQuestion {
			return conversationWaiting(ErrConversationWaitingQuestion, leafMessage)
		}
		if leafMessage.Status == runtimemodel.MessageStatusWaitingClientAction {
			return conversationWaiting(ErrConversationWaitingAction, leafMessage)
		}
		if leafMessage.Status == runtimemodel.MessageStatusWaitingBudget {
			return conversationWaiting(ErrConversationWaitingBudget, leafMessage)
		}
	}
	if !allowRuntimeInputFollowUp {
		pending, err := s.repos.RuntimeInput.HasPendingFollowUp(ctx, conversation.ID)
		if err != nil {
			return err
		}
		if pending {
			return ErrConversationRunning
		}
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
	if err := ensureConversationWorkspaceScope(scope, conversation); err != nil {
		return nil, err
	}
	applyPersistedConversationSurface(conversation, parts)
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
	if s.titleGen == nil || !conversationTitleGenerationEnabled(caller) {
		return conversation, nil
	}
	s.markConversationTitleGenerationPending(ctx, scope, conversation)
	s.generateConversationTitleAsync(ctx, scope, caller, conversation, parts, initialTitle)
	return conversation, nil
}

func (s *service) generateConversationTitleAsync(ctx context.Context, scope Scope, caller Caller, conversation *runtimemodel.Conversation, parts *chatRequestParts, initialTitle string) {
	if conversation == nil || s.titleGen == nil {
		return
	}
	query := ""
	preferredProvider := ""
	preferredModel := ""
	routingStrategy := "stability"
	if parts != nil {
		query = parts.Query
		preferredProvider = parts.Provider
		preferredModel = parts.ModelName
		routingStrategy = parts.RoutingStrategy
	}
	s.enqueueConversationTitleGeneration(ctx, scope, caller, conversation, conversationTitleGenerationInput{
		Messages:          conversationTitleMessagesFromQuery(query),
		FallbackTitle:     initialTitle,
		PreferredProvider: preferredProvider,
		PreferredModel:    preferredModel,
		RoutingStrategy:   routingStrategy,
	})
}

func conversationTitleAppContext(caller Caller, conversationID uuid.UUID) (string, string) {
	if isAutonomousSubjectCaller(caller.Type) {
		if caller.ID != nil && *caller.ID != uuid.Nil {
			return caller.ID.String(), normalizeCallerType(caller.Type)
		}
		return conversationID.String(), normalizeCallerType(caller.Type)
	}
	return conversationID.String(), runtimemodel.MessageBillingReasonSourceAIChat
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

func (s *service) buildUpstreamMessages(ctx context.Context, scope Scope, parentID *uuid.UUID, parts *chatRequestParts, conversationIDs ...uuid.UUID) (*contextBudgetResult, error) {
	if scope.RuntimeFileAuthorizer != nil && scope.RuntimeWorkspaceID != nil && scope.RuntimeSubjectID != nil {
		parts.AttachmentReadAllowed = true
	}
	s.applyCurrentTimeContext(ctx, scope, parts, time.Now())
	systemPrompt := strings.TrimSpace(parts.SystemPrompt)
	if systemPrompt == "" {
		rendered, err := renderAIChatSystemPrompt(parts.Surface)
		if err != nil {
			return nil, err
		}
		systemPrompt = rendered
	}
	var err error
	var resolvedSpec ModelSpec
	var resolvedSpecOK bool
	if s.modelSpecResolver != nil {
		resolvedSpec, resolvedSpecOK, err = s.modelSpecResolver.Resolve(ctx, scope.OrganizationID, parts.Provider, parts.ModelName)
		if err != nil {
			logger.WarnContext(ctx, "failed to resolve model context window; using fallback context window", "error", err)
			resolvedSpec = ModelSpec{}
			resolvedSpecOK = false
		}
		if resolvedSpecOK {
			parts.ModelSupportsVision = resolvedSpec.SupportsVision()
		}
	}
	if !resolvedSpecOK || resolvedSpec.ContextWindow <= 0 {
		resolvedSpec.ContextWindow = contextmgr.DefaultModelContextWindowTokens
		resolvedSpecOK = true
	}
	inputTokenLimit := 0
	if resolvedSpecOK {
		inputTokenLimit = resolvedSpec.MaxInputTokens
		if inputTokenLimit <= 0 {
			inputTokenLimit = resolvedSpec.ContextWindow
		}
	}
	systemPrompt, agentMemoryMetadata, err := s.appendAgentMemoryContext(ctx, scope, parts, systemPrompt, inputTokenLimit)
	if err != nil {
		return nil, err
	}
	if unavailable := agentMemoryUnavailableSystemMessage(parts); unavailable != nil {
		systemPrompt = strings.TrimSpace(systemPrompt) + "\n\n" + strings.TrimSpace(stringFromAny(unavailable.Content))
	}
	if resolvedSpecOK && resolvedSpec.ContextWindow > 0 {
		var contextState *loadedContextState
		if len(conversationIDs) > 0 && conversationIDs[0] != uuid.Nil {
			contextState, err = s.loadContextState(ctx, scope, conversationIDs[0], parentID)
		} else if parentID != nil && *parentID != uuid.Nil {
			parent, parentErr := s.repos.Message.GetScoped(ctx, *parentID, scope.OrganizationID, scope.AccountID)
			if parentErr != nil {
				return nil, parentErr
			}
			contextState, err = s.loadContextState(ctx, scope, parent.ConversationID, parentID)
		} else {
			contextState = &loadedContextState{RawMessages: []*runtimemodel.Message{}}
		}
		if err != nil {
			return nil, err
		}
		branch := contextState.RawMessages
		parts.AgentMemoryRecentUserMessages = recentUserQueries(branch, agentMemoryEvidenceWindow)
		parts.RecentSkillIDs = recentlyUsedSkillIDs(branch, recentSkillTurns)
		registerAttachmentReadFiles(parts, branch)
		applyRecentAssetCandidatesFromBranch(parts, branch)
		applyRecentGeneratedArtifactsFromBranch(ctx, parts, branch)
		applyRecentOperationPlansFromBranch(parts, branch)
		result, err := s.buildTokenBudgetMessages(ctx, resolvedSpec, parts, systemPrompt, branch)
		if err != nil {
			return nil, err
		}
		result.RawMessages = contextState.RawMessages
		result.Metadata = mergeUserMemoryMetadata(result.Metadata, agentMemoryMetadata)
		return result, nil
	}
	currentContent, contextMetadata := s.buildFallbackCurrentUserContent(parts)
	registerAttachmentReadFiles(parts, nil)
	messages := []adapter.Message{{Role: "system", Content: systemPrompt}}
	if memoryContext := agentMemoryContextMessage(parts); memoryContext != nil {
		messages = append(messages, *memoryContext)
	}
	contextMetadata = mergeUserMemoryMetadata(contextMetadata, agentMemoryMetadata)
	if parentID != nil && *parentID != uuid.Nil {
		parent, err := s.repos.Message.GetScoped(ctx, *parentID, scope.OrganizationID, scope.AccountID)
		if err != nil {
			return nil, err
		}
		state, err := s.loadContextState(ctx, scope, parent.ConversationID, parentID)
		if err != nil {
			return nil, err
		}
		branch := state.RawMessages
		parts.AgentMemoryRecentUserMessages = recentUserQueries(branch, agentMemoryEvidenceWindow)
		registerAttachmentReadFiles(parts, branch)
		if !shouldIsolateHistoryForCurrentTurn(parts) {
			groups, err := s.historyMessageGroupsForCurrentRequest(ctx, branch, parts)
			if err != nil {
				return nil, err
			}
			for _, group := range groups {
				messages = append(messages, group...)
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
		if unbound := databaseBindingsRemovedMessage(parts, branch); unbound != nil {
			messages = append(messages, *unbound)
		}
		if turnBoundaryContext := currentTurnBoundaryMessage(parts); turnBoundaryContext != nil {
			messages = append(messages, *turnBoundaryContext)
		}
		applyRecentAssetCandidatesFromBranch(parts, branch)
		applyRecentGeneratedArtifactsFromBranch(ctx, parts, branch)
	}
	if turnContext := turnContextMessage(parts); turnContext != nil {
		messages = append(messages, *turnContext)
	}
	messages = append(messages, adapter.Message{Role: "user", Content: currentContent})
	return &contextBudgetResult{Messages: messages, Metadata: contextMetadata}, nil
}

func (s *service) applyModelCapabilities(ctx context.Context, scope Scope, caller Caller, parts *chatRequestParts) error {
	if parts == nil {
		return nil
	}
	if s.modelSpecResolver == nil {
		return applyModelCapabilityFallback(ctx, parts, "model capability resolver is unavailable")
	}
	spec, ok, err := s.modelSpecResolver.Resolve(ctx, scope.OrganizationID, parts.Provider, parts.ModelName)
	if err != nil {
		return applyModelCapabilityFallback(ctx, parts, err.Error())
	}
	if !ok {
		return applyModelCapabilityFallback(ctx, parts, fmt.Sprintf("model %s/%s was not found", parts.Provider, parts.ModelName))
	}
	parts.ModelSupportsVision = spec.SupportsVision()
	parts.ModelSupportsAgent = spec.SupportsAgent()
	parts.FunctionCallingKnown = true
	parts.ModelSupportsFunctionCalling = spec.SupportsFunctionCalling()
	parts.FunctionCallingAssumed = false
	parts.ModelCapabilityStatus = "resolved"
	parts.ModelCapabilityError = ""
	return nil
}

func applyModelCapabilityFallback(ctx context.Context, parts *chatRequestParts, reason string) error {
	if parts == nil {
		return nil
	}
	parts.ModelSupportsVision = false
	parts.ModelSupportsAgent = false
	parts.FunctionCallingKnown = false
	parts.ModelSupportsFunctionCalling = false
	parts.FunctionCallingAssumed = true
	parts.ModelCapabilityStatus = "fallback"
	parts.ModelCapabilityError = strings.TrimSpace(reason)
	logger.WarnContext(ctx, "AI Chat model capabilities unavailable; using context-window fallback", "reason", parts.ModelCapabilityError)
	return nil
}

func (s *service) applyRootRegenerationModelCapabilities(
	ctx context.Context,
	scope Scope,
	caller Caller,
	message *runtimemodel.Message,
	parts *chatRequestParts,
) error {
	if regenerationKeepsPersistedModel(message, parts) && executionModeCanResumeOnRegeneration(message) {
		restoreExecutionModeFromMetadata(parts, message.Metadata)
	}
	return s.applyModelCapabilities(ctx, scope, caller, parts)
}

func executionModeCanResumeOnRegeneration(message *runtimemodel.Message) bool {
	if message == nil {
		return false
	}
	switch normalizeExecutionMode(stringMetadataValue(message.Metadata["execution_mode"])) {
	case executionModeNativeAgentLoop, executionModeNativeToolLoop, executionModeDirectChat:
		return true
	default:
		// The legacy Agent and tool-chat protocols remain available only for
		// continuations that are already waiting on an external action. A manual
		// regeneration starts a fresh run and must use the current execution route.
		return false
	}
}

func regenerationKeepsPersistedModel(message *runtimemodel.Message, parts *chatRequestParts) bool {
	if message == nil || parts == nil {
		return false
	}
	persistedProvider := ""
	if message.ModelProvider != nil {
		persistedProvider = strings.TrimSpace(*message.ModelProvider)
	}
	return strings.TrimSpace(message.ModelName) == strings.TrimSpace(parts.ModelName) &&
		persistedProvider == strings.TrimSpace(parts.Provider)
}

func (s *service) applyExistingConversationSurfaceForChat(ctx context.Context, scope Scope, caller Caller, req runtimedto.ChatRequest, parts *chatRequestParts) error {
	conversationIDValue := strings.TrimSpace(req.ConversationID)
	if conversationIDValue == "" {
		return nil
	}
	conversationID, err := uuid.Parse(conversationIDValue)
	if err != nil {
		return fmt.Errorf("%w: invalid conversation_id", ErrInvalidInput)
	}
	conversation, err := s.getConversationByCallerScoped(ctx, scope, caller, conversationID)
	if err != nil {
		return err
	}
	applyPersistedConversationSurface(conversation, parts)
	return nil
}

func executionModeForModel(caller Caller, parts *chatRequestParts) string {
	if parts == nil || !parts.ModelSupportsFunctionCalling {
		return executionModeDirectChat
	}
	if isAutonomousSubjectCaller(caller.Type) {
		return executionModeNativeAgentLoop
	}
	if len(parts.SkillIDs) == 0 && !parts.AgentMemoryToolsEnabled && !parts.ConnectorToolsEnabled {
		return executionModeDirectChat
	}
	return executionModeNativeToolLoop
}

func finalizeExecutionMode(caller Caller, parts *chatRequestParts) error {
	if parts == nil {
		return nil
	}
	if parts.ExecutionMode == executionModeDirectChat && (parts.AgentMemoryToolsEnabled || parts.ConnectorToolsEnabled) {
		parts.ExecutionMode = ""
	}
	if parts.ExecutionMode == executionModeNativeToolLoop && len(parts.SkillIDs) == 0 && !parts.ProtocolToolsEnabled && !parts.AgentMemoryToolsEnabled && !parts.ConnectorToolsEnabled {
		parts.ExecutionMode = ""
	}
	if executionModeRequiresFunctionCalling(parts.ExecutionMode) && !parts.ModelSupportsFunctionCalling {
		parts.ExecutionMode = executionModeDirectChat
		parts.ExecutionRouteReason = executionRouteFunctionCallingUnavailable
	} else if normalizeExecutionMode(parts.ExecutionMode) != "" {
		parts.ExecutionRouteReason = executionRoutePersistedMode
	} else {
		parts.ExecutionMode = executionModeForModel(caller, parts)
		switch parts.ExecutionMode {
		case executionModeNativeAgentLoop:
			parts.ExecutionRouteReason = executionRouteNativeAgent
		case executionModeNativeToolLoop:
			if len(parts.SkillIDs) == 0 && parts.AgentMemoryToolsEnabled {
				parts.ExecutionRouteReason = executionRouteAgentMemoryAvailable
			} else {
				parts.ExecutionRouteReason = executionRouteNativeSkillsAvailable
			}
		case executionModeDirectChat:
			if !parts.ModelSupportsFunctionCalling {
				parts.ExecutionRouteReason = executionRouteFunctionCallingUnavailable
			} else {
				parts.ExecutionRouteReason = executionRouteNoUsableSkills
			}
		}
	}
	return nil
}

func logExecutionRoute(ctx context.Context, caller Caller, parts *chatRequestParts) {
	if parts == nil {
		return
	}
	logger.InfoContext(ctx, "chat runtime execution route selected",
		"caller_type", normalizeCallerType(caller.Type),
		"surface", normalizeAIChatSurface(parts.Surface),
		"execution_mode", normalizeExecutionMode(parts.ExecutionMode),
		"execution_route_reason", strings.TrimSpace(parts.ExecutionRouteReason),
		"model_use_case", executionModeModelUseCase(parts.ExecutionMode),
		"function_calling_supported", parts.ModelSupportsFunctionCalling,
		"effective_skill_count", len(parts.SkillIDs),
		"connector_tools_enabled", parts.ConnectorToolsEnabled,
		"agent_memory_tools_enabled", parts.AgentMemoryToolsEnabled,
	)
}

func executionModeRequiresFunctionCalling(mode string) bool {
	switch strings.TrimSpace(mode) {
	case executionModeDirectChat:
		return false
	default:
		return true
	}
}

func applyCallerMemoryPolicy(caller Caller, parts *chatRequestParts) {
	applyAIChatMemoryPolicy(caller, parts)
	if parts == nil || !isDigitalHumanCaller(caller.Type) {
		return
	}
	parts.AgentMemoryEnabled = false
	parts.AgentMemoryAutoExtractionEnabled = false
	parts.AgentMemoryToolsEnabled = false
	parts.AgentMemorySlots = nil
	parts.AgentMemoryUserScope = ""
	parts.AgentMemoryConfigScope = ""
	parts.AgentMemoryConfigRevision = ""
	parts.AgentMemoryAgentID = ""
	parts.AgentMemoryRuntimeState = nil
	parts.AgentMemoryContext = ""
}

func applyProtocolToolsPolicy(caller Caller, parts *chatRequestParts) {
	if parts == nil {
		return
	}
	parts.ProtocolToolsEnabled = isAutonomousSubjectCaller(caller.Type) &&
		parts.FunctionCallingKnown && parts.ModelSupportsFunctionCalling && !parts.FunctionCallingAssumed
}

func applyAgentMemoryToolsPolicy(parts *chatRequestParts) {
	if parts == nil {
		return
	}
	parts.AgentMemoryToolsEnabled = parts.AgentMemoryEnabled &&
		globalAgentMemoryInlineToolsEnabled() &&
		parts.FunctionCallingKnown && parts.ModelSupportsFunctionCalling && !parts.FunctionCallingAssumed
}

func (s *service) applySkillConfig(ctx context.Context, scope Scope, caller Caller, config *RunConfig, parts *chatRequestParts) error {
	if parts == nil {
		return nil
	}
	parts.BaseSkillIDs = nil
	parts.BaseToolSkillIDs = nil
	parts.ConnectorSkillIDs = nil
	parts.MarketSkillIDs = nil
	parts.ConnectorToolsEnabled = false
	parts.DatabaseBindingsAbsent = config != nil && isAutonomousSubjectCaller(caller.Type) && !runConfigHasDatabaseBindings(config)
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
	if err := s.applyConnectorSkillConfig(ctx, scope, caller, parts); err != nil {
		return err
	}
	if parts.ExecutionMode == executionModeDirectChat && !parts.ConnectorToolsEnabled {
		parts.SkillMode = skillModeDisabled
		parts.SkillIDs = nil
		parts.ToolSkillIDs = nil
		return nil
	}
	if parts.ExecutionMode == executionModeDirectChat && parts.ConnectorToolsEnabled {
		// A persisted direct-chat route predates the newly attached Connector.
		// Recompute the execution mode from current capabilities instead of
		// suppressing an explicitly authorized native operation.
		parts.ExecutionMode = ""
	}
	catalog, err := s.catalogSkillMetadata(ctx, scope.OrganizationID)
	if err != nil {
		return err
	}
	orgEnabled, err := s.effectiveOrganizationSkillIDs(ctx, scope.OrganizationID, catalog)
	if err != nil {
		return err
	}
	callerType := normalizeCallerType(caller.Type)
	var enabled []string
	if isAutonomousSubjectCaller(callerType) {
		enabled = effectiveBoundSkillIDs(parts.ConfiguredSkillIDs, catalog, orgEnabled, config, callerType)
		if callerType == runtimemodel.ConversationCallerDigitalHuman {
			enabled = filterDigitalHumanSkillIDs(enabled)
		}
	} else {
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
	if parts.ConnectorToolsDisabled {
		enabled = skillIDsWithout(enabled, []string{skills.SkillConnectorRuntime})
	}
	enabled = withInherentTimeSkill(enabled, catalog, orgEnabled, callerType, parts)
	parts.BaseSkillIDs, parts.BaseToolSkillIDs = filterSkillsForModel(enabled, catalog, parts)
	parts.MarketSkillIDs = s.marketSkillIDs(ctx, scope, caller)
	parts.SkillIDs = mergeNormalizedSkillIDs(parts.BaseSkillIDs, parts.ConnectorSkillIDs, parts.MarketSkillIDs)
	parts.ToolSkillIDs = mergeNormalizedSkillIDs(parts.BaseToolSkillIDs, parts.ConnectorSkillIDs, parts.MarketSkillIDs)
	if len(parts.SkillIDs) == 0 {
		parts.SkillMode = skillModeDisabled
		return nil
	}
	parts.SkillMode = skillModeAuto
	return nil
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
		Provider:        parts.Provider,
		RoutingStrategy: parts.RoutingStrategy,
		Model:           parts.ModelName,
		Messages:        messages,
		Stream:          true,
	}
	applyModelParameters(req, parts.Parameters)
	return req
}

func normalizeRoutingStrategy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "speed", "price", "stability":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "stability"
	}
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
