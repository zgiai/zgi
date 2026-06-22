package service

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"gorm.io/gorm"
)

func (s *service) CreateConversation(ctx context.Context, scope Scope, title string) (*runtimemodel.Conversation, error) {
	return s.CreateConversationForCaller(ctx, scope, Caller{Type: runtimemodel.ConversationCallerAIChat}, title)
}

func (s *service) CreateConversationForCaller(ctx context.Context, scope Scope, caller Caller, title string) (*runtimemodel.Conversation, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	workspaceID, err := s.resolveWorkspaceID(ctx, scope)
	if err != nil {
		return nil, err
	}
	title = normalizeTitle(title, defaultConversationTitle)
	source := normalizeConversationSource(caller.Source)
	sourceWebAppID := normalizeCallerID(caller.SourceWebAppID)
	if source == runtimemodel.ConversationSourceWebApp && sourceWebAppID == nil {
		return nil, fmt.Errorf("%w: source_web_app_id is required for webapp conversations", ErrInvalidInput)
	}
	conversation := &runtimemodel.Conversation{
		OrganizationID: scope.OrganizationID,
		WorkspaceID:    workspaceID,
		AccountID:      scope.AccountID,
		CallerType:     normalizeCallerType(caller.Type),
		CallerID:       normalizeCallerID(caller.ID),
		Title:          title,
		Status:         runtimemodel.ConversationStatusNormal,
		Source:         source,
		SourceWebAppID: sourceWebAppID,
	}
	if err := s.repos.Conversation.Create(ctx, conversation); err != nil {
		return nil, err
	}
	return conversation, nil
}

func normalizeCallerType(value string) string {
	switch strings.TrimSpace(value) {
	case runtimemodel.ConversationCallerAgent:
		return runtimemodel.ConversationCallerAgent
	default:
		return runtimemodel.ConversationCallerAIChat
	}
}

func normalizeConversationSource(value string) string {
	switch strings.TrimSpace(value) {
	case runtimemodel.ConversationSourceWebApp:
		return runtimemodel.ConversationSourceWebApp
	case runtimemodel.ConversationSourceMigration:
		return runtimemodel.ConversationSourceMigration
	default:
		return runtimemodel.ConversationSourceConsole
	}
}

func normalizeCallerID(value *uuid.UUID) *uuid.UUID {
	if value == nil || *value == uuid.Nil {
		return nil
	}
	out := *value
	return &out
}

func (s *service) ListSkills(ctx context.Context, scope Scope) ([]skills.SkillDiscoveryMetadata, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	if s.skillRuntime == nil {
		return []skills.SkillDiscoveryMetadata{}, nil
	}
	metadata, err := s.catalogSkillMetadata(ctx, scope.OrganizationID)
	if err != nil {
		return nil, err
	}
	metadata = visibleSkillMetadata(metadata)
	enabled, err := s.effectiveOrganizationSkillIDs(ctx, scope.OrganizationID, metadata)
	if err != nil {
		return nil, err
	}
	markEnabledSkills(metadata, enabled)
	return metadata, nil
}

func (s *service) GetSkill(ctx context.Context, scope Scope, skillID string) (*skills.SkillDiscoveryMetadata, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	skillID = strings.TrimSpace(skillID)
	if skillID == "" {
		return nil, fmt.Errorf("%w: skill id is required", ErrInvalidInput)
	}
	if s.skillRuntime == nil {
		return nil, fmt.Errorf("%w: skill not found", ErrNotFound)
	}
	catalog, err := s.catalogSkillMetadata(ctx, scope.OrganizationID)
	if err != nil {
		return nil, err
	}
	catalog = visibleSkillMetadata(catalog)
	normalized := strings.ToLower(strings.TrimSpace(skillID))
	for idx := range catalog {
		if catalog[idx].ID != normalized {
			continue
		}
		catalog[idx].Enabled = s.isOrganizationSkillEnabled(ctx, scope.OrganizationID, catalog[idx].ID)
		if catalog[idx].Status == skills.SkillStatusInvalid {
			catalog[idx].Enabled = false
		}
		return &catalog[idx], nil
	}
	return nil, fmt.Errorf("%w: skill not found", ErrNotFound)
}

