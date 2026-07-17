package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	legacyconversation "github.com/zgiai/zgi/api/internal/modules/app/conversation"
	pkguuid "github.com/zgiai/zgi/api/pkg/uuid"
	"gorm.io/gorm"
)

const legacyImageWorkflowScenario = "imagegen_chat"

func (s *service) listImageConversationsWithLegacy(ctx context.Context, scope Scope, caller Caller, page, limit int) ([]*runtimemodel.Conversation, int64, error) {
	limit = clampLimit(limit, 20, 100)
	offset := pageOffset(page, limit)
	fetchLimit := offset + limit
	if fetchLimit < limit {
		fetchLimit = limit
	}

	current, currentTotal, err := s.repos.Conversation.ListByCallerScoped(ctx, scope.OrganizationID, scope.AccountID, normalizeCallerType(caller.Type), normalizeCallerID(caller.ID), runtimemodel.ConversationTypeImage, fetchLimit, 0)
	if err != nil {
		return nil, 0, err
	}
	legacy, legacyTotal, err := s.listLegacyImageConversations(ctx, scope, fetchLimit, 0)
	if err != nil {
		return nil, 0, err
	}

	merged := append([]*runtimemodel.Conversation{}, current...)
	merged = append(merged, legacy...)
	sort.SliceStable(merged, func(i, j int) bool {
		if !merged[i].UpdatedAt.Equal(merged[j].UpdatedAt) {
			return merged[i].UpdatedAt.After(merged[j].UpdatedAt)
		}
		if !merged[i].CreatedAt.Equal(merged[j].CreatedAt) {
			return merged[i].CreatedAt.After(merged[j].CreatedAt)
		}
		return merged[i].ID.String() > merged[j].ID.String()
	})

	end := offset + limit
	if offset >= len(merged) {
		return []*runtimemodel.Conversation{}, currentTotal + legacyTotal, nil
	}
	if end > len(merged) {
		end = len(merged)
	}
	return merged[offset:end], currentTotal + legacyTotal, nil
}

func (s *service) getLegacyImageConversation(ctx context.Context, scope Scope, id uuid.UUID) (*runtimemodel.Conversation, error) {
	var legacy legacyconversation.AgentConversation
	err := s.repos.DB.WithContext(ctx).
		Where("id = ? AND agent_id = ? AND deleted_at IS NULL", id, legacyImageWorkflowAgentID()).
		Where("(from_account_id = ? OR created_by = ?)", scope.AccountID, scope.AccountID).
		Where("NOT EXISTS (?)",
			s.repos.DB.Model(&runtimemodel.Conversation{}).
				Select("1").
				Where("source_conversation_id = agents_conversations.id AND deleted_at IS NULL"),
		).
		Take(&legacy).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get legacy image conversation: %w", err)
	}
	return legacyImageConversationToRuntime(scope, &legacy), nil
}

func (s *service) listLegacyImageConversations(ctx context.Context, scope Scope, limit, offset int) ([]*runtimemodel.Conversation, int64, error) {
	var rows []*legacyconversation.AgentConversation
	var total int64
	query := s.repos.DB.WithContext(ctx).Model(&legacyconversation.AgentConversation{}).
		Where("agent_id = ? AND deleted_at IS NULL", legacyImageWorkflowAgentID()).
		Where("(from_account_id = ? OR created_by = ?)", scope.AccountID, scope.AccountID).
		Where("NOT EXISTS (?)",
			s.repos.DB.Model(&runtimemodel.Conversation{}).
				Select("1").
				Where("source_conversation_id = agents_conversations.id AND deleted_at IS NULL"),
		)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count legacy image conversations: %w", err)
	}
	if err := query.Order("updated_at DESC, created_at DESC, id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list legacy image conversations: %w", err)
	}
	out := make([]*runtimemodel.Conversation, 0, len(rows))
	for _, row := range rows {
		out = append(out, legacyImageConversationToRuntime(scope, row))
	}
	return out, total, nil
}

