package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
)

func TestEnsureConversationAllowsNewTurnRejectsWaitingApprovalLeaf(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	messageID := uuid.New()
	conversation := &runtimemodel.Conversation{
		ID:                   uuid.New(),
		OrganizationID:       organizationID,
		AccountID:            accountID,
		RuntimeStatus:        runtimemodel.ConversationRuntimeStatusIdle,
		CurrentLeafMessageID: &messageID,
	}
	svc := &service{
		repos: &repository.Repositories{
			Message: fakeWaitingMessageRepo{messageID: messageID, status: runtimemodel.MessageStatusWaitingApproval},
		},
	}

	err := svc.ensureConversationAllowsNewTurn(context.Background(), Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
	}, conversation)
	if !errors.Is(err, ErrConversationWaitingApproval) {
		t.Fatalf("ensureConversationAllowsNewTurn() error = %v, want ErrConversationWaitingApproval", err)
	}
}

func TestEnsureConversationAllowsNewTurnRejectsWaitingClientActionLeaf(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	messageID := uuid.New()
	conversation := &runtimemodel.Conversation{
		ID:                   uuid.New(),
		OrganizationID:       organizationID,
		AccountID:            accountID,
		RuntimeStatus:        runtimemodel.ConversationRuntimeStatusIdle,
		CurrentLeafMessageID: &messageID,
	}
	svc := &service{
		repos: &repository.Repositories{
			Message: fakeWaitingMessageRepo{messageID: messageID, status: runtimemodel.MessageStatusWaitingClientAction},
		},
	}

	err := svc.ensureConversationAllowsNewTurn(context.Background(), Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
	}, conversation)
	if !errors.Is(err, ErrConversationWaitingAction) {
		t.Fatalf("ensureConversationAllowsNewTurn() error = %v, want ErrConversationWaitingAction", err)
	}
}

func TestCompletePlainUserInputLeafForNewTurnCompletesPlainQuestion(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	messageID := uuid.New()
	conversation := &runtimemodel.Conversation{
		ID:                   uuid.New(),
		OrganizationID:       organizationID,
		AccountID:            accountID,
		RuntimeStatus:        runtimemodel.ConversationRuntimeStatusIdle,
		CurrentLeafMessageID: &messageID,
	}
	var completedAnswer string
	var completedMetadata map[string]interface{}
	svc := &service{
		repos: &repository.Repositories{
			Message: fakeWaitingMessageRepo{
				messageID: messageID,
				status:    runtimemodel.MessageStatusWaitingQuestion,
				answer:    "I need one more detail before continuing.",
				metadata: map[string]interface{}{
					"usage": map[string]interface{}{"total_tokens": 7},
					"user_input_request": map[string]interface{}{
						"request_id": "plain-question",
						"questions": []interface{}{
							map[string]interface{}{"question": "Which Agent should I edit?"},
						},
					},
				},
				onUpdateCompleted: func(_ uuid.UUID, answer string, metadata map[string]interface{}) error {
					completedAnswer = answer
					completedMetadata = metadata
					return nil
				},
			},
		},
	}

	err := svc.completePlainUserInputLeafForNewTurn(context.Background(), Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
	}, conversation, "")
	if err != nil {
		t.Fatalf("completePlainUserInputLeafForNewTurn() error = %v", err)
	}
	if completedAnswer != "I need one more detail before continuing." {
		t.Fatalf("completed answer = %q", completedAnswer)
	}
	if _, ok := completedMetadata["user_input_request"]; ok {
		t.Fatalf("completed metadata still contains user_input_request: %#v", completedMetadata)
	}
	if completedMetadata["usage"] == nil {
		t.Fatalf("completed metadata lost existing usage: %#v", completedMetadata)
	}
}

func TestCompletePlainUserInputLeafForNewTurnSkipsWorkflowQuestion(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	messageID := uuid.New()
	conversation := &runtimemodel.Conversation{
		ID:                   uuid.New(),
		OrganizationID:       organizationID,
		AccountID:            accountID,
		RuntimeStatus:        runtimemodel.ConversationRuntimeStatusIdle,
		CurrentLeafMessageID: &messageID,
	}
	completed := false
	svc := &service{
		repos: &repository.Repositories{
			Message: fakeWaitingMessageRepo{
				messageID: messageID,
				status:    runtimemodel.MessageStatusWaitingQuestion,
				metadata: map[string]interface{}{
					"user_input_request": map[string]interface{}{
						"source":     "agent_workflow_question_answer",
						"request_id": "workflow-question",
						"questions": []interface{}{
							map[string]interface{}{"question": "Approve the workflow answer?"},
						},
					},
				},
				onUpdateCompleted: func(_ uuid.UUID, _ string, _ map[string]interface{}) error {
					completed = true
					return nil
				},
			},
		},
	}

	err := svc.completePlainUserInputLeafForNewTurn(context.Background(), Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
	}, conversation, "")
	if err != nil {
		t.Fatalf("completePlainUserInputLeafForNewTurn() error = %v", err)
	}
	if completed {
		t.Fatal("workflow question was completed by plain user input path")
	}
}

