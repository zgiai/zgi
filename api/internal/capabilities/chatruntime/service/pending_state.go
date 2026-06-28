package service

import (
	"context"
	"fmt"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

func (s *service) persistPendingMessageAndFinishConversationBestEffort(
	ctx context.Context,
	prepared *PreparedChat,
	pendingKind string,
	updateMessage func(repository.MessageRepository) error,
	finishConversation func(repository.ConversationRepository) error,
) bool {
	if s == nil || s.repos == nil || s.repos.Message == nil || s.repos.Conversation == nil {
		return false
	}
	if updateMessage == nil || finishConversation == nil {
		return false
	}
	var err error
	if s.repos.DB != nil {
		err = s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			txRepos := repository.NewRepositories(tx)
			if err := updateMessage(txRepos.Message); err != nil {
				return err
			}
			if err := finishConversation(txRepos.Conversation); err != nil {
				return err
			}
			return nil
		})
	} else if err = updateMessage(s.repos.Message); err == nil {
		err = finishConversation(s.repos.Conversation)
	}
	if err != nil {
		messageID := ""
		conversationID := ""
		if prepared != nil && prepared.Message != nil {
			messageID = prepared.Message.ID.String()
		}
		if prepared != nil && prepared.Conversation != nil {
			conversationID = prepared.Conversation.ID.String()
		}
		logger.WarnContext(ctx, fmt.Sprintf("failed to persist aichat %s pending state", pendingKind),
			"conversation_id", conversationID,
			"message_id", messageID,
			err,
		)
		return false
	}
	return true
}
