package service

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
)

type externalContinuationFixture struct {
	svc            *service
	mock           sqlmock.Sqlmock
	message        *runtimemodel.Message
	scope          Scope
	caller         Caller
	config         RunConfig
	conversationID uuid.UUID
}

type sourcedConversationRepo struct {
	*callerScopedStreamConversationRepo
	source string
}

func (r sourcedConversationRepo) GetByCallerScoped(ctx context.Context, id, organizationID, accountID uuid.UUID, callerType string, callerID, externalUserID *uuid.UUID, conversationType string) (*runtimemodel.Conversation, error) {
	conversation, err := r.callerScopedStreamConversationRepo.GetByCallerScoped(ctx, id, organizationID, accountID, callerType, callerID, externalUserID, conversationType)
	if conversation != nil {
		conversation.Source = r.source
	}
	return conversation, err
}

// newExternalContinuationFixture is an API conversation whose workflow
// approval belongs to designated approvers (ui_approval_allowed=false).
func newExternalContinuationFixture(t *testing.T, status, kind string) externalContinuationFixture {
	t.Helper()
	digitalHumanID, conversationID := uuid.New(), uuid.New()
	message := &runtimemodel.Message{
		ID: uuid.New(), ConversationID: conversationID, Status: status,
		Metadata: map[string]interface{}{"agent_workflow_continuation": map[string]interface{}{
			"workflow_run_id": "run-1", "kind": kind, "binding_id": "binding-1", "ui_approval_allowed": false,
		}},
	}
	db, mock := newAgentBindingStepTestDB(t)
	return externalContinuationFixture{
		svc: &service{repos: &repository.Repositories{
			Access: callerScopedStreamAccessRepo{},
			Conversation: sourcedConversationRepo{
				callerScopedStreamConversationRepo: &callerScopedStreamConversationRepo{allowedCallerType: runtimemodel.ConversationCallerDigitalHuman, allowedCallerID: digitalHumanID},
				source:                             runtimemodel.ConversationSourceExternalAPI,
			},
			Message: &callerScopedStreamMessageRepo{message: message},
			DB:      db,
		}},
		mock: mock, message: message, conversationID: conversationID,
		scope:  Scope{OrganizationID: uuid.New(), AccountID: uuid.New()},
		caller: Caller{Type: runtimemodel.ConversationCallerDigitalHuman, ID: &digitalHumanID, Source: runtimemodel.ConversationSourceExternalAPI},
		config: RunConfig{WorkflowBindings: []AgentWorkflowBinding{{BindingID: "binding-1"}}},
	}
}

func TestExternalWorkflowContinuationClaimsADesignatedApproverApproval(t *testing.T) {
	f := newExternalContinuationFixture(t, runtimemodel.MessageStatusWaitingApproval, "")
	if _, err := f.svc.BeginWorkflowApprovalContinuation(context.Background(), f.scope, f.caller, f.config, f.conversationID, f.message.ID); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("conversation submit error = %v, want the approval kept with its approvers", err)
	}
	f.mock.ExpectBegin()
	f.mock.ExpectExec(`UPDATE "chat_runtime_conversations"`).WillReturnResult(sqlmock.NewResult(0, 1))
	f.mock.ExpectExec(`UPDATE "chat_runtime_messages"`).WillReturnResult(sqlmock.NewResult(0, 1))
	f.mock.ExpectCommit()
	continuation, err := f.svc.BeginExternalWorkflowApprovalContinuation(context.Background(), f.scope, f.caller, f.config, f.conversationID, f.message.ID)
	if err != nil || continuation == nil || continuation.WorkflowRunID != "run-1" {
		t.Fatalf("BeginExternalWorkflowApprovalContinuation() = %#v, %v", continuation, err)
	}
	if err := f.mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExternalWorkflowContinuationClaimsTheMessageOnce(t *testing.T) {
	f := newExternalContinuationFixture(t, runtimemodel.MessageStatusWaitingApproval, "")
	f.mock.ExpectBegin()
	f.mock.ExpectExec(`UPDATE "chat_runtime_conversations"`).WillReturnResult(sqlmock.NewResult(0, 1))
	f.mock.ExpectExec(`UPDATE "chat_runtime_messages"`).WillReturnResult(sqlmock.NewResult(0, 0))
	f.mock.ExpectRollback()
	if _, err := f.svc.BeginExternalWorkflowApprovalContinuation(context.Background(), f.scope, f.caller, f.config, f.conversationID, f.message.ID); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("second claim error = %v, want ErrInvalidInput", err)
	}
	if err := f.mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExternalWorkflowContinuationOnlyObservesWaitingWorkflowApprovals(t *testing.T) {
	for name, f := range map[string]externalContinuationFixture{
		"question":          newExternalContinuationFixture(t, runtimemodel.MessageStatusWaitingQuestion, ""),
		"streaming":         newExternalContinuationFixture(t, runtimemodel.MessageStatusStreaming, ""),
		"DigitalHuman task": newExternalContinuationFixture(t, runtimemodel.MessageStatusWaitingApproval, "digital_human_task"),
	} {
		if _, err := f.svc.BeginExternalWorkflowApprovalContinuation(context.Background(), f.scope, f.caller, f.config, f.conversationID, f.message.ID); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("%s: error = %v, want ErrInvalidInput", name, err)
		}
		if err := f.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
