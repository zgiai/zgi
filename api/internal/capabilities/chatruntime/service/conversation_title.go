package service

import (
	"context"
	"strings"
	"time"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	legacyconversation "github.com/zgiai/zgi/api/internal/modules/app/conversation"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"github.com/zgiai/zgi/api/internal/modules/shared/titlegen"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	conversationTitleGenerationStatusKey = "title_generation_status"
	conversationTitleStatusPending       = "pending"
	conversationTitleStatusCompleted     = "completed"
	conversationTitleStatusFailed        = "failed"
	conversationTitleBackfillMaxTurns    = 3
)

type conversationTitleGenerationInput struct {
	Messages          []titlegen.Message
	FallbackTitle     string
	PreferredProvider string
	PreferredModel    string
	LegacyFallback    bool
}

func conversationTitleMessagesFromQuery(query string) []titlegen.Message {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	return []titlegen.Message{{Role: "user", Content: query}}
}

func conversationTitleGenerationStatus(metadata map[string]interface{}) string {
	if metadata == nil {
		return ""
	}
	status, _ := metadata[conversationTitleGenerationStatusKey].(string)
	return strings.TrimSpace(status)
}

func conversationMetadataWithTitleStatus(metadata map[string]interface{}, status string) map[string]interface{} {
	next := make(map[string]interface{}, len(metadata)+1)
	for key, value := range metadata {
		next[key] = value
	}
	next[conversationTitleGenerationStatusKey] = status
	return next
}

func (s *service) markConversationTitleGenerationPending(ctx context.Context, scope Scope, conversation *runtimemodel.Conversation) {
	if s == nil || conversation == nil || conversation.LegacyFallback {
		return
	}
	conversation.Metadata = conversationMetadataWithTitleStatus(conversation.Metadata, conversationTitleStatusPending)
	if s.repos == nil || s.repos.Conversation == nil {
		return
	}
	if err := s.repos.Conversation.UpdateScoped(ctx, conversation.ID, scope.OrganizationID, scope.AccountID, map[string]interface{}{
		"metadata": conversation.Metadata,
	}); err != nil {
		logger.WarnContext(ctx, "failed to mark chat runtime conversation title pending", "conversation_id", conversation.ID.String(), err)
	}
}

func (s *service) enqueueConversationTitleGeneration(
	ctx context.Context,
	scope Scope,
	caller Caller,
	conversation *runtimemodel.Conversation,
	input conversationTitleGenerationInput,
) {
	if s == nil || s.titleGen == nil || conversation == nil || len(input.Messages) == 0 {
		return
	}
	conversationID := conversation.ID
	if _, loaded := s.titleGenerationJobs.LoadOrStore(conversationID, struct{}{}); loaded {
		return
	}
	workspaceID := conversation.WorkspaceID
	appID, appType := conversationTitleAppContext(caller, conversationID)

	go func() {
		defer s.titleGenerationJobs.Delete(conversationID)
		titleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), titleGenerationTimeout)
		defer cancel()

		result, err := s.titleGen.Generate(titleCtx, titlegen.GenerateRequest{
			OrganizationID:    scope.OrganizationID,
			AccountID:         scope.AccountID,
			WorkspaceID:       workspaceID,
			AppID:             appID,
			AppType:           appType,
			SessionID:         conversationID.String(),
			ConversationID:    conversationID.String(),
			Messages:          input.Messages,
			FallbackTitle:     input.FallbackTitle,
			PreferredProvider: input.PreferredProvider,
			PreferredModel:    input.PreferredModel,
			PreferredUseCase:  string(llmmodelmodel.UseCaseTextChat),
		})
		if err != nil || result == nil {
			s.persistConversationTitleGenerationFailure(titleCtx, scope, conversation)
			logger.WarnContext(titleCtx, "failed to generate chat runtime conversation title", "conversation_id", conversationID.String(), "caller_type", normalizeCallerType(caller.Type), err)
			return
		}

		title := normalizeTitle(result.Title, input.FallbackTitle)
		if err := s.persistConversationTitleGenerationResult(titleCtx, scope, conversation, input.FallbackTitle, title, input.LegacyFallback); err != nil {
			logger.WarnContext(titleCtx, "failed to update generated chat runtime conversation title", "conversation_id", conversationID.String(), "caller_type", normalizeCallerType(caller.Type), err)
		}
	}()
}

func (s *service) persistConversationTitleGenerationFailure(ctx context.Context, scope Scope, conversation *runtimemodel.Conversation) {
	if conversation == nil || conversation.LegacyFallback || s.repos == nil || s.repos.Conversation == nil {
		return
	}
	current, err := s.repos.Conversation.GetScoped(ctx, conversation.ID, scope.OrganizationID, scope.AccountID)
	if err != nil || current == nil {
		return
	}
	metadata := conversationMetadataWithTitleStatus(current.Metadata, conversationTitleStatusFailed)
	_ = s.repos.Conversation.UpdateScoped(ctx, conversation.ID, scope.OrganizationID, scope.AccountID, map[string]interface{}{"metadata": metadata})
}

func (s *service) persistConversationTitleGenerationResult(
	ctx context.Context,
	scope Scope,
	conversation *runtimemodel.Conversation,
	expectedTitle string,
	title string,
	legacyFallback bool,
) error {
	if legacyFallback {
		if s.repos == nil || s.repos.DB == nil {
			return nil
		}
		return s.repos.DB.WithContext(ctx).
			Model(&legacyconversation.AgentConversation{}).
			Where("id = ? AND name = ? AND deleted_at IS NULL", conversation.ID, expectedTitle).
			Update("name", title).Error
	}
	if s.repos == nil || s.repos.Conversation == nil {
		return nil
	}
	current, err := s.repos.Conversation.GetScoped(ctx, conversation.ID, scope.OrganizationID, scope.AccountID)
	if err != nil || current == nil {
		return err
	}
	if strings.TrimSpace(current.Title) != strings.TrimSpace(expectedTitle) {
		return nil
	}
	metadata := conversationMetadataWithTitleStatus(current.Metadata, conversationTitleStatusCompleted)
	updates := map[string]interface{}{"metadata": metadata}
	if title != "" && title != current.Title {
		updates["title"] = title
	}
	return s.repos.Conversation.UpdateScoped(ctx, conversation.ID, scope.OrganizationID, scope.AccountID, updates)
}

