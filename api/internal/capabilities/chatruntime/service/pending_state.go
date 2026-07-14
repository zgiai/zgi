package service

import (
	"context"
	"errors"
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
) error {
	if s == nil || s.repos == nil || s.repos.Message == nil || s.repos.Conversation == nil {
		return fmt.Errorf("aichat repositories are not configured")
	}
	if updateMessage == nil || finishConversation == nil {
		return fmt.Errorf("pending state persistence callbacks are required")
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
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
		return err
	}
	return nil
}
