package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	aichatdto "github.com/zgiai/zgi/api/internal/modules/aichat/dto"
	aichatmodel "github.com/zgiai/zgi/api/internal/modules/aichat/model"
	"github.com/zgiai/zgi/api/internal/modules/aichat/repository"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"gorm.io/gorm"
)

func (s *service) CreateConversation(ctx context.Context, scope Scope, title string) (*aichatmodel.Conversation, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	workspaceID, err := s.resolveWorkspaceID(ctx, scope)
	if err != nil {
		return nil, err
	}
	title = normalizeTitle(title, defaultConversationTitle)
	conversation := &aichatmodel.Conversation{
		OrganizationID: scope.OrganizationID,
		WorkspaceID:    workspaceID,
		AccountID:      scope.AccountID,
		Title:          title,
		Status:         aichatmodel.ConversationStatusNormal,
		Source:         aichatmodel.ConversationSourceConsole,
	}
	if err := s.repos.Conversation.Create(ctx, conversation); err != nil {
		return nil, err
	}
	return conversation, nil
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

func (s *service) UpdateSkillConfig(ctx context.Context, scope Scope, req aichatdto.UpdateSkillConfigRequest) (*SkillConfig, error) {
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

func (s *service) ListConversations(ctx context.Context, scope Scope, page, limit int) ([]*aichatmodel.Conversation, int64, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, 0, err
	}
	limit = clampLimit(limit, 20, 100)
	offset := pageOffset(page, limit)
	return s.repos.Conversation.ListScoped(ctx, scope.OrganizationID, scope.AccountID, limit, offset)
}

func (s *service) GetConversation(ctx context.Context, scope Scope, id uuid.UUID) (*aichatmodel.Conversation, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	return s.getConversation(ctx, scope, id)
}

func (s *service) UpdateConversation(ctx context.Context, scope Scope, id uuid.UUID, req aichatdto.UpdateConversationRequest) (*aichatmodel.Conversation, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	var conversation *aichatmodel.Conversation
	updates := make(map[string]interface{})
	if req.Title != nil {
		updates["title"] = normalizeTitle(*req.Title, defaultConversationTitle)
	}
	if req.Status != nil {
		status := strings.TrimSpace(*req.Status)
		if status != aichatmodel.ConversationStatusNormal && status != aichatmodel.ConversationStatusArchived {
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

func (s *service) validateCurrentLeafMessage(ctx context.Context, scope Scope, conversation *aichatmodel.Conversation, leafID uuid.UUID) error {
	message, err := s.repos.Message.GetScoped(ctx, leafID, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return mapRepoError(err)
	}
	if message.ConversationID != conversation.ID {
		return fmt.Errorf("%w: current leaf message belongs to another conversation", ErrInvalidInput)
	}
	switch message.Status {
	case aichatmodel.MessageStatusCompleted, aichatmodel.MessageStatusStopped, aichatmodel.MessageStatusError:
		return nil
	case aichatmodel.MessageStatusStreaming:
		if conversation.RuntimeStatus == aichatmodel.ConversationRuntimeStatusStreaming &&
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

func (s *service) ListMessages(ctx context.Context, scope Scope, conversationID uuid.UUID, page, limit int) ([]*aichatmodel.Message, int64, error) {
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

func (s *service) StopMessage(ctx context.Context, scope Scope, id uuid.UUID) (*aichatmodel.Message, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	message, err := s.repos.Message.GetScoped(ctx, id, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	if !isActiveMessageStatus(message.Status) {
		hydrateMessageGeneratedFileURLs(message)
		return message, nil
	}

	s.streams.Stop(id)
	if err := s.repos.Message.MarkStopped(ctx, id); err != nil {
		latest, loadErr := s.repos.Message.GetScoped(ctx, id, scope.OrganizationID, scope.AccountID)
		if loadErr == nil && !isActiveMessageStatus(latest.Status) {
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
	if conversation.RuntimeStatus != aichatmodel.ConversationRuntimeStatusStreaming || conversation.ActiveMessageID == nil {
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
