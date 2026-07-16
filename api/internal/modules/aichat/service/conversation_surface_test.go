//go:build legacy_aichat_service
// +build legacy_aichat_service

package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	aichatdto "github.com/zgiai/zgi/api/internal/modules/aichat/dto"
	aichatmodel "github.com/zgiai/zgi/api/internal/modules/aichat/model"
	"github.com/zgiai/zgi/api/internal/modules/aichat/repository"
)

func TestUpdateConversationAllowsPausedAIChatMessageAsCurrentLeaf(t *testing.T) {
	for _, status := range []string{
		aichatmodel.MessageStatusPending,
		aichatmodel.MessageStatusWaitingApproval,
		aichatmodel.MessageStatusWaitingQuestion,
		aichatmodel.MessageStatusWaitingClientAction,
	} {
		t.Run(status, func(t *testing.T) {
			organizationID := uuid.New()
			accountID := uuid.New()
			conversationID := uuid.New()
			messageID := uuid.New()
			messageIDString := messageID.String()
			conversationRepo := &capturingAIChatConversationRepo{
				conversation: &aichatmodel.Conversation{
					ID:             conversationID,
					OrganizationID: organizationID,
					AccountID:      accountID,
					Status:         aichatmodel.ConversationStatusNormal,
					RuntimeStatus:  aichatmodel.ConversationRuntimeStatusIdle,
					Metadata:       map[string]interface{}{},
				},
			}
			messageRepo := pausedAIChatLeafMessageRepo{
				message: &aichatmodel.Message{
					ID:             messageID,
					ConversationID: conversationID,
					Status:         status,
					Metadata:       map[string]interface{}{},
				},
			}
			svc := &service{
				repos: &repository.Repositories{
					Access:       aiChatSurfaceAccessRepo{},
					Conversation: conversationRepo,
					Message:      messageRepo,
				},
			}

			conversation, err := svc.UpdateConversation(context.Background(), Scope{
				OrganizationID: organizationID,
				AccountID:      accountID,
			}, conversationID, aichatdto.UpdateConversationRequest{
				CurrentLeafMessageID: &messageIDString,
			})
			if err != nil {
				t.Fatalf("UpdateConversation() error = %v", err)
			}
			if conversation.CurrentLeafMessageID == nil || *conversation.CurrentLeafMessageID != messageID {
				t.Fatalf("current leaf = %#v, want %s", conversation.CurrentLeafMessageID, messageID)
			}
			if updated, ok := conversationRepo.updated["current_leaf_message_id"].(uuid.UUID); !ok || updated != messageID {
				t.Fatalf("updated current leaf = %#v, want %s", conversationRepo.updated["current_leaf_message_id"], messageID)
			}
		})
	}
}

type aiChatSurfaceAccessRepo struct {
	repository.AccessRepository
}

func (aiChatSurfaceAccessRepo) IsOrganizationMember(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}

type capturingAIChatConversationRepo struct {
	repository.ConversationRepository
	conversation *aichatmodel.Conversation
	updated      map[string]interface{}
}

func (r *capturingAIChatConversationRepo) GetScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*aichatmodel.Conversation, error) {
	return r.conversation, nil
}

func (r *capturingAIChatConversationRepo) UpdateScoped(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID, updates map[string]interface{}) error {
	r.updated = updates
	if r.conversation == nil {
		return nil
	}
	if leafID, ok := updates["current_leaf_message_id"].(uuid.UUID); ok {
		r.conversation.CurrentLeafMessageID = &leafID
	}
	return nil
}

type pausedAIChatLeafMessageRepo struct {
	repository.MessageRepository
	message *aichatmodel.Message
}

func (r pausedAIChatLeafMessageRepo) GetScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*aichatmodel.Message, error) {
	return r.message, nil
}
