package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
)

func TestCreateConversationForChatStoresSurfaceMetadata(t *testing.T) {
	workspaceID := uuid.New()
	conversationRepo := &capturingConversationRepo{}
	svc := &service{
		repos: &repository.Repositories{
			Access:       surfaceAccessRepo{},
			Conversation: conversationRepo,
		},
	}

	conversation, err := svc.createConversationForChat(context.Background(), Scope{
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
		WorkspaceID:    &workspaceID,
	}, Caller{Type: runtimemodel.ConversationCallerAIChat}, "hello from the sidebar", aiChatSurfaceContextualSidebar)
	if err != nil {
		t.Fatalf("createConversationForChat: %v", err)
	}
	if conversation == nil || conversation.Metadata["surface"] != aiChatSurfaceContextualSidebar {
		t.Fatalf("conversation surface metadata = %v, want %q", conversation.Metadata["surface"], aiChatSurfaceContextualSidebar)
	}
	if conversationRepo.created == nil || conversationRepo.created.Metadata["surface"] != aiChatSurfaceContextualSidebar {
		t.Fatalf("stored conversation surface metadata = %v, want %q", conversationRepo.created.Metadata["surface"], aiChatSurfaceContextualSidebar)
	}
}

func TestListConversationsBySurfaceNormalizesSurface(t *testing.T) {
	conversationRepo := &capturingConversationRepo{}
	svc := &service{
		repos: &repository.Repositories{
			Access:       surfaceAccessRepo{},
			Conversation: conversationRepo,
		},
	}

	_, _, err := svc.ListConversationsBySurface(context.Background(), Scope{
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
	}, "sidebar", 1, 20)
	if err != nil {
		t.Fatalf("ListConversationsBySurface: %v", err)
	}
	if conversationRepo.listSurface != aiChatSurfaceContextualSidebar {
		t.Fatalf("list surface = %q, want %q", conversationRepo.listSurface, aiChatSurfaceContextualSidebar)
	}
}

func TestUpdateConversationAllowsPausedRuntimeMessageAsCurrentLeaf(t *testing.T) {
	for _, status := range []string{
		runtimemodel.MessageStatusWaitingApproval,
		runtimemodel.MessageStatusWaitingQuestion,
		runtimemodel.MessageStatusWaitingClientAction,
	} {
		t.Run(status, func(t *testing.T) {
			organizationID := uuid.New()
			accountID := uuid.New()
			conversationID := uuid.New()
			messageID := uuid.New()
			messageIDString := messageID.String()
			conversationRepo := &capturingConversationRepo{
				conversation: &runtimemodel.Conversation{
					ID:             conversationID,
					OrganizationID: organizationID,
					AccountID:      accountID,
					Status:         runtimemodel.ConversationStatusNormal,
					RuntimeStatus:  runtimemodel.ConversationRuntimeStatusIdle,
					Metadata:       map[string]interface{}{},
				},
			}
			messageRepo := pausedLeafMessageRepo{
				message: &runtimemodel.Message{
					ID:             messageID,
					ConversationID: conversationID,
					Status:         status,
					Metadata:       map[string]interface{}{},
				},
			}
			svc := &service{
				repos: &repository.Repositories{
					Access:       surfaceAccessRepo{},
					Conversation: conversationRepo,
					Message:      messageRepo,
				},
			}

			conversation, err := svc.UpdateConversation(context.Background(), Scope{
				OrganizationID: organizationID,
				AccountID:      accountID,
			}, conversationID, runtimedto.UpdateConversationRequest{
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

type surfaceAccessRepo struct {
	repository.AccessRepository
}

func (surfaceAccessRepo) IsOrganizationMember(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}

type capturingConversationRepo struct {
	repository.ConversationRepository
	created      *runtimemodel.Conversation
	conversation *runtimemodel.Conversation
	listSurface  string
	updated      map[string]interface{}
}

func (r *capturingConversationRepo) Create(_ context.Context, conversation *runtimemodel.Conversation) error {
	r.created = conversation
	return nil
}

func (r *capturingConversationRepo) GetScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*runtimemodel.Conversation, error) {
	return r.conversation, nil
}

func (r *capturingConversationRepo) ListByCallerSurfaceScoped(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ string, _ *uuid.UUID, surface string, _ int, _ int) ([]*runtimemodel.Conversation, int64, error) {
	r.listSurface = surface
	return nil, 0, nil
}

func (r *capturingConversationRepo) UpdateScoped(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID, updates map[string]interface{}) error {
	r.updated = updates
	if r.conversation == nil {
		return nil
	}
	if leafID, ok := updates["current_leaf_message_id"].(uuid.UUID); ok {
		r.conversation.CurrentLeafMessageID = &leafID
	}
	return nil
}

type pausedLeafMessageRepo struct {
	repository.MessageRepository
	message *runtimemodel.Message
}

func (r pausedLeafMessageRepo) GetScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*runtimemodel.Message, error) {
	return r.message, nil
}