func (s *service) GetSkillConfig(ctx context.Context, scope Scope) (*SkillConfig, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	metadata, err := s.catalogSkillMetadata(ctx, scope.OrganizationID)
	if err != nil {
		return nil, err
	}
	metadata = visibleSkillMetadata(metadata)
	enabled, err := s.effectiveOrganizationSkillIDs(ctx, scope.OrganizationID, metadata)
	if err != nil {
		return nil, err
	}
	return &SkillConfig{EnabledSkillIDs: enabled}, nil
}

func (s *service) UpdateSkillConfig(ctx context.Context, scope Scope, req runtimedto.UpdateSkillConfigRequest) (*SkillConfig, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	metadata, err := s.catalogSkillMetadata(ctx, scope.OrganizationID)
	if err != nil {
		return nil, err
	}
	metadata = visibleSkillMetadata(metadata)
	normalized, err := validateSkillConfigIDs(req.EnabledSkillIDs, metadata)
	if err != nil {
		return nil, err
	}
	configs := organizationSkillConfigRows(scope.OrganizationID, metadata, normalized)
	if err := s.repos.SkillConfig.ReplaceForOrganization(ctx, scope.OrganizationID, configs); err != nil {
		return nil, err
	}
	return &SkillConfig{EnabledSkillIDs: normalized}, nil
}

func (s *service) GetAccountSkillPreference(ctx context.Context, scope Scope, callerType string) (*AccountSkillPreference, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	callerType = normalizeCallerType(callerType)
	if callerType != runtimemodel.ConversationCallerAIChat {
		return nil, fmt.Errorf("%w: unsupported caller type", ErrInvalidInput)
	}
	metadata, err := s.catalogSkillMetadata(ctx, scope.OrganizationID)
	if err != nil {
		return nil, err
	}
	metadata = visibleSkillMetadata(metadata)
	orgEnabled, err := s.effectiveOrganizationSkillIDs(ctx, scope.OrganizationID, metadata)
	if err != nil {
		return nil, err
	}
	enabled, defaulted, err := s.effectiveAccountSkillPreferenceIDs(ctx, scope, callerType, metadata, orgEnabled)
	if err != nil {
		return nil, err
	}
	return &AccountSkillPreference{EnabledSkillIDs: enabled, Defaulted: defaulted}, nil
}

func (s *service) UpdateAccountSkillPreference(ctx context.Context, scope Scope, callerType string, req runtimedto.UpdateAccountSkillPreferenceRequest) (*AccountSkillPreference, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	callerType = normalizeCallerType(callerType)
	if callerType != runtimemodel.ConversationCallerAIChat {
		return nil, fmt.Errorf("%w: unsupported caller type", ErrInvalidInput)
	}
	metadata, err := s.catalogSkillMetadata(ctx, scope.OrganizationID)
	if err != nil {
		return nil, err
	}
	metadata = visibleSkillMetadata(metadata)
	orgEnabled, err := s.effectiveOrganizationSkillIDs(ctx, scope.OrganizationID, metadata)
	if err != nil {
		return nil, err
	}
	normalized, err := validateSkillIDsForCaller(req.EnabledSkillIDs, metadata, orgEnabled, callerType, nil)
	if err != nil {
		return nil, err
	}
	if s.repos == nil || s.repos.SkillPref == nil {
		return nil, fmt.Errorf("%w: account skill preference repository is not configured", ErrInvalidInput)
	}
	if err := s.repos.SkillPref.Upsert(ctx, &runtimemodel.AccountSkillPreference{
		OrganizationID:  scope.OrganizationID,
		AccountID:       scope.AccountID,
		CallerType:      callerType,
		EnabledSkillIDs: normalized,
	}); err != nil {
		return nil, err
	}
	return &AccountSkillPreference{EnabledSkillIDs: normalized, Defaulted: false}, nil
}