func (s *service) listLegacyImageMessages(ctx context.Context, scope Scope, conversationID uuid.UUID, page, limit int) ([]*runtimemodel.Message, int64, error) {
	if _, err := s.getLegacyImageConversation(ctx, scope, conversationID); err != nil {
		return nil, 0, err
	}
	limit = clampLimit(limit, 50, 200)
	offset := pageOffset(page, limit)
	var rows []*legacyconversation.AgentMessage
	var total int64
	query := s.repos.DB.WithContext(ctx).Model(&legacyconversation.AgentMessage{}).
		Where("conversation_id = ? AND deleted_at IS NULL", conversationID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count legacy image messages: %w", err)
	}
	if err := query.Order("created_at ASC, id ASC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list legacy image messages: %w", err)
	}
	messages := make([]*runtimemodel.Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, legacyImageMessageToRuntime(row))
	}
	hydrateMessagesGeneratedFileState(ctx, messages)
	hydrateMessagesPublicErrors(messages)
	return messages, total, nil
}

func (s *service) searchLegacyImageConversations(ctx context.Context, scope Scope, queryText string, limit int) ([]*SearchResult, error) {
	keyword := strings.TrimSpace(queryText)
	if keyword == "" || limit <= 0 {
		return []*SearchResult{}, nil
	}
	pattern := "%" + strings.ToLower(legacyImageEscapeLikePattern(keyword)) + "%"
	var rows []struct {
		Type              string
		ConversationID    uuid.UUID
		ConversationTitle string
		MessageID         *uuid.UUID
		MatchText         string
		UpdatedAt         time.Time
	}
	err := s.repos.DB.WithContext(ctx).Raw(`
		SELECT *
		FROM (
			SELECT
				'conversation' AS type,
				c.id AS conversation_id,
				c.name AS conversation_title,
				NULL::uuid AS message_id,
				c.name AS match_text,
				c.updated_at AS updated_at,
				0 AS rank
			FROM agents_conversations AS c
			WHERE c.agent_id = ?
				AND (c.from_account_id = ? OR c.created_by = ?)
				AND c.deleted_at IS NULL
				AND NOT EXISTS (
					SELECT 1 FROM chat_runtime_conversations rtc
					WHERE rtc.source_conversation_id = c.id AND rtc.deleted_at IS NULL
				)
				AND LOWER(COALESCE(c.name, '')) LIKE ? ESCAPE '\'
			UNION ALL
			SELECT
				'message' AS type,
				c.id AS conversation_id,
				c.name AS conversation_title,
				m.id AS message_id,
				CASE
					WHEN LOWER(COALESCE(m.query, '')) LIKE ? ESCAPE '\' THEN m.query
					ELSE m.answer
				END AS match_text,
				GREATEST(m.updated_at, c.updated_at) AS updated_at,
				1 AS rank
			FROM agents_messages AS m
			JOIN agents_conversations AS c ON c.id = m.conversation_id
			WHERE c.agent_id = ?
				AND (c.from_account_id = ? OR c.created_by = ?)
				AND c.deleted_at IS NULL
				AND m.deleted_at IS NULL
				AND NOT EXISTS (
					SELECT 1 FROM chat_runtime_conversations rtc
					WHERE rtc.source_conversation_id = c.id AND rtc.deleted_at IS NULL
				)
				AND (
					LOWER(COALESCE(m.query, '')) LIKE ? ESCAPE '\'
					OR LOWER(COALESCE(m.answer, '')) LIKE ? ESCAPE '\'
				)
		) AS matches
		ORDER BY rank ASC, updated_at DESC
		LIMIT ?
	`, legacyImageWorkflowAgentID(), scope.AccountID, scope.AccountID, pattern, pattern, legacyImageWorkflowAgentID(), scope.AccountID, scope.AccountID, pattern, pattern, limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to search legacy image conversations: %w", err)
	}
	results := make([]*SearchResult, 0, len(rows))
	for _, row := range rows {
		results = append(results, &SearchResult{
			Type:              row.Type,
			ConversationID:    row.ConversationID,
			ConversationTitle: row.ConversationTitle,
			MessageID:         row.MessageID,
			Snippet:           searchSnippet(row.MatchText, keyword, searchSnippetRunes),
			UpdatedAt:         row.UpdatedAt,
		})
	}
	return results, nil
}