type fakeWaitingMessageRepo struct {
	messageID         uuid.UUID
	status            string
	answer            string
	metadata          map[string]interface{}
	onUpdateCompleted func(uuid.UUID, string, map[string]interface{}) error
}

func (f fakeWaitingMessageRepo) GetScoped(ctx context.Context, id, organizationID, accountID uuid.UUID) (*runtimemodel.Message, error) {
	_ = ctx
	_ = organizationID
	_ = accountID
	status := f.status
	if status == "" {
		status = runtimemodel.MessageStatusWaitingApproval
	}
	return &runtimemodel.Message{ID: id, Status: status, Answer: f.answer, Metadata: f.metadata}, nil
}

func (f fakeWaitingMessageRepo) Create(context.Context, *runtimemodel.Message) error {
	panic("not implemented")
}

func (f fakeWaitingMessageRepo) GetBySourceMessage(context.Context, uuid.UUID) (*runtimemodel.Message, error) {
	panic("not implemented")
}

func (f fakeWaitingMessageRepo) ListByConversationScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int, int) ([]*runtimemodel.Message, int64, error) {
	panic("not implemented")
}

func (f fakeWaitingMessageRepo) ListByCallerScoped(context.Context, uuid.UUID, uuid.UUID, string, *uuid.UUID, int, int) ([]*runtimemodel.Message, int64, error) {
	panic("not implemented")
}

func (f fakeWaitingMessageRepo) ListByCallerSourceScoped(context.Context, uuid.UUID, uuid.UUID, string, *uuid.UUID, string, int, int) ([]*runtimemodel.Message, int64, error) {
	panic("not implemented")
}

func (f fakeWaitingMessageRepo) ListByCallerLogFilterScoped(context.Context, uuid.UUID, uuid.UUID, string, *uuid.UUID, string, *uuid.UUID, string, int, int) ([]*runtimemodel.Message, int64, error) {
	panic("not implemented")
}

func (f fakeWaitingMessageRepo) ListByCallerRuntimeLogScoped(context.Context, uuid.UUID, *uuid.UUID, uuid.UUID, string, *uuid.UUID, string, *uuid.UUID, string, int, int) ([]*runtimemodel.Message, int64, error) {
	panic("not implemented")
}

func (f fakeWaitingMessageRepo) GetRuntimeLogScoped(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, uuid.UUID, string, *uuid.UUID, string) (*runtimemodel.Message, error) {
	panic("not implemented")
}

func (f fakeWaitingMessageRepo) ListBranch(context.Context, uuid.UUID, int) ([]*runtimemodel.Message, error) {
	panic("not implemented")
}

func (f fakeWaitingMessageRepo) CountByConversation(context.Context, uuid.UUID) (int64, error) {
	panic("not implemented")
}

func (f fakeWaitingMessageRepo) ReplaceRootForStreaming(context.Context, *runtimemodel.Message) error {
	panic("not implemented")
}

func (f fakeWaitingMessageRepo) UpdateCompleted(_ context.Context, id uuid.UUID, answer string, metadata map[string]interface{}) error {
	if f.onUpdateCompleted != nil {
		return f.onUpdateCompleted(id, answer, metadata)
	}
	panic("not implemented")
}

func (f fakeWaitingMessageRepo) UpdateMetadata(context.Context, uuid.UUID, map[string]interface{}) error {
	panic("not implemented")
}

func (f fakeWaitingMessageRepo) UpdateMetadataAnyStatus(context.Context, uuid.UUID, map[string]interface{}) error {
	panic("not implemented")
}

func (f fakeWaitingMessageRepo) UpdateWaitingApproval(context.Context, uuid.UUID, map[string]interface{}) error {
	panic("not implemented")
}

func (f fakeWaitingMessageRepo) UpdateWaitingQuestion(context.Context, uuid.UUID, map[string]interface{}) error {
	panic("not implemented")
}

func (f fakeWaitingMessageRepo) UpdateWaitingClientAction(context.Context, uuid.UUID, map[string]interface{}) error {
	panic("not implemented")
}

func (f fakeWaitingMessageRepo) UpdateError(context.Context, uuid.UUID, string) error {
	panic("not implemented")
}

func (f fakeWaitingMessageRepo) MarkStopped(context.Context, uuid.UUID) error {
	panic("not implemented")
}

func (f fakeWaitingMessageRepo) UpdateStoppedAnswer(context.Context, uuid.UUID, string, map[string]interface{}) error {
	panic("not implemented")
}

func (f fakeWaitingMessageRepo) DeleteSubtreeScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*repository.MessageDeleteResult, error) {
	panic("not implemented")
}

func (f fakeWaitingMessageRepo) ListStaleActiveIDs(context.Context, time.Time) ([]uuid.UUID, error) {
	panic("not implemented")
}

func (f fakeWaitingMessageRepo) MarkStaleActiveAsError(context.Context, time.Time, string) (int64, error) {
	panic("not implemented")
}