func (s *service) ListConversations(ctx context.Context, scope Scope, page, limit int) ([]*runtimemodel.Conversation, int64, error) {
	return s.ListConversationsByCaller(ctx, scope, Caller{Type: runtimemodel.ConversationCallerAIChat}, page, limit)
}

func (s *service) ListConversationsByCaller(ctx context.Context, scope Scope, caller Caller, page, limit int) ([]*runtimemodel.Conversation, int64, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, 0, err
	}
	limit = clampLimit(limit, 20, 100)
	offset := pageOffset(page, limit)
	return s.repos.Conversation.ListByCallerScoped(ctx, scope.OrganizationID, scope.AccountID, normalizeCallerType(caller.Type), normalizeCallerID(caller.ID), limit, offset)
}

func (s *service) Search(ctx context.Context, scope Scope, query string, limit int) ([]*SearchResult, error) {
	return s.SearchByCaller(ctx, scope, Caller{Type: runtimemodel.ConversationCallerAIChat}, query, limit)
}

func (s *service) SearchByCaller(ctx context.Context, scope Scope, caller Caller, query string, limit int) ([]*SearchResult, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return []*SearchResult{}, nil
	}
	limit = clampLimit(limit, defaultSearchLimit, maxSearchLimit)
	rows, err := s.repos.Conversation.SearchByCallerScoped(
		ctx,
		scope.OrganizationID,
		scope.AccountID,
		normalizeCallerType(caller.Type),
		normalizeCallerID(caller.ID),
		strings.TrimSpace(caller.Source),
		normalizeCallerID(caller.SourceWebAppID),
		query,
		limit,
	)
	if err != nil {
		return nil, err
	}
	results := make([]*SearchResult, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		results = append(results, &SearchResult{
			Type:              row.Type,
			ConversationID:    row.ConversationID,
			ConversationTitle: row.ConversationTitle,
			MessageID:         row.MessageID,
			Snippet:           searchSnippet(row.MatchText, query, searchSnippetRunes),
			UpdatedAt:         row.UpdatedAt,
		})
	}
	return results, nil
}

func searchSnippet(text string, query string, maxRunes int) string {
	text = strings.TrimSpace(collapseWhitespace(text))
	query = strings.TrimSpace(query)
	if text == "" || maxRunes <= 0 {
		return ""
	}
	if utf8.RuneCountInString(text) <= maxRunes {
		return text
	}
	matchStart := 0
	if query != "" {
		lowerText := strings.ToLower(text)
		if idx := strings.Index(lowerText, strings.ToLower(query)); idx >= 0 {
			matchStart = utf8.RuneCountInString(lowerText[:idx])
		}
	}
	runes := []rune(text)
	start := matchStart - maxRunes/3
	if start < 0 {
		start = 0
	}
	end := start + maxRunes
	if end > len(runes) {
		end = len(runes)
		start = end - maxRunes
		if start < 0 {
			start = 0
		}
	}
	snippet := string(runes[start:end])
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(runes) {
		snippet += "..."
	}
	return snippet
}

func collapseWhitespace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func (s *service) GetConversation(ctx context.Context, scope Scope, id uuid.UUID) (*runtimemodel.Conversation, error) {
	return s.GetConversationByCaller(ctx, scope, Caller{Type: runtimemodel.ConversationCallerAIChat}, id)
}

func (s *service) GetConversationByCaller(ctx context.Context, scope Scope, caller Caller, id uuid.UUID) (*runtimemodel.Conversation, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	conversation, err := s.repos.Conversation.GetByCallerScoped(ctx, id, scope.OrganizationID, scope.AccountID, normalizeCallerType(caller.Type), normalizeCallerID(caller.ID))
	if err != nil {
		return nil, mapRepoError(err)
	}
	return conversation, nil
}