func (s *service) enqueueConversationTitleBackfills(ctx context.Context, scope Scope, caller Caller, conversations []*runtimemodel.Conversation) {
	if s == nil || s.titleGen == nil || s.repos == nil || s.repos.DB == nil {
		return
	}
	for _, conversation := range conversations {
		s.enqueueConversationTitleBackfill(ctx, scope, caller, conversation)
	}
}

func (s *service) enqueueConversationTitleBackfill(ctx context.Context, scope Scope, caller Caller, conversation *runtimemodel.Conversation) {
	if s == nil || s.titleGen == nil || s.repos == nil || s.repos.DB == nil || conversation == nil || conversation.DialogueCount <= 0 {
		return
	}
	status := conversationTitleGenerationStatus(conversation.Metadata)
	if status == conversationTitleStatusCompleted {
		return
	}

	messages, preferredProvider, preferredModel, err := s.loadConversationTitleBackfillMessages(ctx, scope, conversation)
	if err != nil || len(messages) == 0 {
		return
	}
	if status == "" && !conversationTitleLooksUnprocessed(conversation.Title, messages[0].Content) {
		return
	}

	if conversation.LegacyFallback {
		conversation.Metadata = conversationMetadataWithTitleStatus(conversation.Metadata, conversationTitleStatusPending)
	} else {
		s.markConversationTitleGenerationPending(ctx, scope, conversation)
	}
	s.enqueueConversationTitleGeneration(ctx, scope, caller, conversation, conversationTitleGenerationInput{
		Messages:          messages,
		FallbackTitle:     conversation.Title,
		PreferredProvider: preferredProvider,
		PreferredModel:    preferredModel,
		LegacyFallback:    conversation.LegacyFallback,
	})
}

func (s *service) loadConversationTitleBackfillMessages(
	ctx context.Context,
	scope Scope,
	conversation *runtimemodel.Conversation,
) ([]titlegen.Message, string, string, error) {
	if conversation.LegacyFallback {
		var rows []*legacyconversation.AgentMessage
		err := s.repos.DB.WithContext(ctx).
			Where("conversation_id = ? AND deleted_at IS NULL", conversation.ID).
			Order("created_at ASC, id ASC").
			Limit(conversationTitleBackfillMaxTurns).
			Find(&rows).Error
		messages := make([]titlegen.Message, 0, len(rows)*2)
		preferredProvider := ""
		preferredModel := ""
		for _, row := range rows {
			if preferredProvider == "" && row.ModelProvider != nil {
				preferredProvider = strings.TrimSpace(*row.ModelProvider)
			}
			if preferredModel == "" && row.ModelVersionID != nil {
				preferredModel = strings.TrimSpace(*row.ModelVersionID)
			}
			messages = appendConversationTitleTurn(messages, row.Query, row.Answer)
		}
		return messages, preferredProvider, preferredModel, err
	}

	var rows []*runtimemodel.Message
	err := s.repos.DB.WithContext(ctx).
		Table("chat_runtime_messages AS m").
		Joins("JOIN chat_runtime_conversations AS c ON c.id = m.conversation_id").
		Where("m.conversation_id = ? AND c.organization_id = ? AND c.account_id = ? AND m.deleted_at IS NULL AND c.deleted_at IS NULL", conversation.ID, scope.OrganizationID, scope.AccountID).
		Select("m.*").
		Order("m.created_at ASC, m.id ASC").
		Limit(conversationTitleBackfillMaxTurns).
		Find(&rows).Error
	messages := make([]titlegen.Message, 0, len(rows)*2)
	preferredProvider := ""
	preferredModel := ""
	for _, row := range rows {
		if preferredProvider == "" && row.ModelProvider != nil {
			preferredProvider = strings.TrimSpace(*row.ModelProvider)
		}
		if preferredModel == "" {
			preferredModel = strings.TrimSpace(row.ModelName)
		}
		messages = appendConversationTitleTurn(messages, row.Query, row.Answer)
	}
	return messages, preferredProvider, preferredModel, err
}

func appendConversationTitleTurn(messages []titlegen.Message, query string, answer string) []titlegen.Message {
	if query = strings.TrimSpace(query); query != "" {
		messages = append(messages, titlegen.Message{Role: "user", Content: query})
	}
	if answer = strings.TrimSpace(answer); answer != "" {
		messages = append(messages, titlegen.Message{Role: "assistant", Content: answer})
	}
	return messages
}

func conversationTitleLooksUnprocessed(title string, firstQuery string) bool {
	normalized := strings.TrimSpace(title)
	if normalized == "" || normalized == defaultConversationTitle || normalized == "New Conversation" || normalized == "New conversation" || normalized == "\u65b0\u5efa\u4f1a\u8bdd" {
		return true
	}
	if normalized == conversationTitleFallback(firstQuery, initialConversationTitle()) {
		return true
	}
	for _, prefix := range []string{"Conversation ", "\u4f1a\u8bdd "} {
		timestamp := strings.TrimPrefix(normalized, prefix)
		if timestamp != normalized && timestamp != "" {
			if _, err := time.Parse("2006-01-02 15:04:05", timestamp); err == nil {
				return true
			}
		}
	}
	return false
}