func legacyImageConversationToRuntime(scope Scope, legacy *legacyconversation.AgentConversation) *runtimemodel.Conversation {
	sourceID := legacy.ID
	conversation := &runtimemodel.Conversation{
		ID:                   legacy.ID,
		OrganizationID:       scope.OrganizationID,
		WorkspaceID:          scope.WorkspaceID,
		AccountID:            scope.AccountID,
		CallerType:           runtimemodel.ConversationCallerAIChat,
		ConversationType:     runtimemodel.ConversationTypeImage,
		Title:                normalizeTitle(legacy.Name, defaultConversationTitle),
		Status:               runtimemodel.ConversationStatusNormal,
		RuntimeStatus:        runtimemodel.ConversationRuntimeStatusIdle,
		DialogueCount:        legacy.DialogueCount,
		Source:               runtimemodel.ConversationSourceMigration,
		SourceConversationID: &sourceID,
		Metadata: map[string]interface{}{
			"legacy_workflow_image": true,
		},
		CreatedAt: legacy.CreatedAt,
		UpdatedAt: legacy.UpdatedAt,
	}
	if legacy.WebAppID != nil {
		if webAppID, err := uuid.Parse(strings.TrimSpace(*legacy.WebAppID)); err == nil && webAppID != uuid.Nil {
			conversation.SourceWebAppID = &webAppID
		}
	}
	return conversation
}

func legacyImageMessageToRuntime(legacy *legacyconversation.AgentMessage) *runtimemodel.Message {
	metadata := legacyImageMessageMetadata(legacy)
	message := &runtimemodel.Message{
		ID:              legacy.ID,
		ConversationID:  legacy.ConversationID,
		ParentID:        legacy.ParentMessageID,
		Query:           legacy.Query,
		Answer:          legacy.Answer,
		Status:          legacyImageMessageStatus(legacy.Status, legacy.Error),
		Error:           legacy.Error,
		ModelProvider:   legacy.ModelProvider,
		ModelName:       legacyImageModelName(legacy.ModelVersionID),
		ModelParameters: map[string]interface{}{},
		Metadata:        metadata,
		SourceMessageID: &legacy.ID,
		CreatedAt:       legacy.CreatedAt,
		UpdatedAt:       legacy.UpdatedAt,
	}
	return message
}

func legacyImageMessageMetadata(legacy *legacyconversation.AgentMessage) map[string]interface{} {
	metadata := map[string]interface{}{
		"migrated_from": "agents_messages",
	}
	raw, err := legacy.GetMessageMetadataAsMap()
	if err == nil {
		for key, value := range raw {
			metadata[key] = value
		}
	}
	files := generatedFilesFromMetadata(metadata["generated_files"])
	if len(files) > 0 {
		metadata["image_generation"] = map[string]interface{}{
			"provider":    legacyImageStringPtrValue(legacy.ModelProvider),
			"model":       legacyImageModelName(legacy.ModelVersionID),
			"model_label": legacyImageModelName(legacy.ModelVersionID),
			"prompt":      legacy.Query,
			"files":       mapsToInterfaceSlice(files),
		}
	}
	return metadata
}

func legacyImageMessageStatus(status string, messageError *string) string {
	if messageError != nil && *messageError != "" {
		return runtimemodel.MessageStatusError
	}
	switch status {
	case legacyconversation.AgentMessageStatusError:
		return runtimemodel.MessageStatusError
	case legacyconversation.AgentMessageStatusStopped:
		return runtimemodel.MessageStatusStopped
	default:
		return runtimemodel.MessageStatusCompleted
	}
}

func legacyImageModelName(value *string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return "unknown"
	}
	return strings.TrimSpace(*value)
}

func legacyImageWorkflowAgentID() uuid.UUID {
	return pkguuid.GenerateBuiltInWorkflowUUID(legacyImageWorkflowScenario)
}

func legacyImageStringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func legacyImageEscapeLikePattern(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}