func (s *service) UpdateConversation(ctx context.Context, scope Scope, id uuid.UUID, req runtimedto.UpdateConversationRequest) (*runtimemodel.Conversation, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	var conversation *runtimemodel.Conversation
	updates := make(map[string]interface{})
	if req.Title != nil {
		updates["title"] = normalizeTitle(*req.Title, defaultConversationTitle)
	}
	if req.Status != nil {
		status := strings.TrimSpace(*req.Status)
		if status != runtimemodel.ConversationStatusNormal && status != runtimemodel.ConversationStatusArchived {
			return nil, fmt.Errorf("%w: invalid conversation status", ErrInvalidInput)
		}
		updates["status"] = status
	}
	if req.CurrentLeafMessageID != nil {
		leafID, err := uuid.Parse(strings.TrimSpace(*req.CurrentLeafMessageID))
		if err != nil || leafID == uuid.Nil {
			return nil, fmt.Errorf("%w: invalid current leaf message id", ErrInvalidInput)
		}
		conversation, err = s.getConversation(ctx, scope, id)
		if err != nil {
			return nil, err
		}
		if err := s.validateCurrentLeafMessage(ctx, scope, conversation, leafID); err != nil {
			return nil, err
		}
		updates["current_leaf_message_id"] = leafID
	}
	if err := s.repos.Conversation.UpdateScoped(ctx, id, scope.OrganizationID, scope.AccountID, updates); err != nil {
		return nil, mapRepoError(err)
	}
	return s.getConversation(ctx, scope, id)
}

func (s *service) UpdateConversationByCaller(ctx context.Context, scope Scope, caller Caller, id uuid.UUID, req runtimedto.UpdateConversationRequest) (*runtimemodel.Conversation, error) {
	if _, err := s.GetConversationByCaller(ctx, scope, caller, id); err != nil {
		return nil, err
	}
	return s.UpdateConversation(ctx, scope, id, req)
}

func (s *service) validateCurrentLeafMessage(ctx context.Context, scope Scope, conversation *runtimemodel.Conversation, leafID uuid.UUID) error {
	message, err := s.repos.Message.GetScoped(ctx, leafID, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return mapRepoError(err)
	}
	if message.ConversationID != conversation.ID {
		return fmt.Errorf("%w: current leaf message belongs to another conversation", ErrInvalidInput)
	}
	switch message.Status {
	case runtimemodel.MessageStatusCompleted, runtimemodel.MessageStatusStopped, runtimemodel.MessageStatusError:
		return nil
	case runtimemodel.MessageStatusStreaming:
		if conversation.RuntimeStatus == runtimemodel.ConversationRuntimeStatusStreaming &&
			conversation.ActiveMessageID != nil &&
			*conversation.ActiveMessageID == message.ID {
			return nil
		}
		return fmt.Errorf("%w: current leaf message is not the active streaming message", ErrInvalidInput)
	default:
		return fmt.Errorf("%w: invalid current leaf message status", ErrInvalidInput)
	}
}

func (s *service) DeleteConversation(ctx context.Context, scope Scope, id uuid.UUID) error {
	if err := s.ensureMember(ctx, scope); err != nil {
		return err
	}
	return mapRepoError(s.repos.Conversation.DeleteScoped(ctx, id, scope.OrganizationID, scope.AccountID))
}

func (s *service) DeleteConversationByCaller(ctx context.Context, scope Scope, caller Caller, id uuid.UUID) error {
	if _, err := s.GetConversationByCaller(ctx, scope, caller, id); err != nil {
		return err
	}
	return s.DeleteConversation(ctx, scope, id)
}

func (s *service) ListMessages(ctx context.Context, scope Scope, conversationID uuid.UUID, page, limit int) ([]*runtimemodel.Message, int64, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, 0, err
	}
	limit = clampLimit(limit, 50, 200)
	offset := pageOffset(page, limit)
	messages, total, err := s.repos.Message.ListByConversationScoped(ctx, conversationID, scope.OrganizationID, scope.AccountID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	hydrateMessagesGeneratedFileURLs(messages)
	return messages, total, nil
}

func (s *service) ListConversationMessagesByCaller(ctx context.Context, scope Scope, caller Caller, conversationID uuid.UUID, page, limit int) ([]*runtimemodel.Message, int64, error) {
	if _, err := s.GetConversationByCaller(ctx, scope, caller, conversationID); err != nil {
		return nil, 0, err
	}
	return s.ListMessages(ctx, scope, conversationID, page, limit)
}

