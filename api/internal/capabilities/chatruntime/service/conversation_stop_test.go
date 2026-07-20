package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
)

func TestStopConversationStopsWaitingLeafMessage(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()

	conversationRepo := &stopConversationRepo{
		conversation: &runtimemodel.Conversation{
			ID:                   conversationID,
			OrganizationID:       organizationID,
			AccountID:            accountID,
			RuntimeStatus:        runtimemodel.ConversationRuntimeStatusIdle,
			CurrentLeafMessageID: &messageID,
		},
	}
	messageRepo := &stopConversationMessageRepo{
		message: &runtimemodel.Message{
			ID:             messageID,
			ConversationID: conversationID,
			Status:         runtimemodel.MessageStatusWaitingApproval,
			Metadata: map[string]interface{}{
				"agent_workflow_continuation": map[string]interface{}{
					"workflow_run_id": "workflow-run-1",
					"status":          workflowContinuationStatusWaitingApproval,
				},
			},
		},
	}
	svc := &service{
		repos: &repository.Repositories{
			Access:       stopConversationAccessRepo{},
			Conversation: conversationRepo,
			Message:      messageRepo,
		},
		streams: newStreamRegistry(),
	}

	result, err := svc.StopConversation(context.Background(), Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
	}, conversationID)
	if err != nil {
		t.Fatalf("StopConversation() error = %v", err)
	}
	if result.Message == nil {
		t.Fatal("StopConversation() message = nil, want stopped waiting message")
	}
	if result.Message.Status != runtimemodel.MessageStatusStopped {
		t.Fatalf("message status = %q, want %q", result.Message.Status, runtimemodel.MessageStatusStopped)
	}
	if !messageRepo.updateStoppedCalled {
		t.Fatal("UpdateStoppedAnswer was not called")
	}
	if !conversationRepo.clearActiveCalled {
		t.Fatal("ClearActiveMessage was not called")
	}
}

func TestStopConversationIgnoresCompletedLeafMessage(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()

	messageRepo := &stopConversationMessageRepo{
		message: &runtimemodel.Message{
			ID:             messageID,
			ConversationID: conversationID,
			Status:         runtimemodel.MessageStatusCompleted,
		},
	}
	svc := &service{
		repos: &repository.Repositories{
			Access: stopConversationAccessRepo{},
			Conversation: &stopConversationRepo{conversation: &runtimemodel.Conversation{
				ID:                   conversationID,
				OrganizationID:       organizationID,
				AccountID:            accountID,
				RuntimeStatus:        runtimemodel.ConversationRuntimeStatusIdle,
				CurrentLeafMessageID: &messageID,
			}},
			Message: messageRepo,
		},
		streams: newStreamRegistry(),
	}

	result, err := svc.StopConversation(context.Background(), Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
	}, conversationID)
	if err != nil {
		t.Fatalf("StopConversation() error = %v", err)
	}
	if result.Message != nil {
		t.Fatalf("StopConversation() message = %#v, want nil", result.Message)
	}
	if messageRepo.updateStoppedCalled {
		t.Fatal("completed message must not be stopped")
	}
}

type stopConversationAccessRepo struct {
	repository.AccessRepository
}

func (stopConversationAccessRepo) IsOrganizationMember(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}

type stopConversationRepo struct {
	repository.ConversationRepository
	conversation      *runtimemodel.Conversation
	clearActiveCalled bool
}

func (r *stopConversationRepo) GetScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*runtimemodel.Conversation, error) {
	return r.conversation, nil
}

func (r *stopConversationRepo) ClearActiveMessage(context.Context, uuid.UUID, uuid.UUID) error {
	r.clearActiveCalled = true
	return nil
}

type stopConversationMessageRepo struct {
	repository.MessageRepository
	message             *runtimemodel.Message
	updateStoppedCalled bool
}

func (r *stopConversationMessageRepo) GetScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*runtimemodel.Message, error) {
	return r.message, nil
}

func (r *stopConversationMessageRepo) UpdateStoppedAnswer(_ context.Context, _ uuid.UUID, answer string, metadata map[string]interface{}) error {
	r.updateStoppedCalled = true
	r.message.Answer = answer
	r.message.Metadata = metadata
	r.message.Status = runtimemodel.MessageStatusStopped
	return nil
}