func (s *service) ListMessagesByCaller(ctx context.Context, scope Scope, caller Caller, page, limit int) ([]*runtimemodel.Message, int64, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, 0, err
	}
	limit = clampLimit(limit, 50, 200)
	offset := pageOffset(page, limit)
	messages, total, err := s.repos.Message.ListByCallerScoped(ctx, scope.OrganizationID, scope.AccountID, normalizeCallerType(caller.Type), normalizeCallerID(caller.ID), limit, offset)
	if err != nil {
		return nil, 0, err
	}
	hydrateMessagesGeneratedFileURLs(messages)
	return messages, total, nil
}

func (s *service) ListMessagesByCallerSource(ctx context.Context, scope Scope, caller Caller, source string, page, limit int) ([]*runtimemodel.Message, int64, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, 0, err
	}
	source = strings.TrimSpace(source)
	if source == "" {
		return s.ListMessagesByCaller(ctx, scope, caller, page, limit)
	}
	limit = clampLimit(limit, 50, 200)
	offset := pageOffset(page, limit)
	messages, total, err := s.repos.Message.ListByCallerSourceScoped(ctx, scope.OrganizationID, scope.AccountID, normalizeCallerType(caller.Type), normalizeCallerID(caller.ID), source, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	hydrateMessagesGeneratedFileURLs(messages)
	return messages, total, nil
}

func (s *service) ListMessagesByCallerLogFilters(ctx context.Context, scope Scope, caller Caller, source string, conversationID *uuid.UUID, queryText string, page, limit int) ([]*runtimemodel.Message, int64, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, 0, err
	}
	limit = clampLimit(limit, 50, 200)
	offset := pageOffset(page, limit)
	messages, total, err := s.repos.Message.ListByCallerLogFilterScoped(ctx, scope.OrganizationID, scope.AccountID, normalizeCallerType(caller.Type), normalizeCallerID(caller.ID), strings.TrimSpace(source), conversationID, strings.TrimSpace(queryText), limit, offset)
	if err != nil {
		return nil, 0, err
	}
	hydrateMessagesGeneratedFileURLs(messages)
	return messages, total, nil
}

func (s *service) ListMessagesByCallerRuntimeLogFilters(ctx context.Context, scope Scope, caller Caller, source string, conversationID *uuid.UUID, queryText string, page, limit int) ([]*runtimemodel.Message, int64, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, 0, err
	}
	limit = clampLimit(limit, 50, 200)
	offset := pageOffset(page, limit)
	messages, total, err := s.repos.Message.ListByCallerRuntimeLogScoped(ctx, scope.OrganizationID, scope.WorkspaceID, scope.AccountID, normalizeCallerType(caller.Type), normalizeCallerID(caller.ID), strings.TrimSpace(source), conversationID, strings.TrimSpace(queryText), limit, offset)
	if err != nil {
		return nil, 0, err
	}
	hydrateMessagesGeneratedFileURLs(messages)
	return messages, total, nil
}

func (s *service) GetMessageByCaller(ctx context.Context, scope Scope, caller Caller, id uuid.UUID) (*runtimemodel.Message, *runtimemodel.Conversation, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, nil, err
	}
	message, err := s.repos.Message.GetScoped(ctx, id, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return nil, nil, mapRepoError(err)
	}
	conversation, err := s.GetConversationByCaller(ctx, scope, caller, message.ConversationID)
	if err != nil {
		return nil, nil, err
	}
	hydrateMessageGeneratedFileURLs(message)
	return message, conversation, nil
}

func (s *service) GetMessageByCallerRuntimeLog(ctx context.Context, scope Scope, caller Caller, id uuid.UUID, source string) (*runtimemodel.Message, *runtimemodel.Conversation, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, nil, err
	}
	normalizedCallerType := normalizeCallerType(caller.Type)
	normalizedCallerID := normalizeCallerID(caller.ID)
	message, err := s.repos.Message.GetRuntimeLogScoped(ctx, id, scope.OrganizationID, scope.WorkspaceID, scope.AccountID, normalizedCallerType, normalizedCallerID, strings.TrimSpace(source))
	if err != nil {
		return nil, nil, mapRepoError(err)
	}
	conversation, err := s.repos.Conversation.GetRuntimeLogScoped(ctx, message.ConversationID, scope.OrganizationID, scope.WorkspaceID, scope.AccountID, normalizedCallerType, normalizedCallerID, strings.TrimSpace(source))
	if err != nil {
		return nil, nil, mapRepoError(err)
	}
	hydrateMessageGeneratedFileURLs(message)
	return message, conversation, nil
}

func (s *service) DeleteMessage(ctx context.Context, scope Scope, id uuid.UUID) error {
	if err := s.ensureMember(ctx, scope); err != nil {
		return err
	}
	err := s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepos := repository.NewRepositories(tx)
		result, err := txRepos.Message.DeleteSubtreeScoped(ctx, id, scope.OrganizationID, scope.AccountID)
		if err != nil {
			return err
		}
		return txRepos.Conversation.RefreshAfterMessageDelete(ctx, result.ConversationID)
	})
	return mapRepoError(err)
}

func (s *service) StopMessage(ctx context.Context, scope Scope, id uuid.UUID) (*runtimemodel.Message, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	message, err := s.repos.Message.GetScoped(ctx, id, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	if !isStoppableMessageStatus(message.Status) {
		hydrateMessageGeneratedFileURLs(message)
		return message, nil
	}

	s.streams.Stop(id)
	metadata := workflowContinuationMetadataWithoutUserInputRequest(message.Metadata)
	if continuation := workflowApprovalContinuationFromMetadata(metadata); continuation.WorkflowRunID != "" {
		metadata = mergeWorkflowRunMetadata(metadata, "workflow_stopped", map[string]interface{}{
			"workflow_run_id": continuation.WorkflowRunID,
			"status":          runtimemodel.MessageStatusStopped,
			"created_at":      time.Now().Unix(),
		})
		metadata = workflowContinuationMetadataWithStatus(metadata, workflowContinuationStatusFailed)
	}
	if err := s.repos.Message.UpdateStoppedAnswer(ctx, id, message.Answer, metadata); err != nil {
		latest, loadErr := s.repos.Message.GetScoped(ctx, id, scope.OrganizationID, scope.AccountID)
		if loadErr == nil && !isStoppableMessageStatus(latest.Status) {
			hydrateMessageGeneratedFileURLs(latest)
			return latest, nil
		}
		return nil, mapRepoError(err)
	}
	if err := s.repos.Conversation.ClearActiveMessage(ctx, message.ConversationID, id); err != nil {
		return nil, mapRepoError(err)
	}
	stopped, err := s.repos.Message.GetScoped(ctx, id, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	hydrateMessageGeneratedFileURLs(stopped)
	return stopped, nil
}

func (s *service) StopConversation(ctx context.Context, scope Scope, id uuid.UUID) (*StopConversationResult, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	conversation, err := s.getConversation(ctx, scope, id)
	if err != nil {
		return nil, err
	}
	if conversation.RuntimeStatus != runtimemodel.ConversationRuntimeStatusStreaming || conversation.ActiveMessageID == nil {
		return &StopConversationResult{Conversation: conversation}, nil
	}

	message, err := s.StopMessage(ctx, scope, *conversation.ActiveMessageID)
	if err != nil {
		return nil, err
	}
	updated, err := s.getConversation(ctx, scope, id)
	if err != nil {
		return nil, err
	}
	return &StopConversationResult{Conversation: updated, Message: message}, nil
}

func (s *service) StopConversationByCaller(ctx context.Context, scope Scope, caller Caller, id uuid.UUID) (*StopConversationResult, error) {
	if _, err := s.GetConversationByCaller(ctx, scope, caller, id); err != nil {
		return nil, err
	}
	return s.StopConversation(ctx, scope, id)
}
