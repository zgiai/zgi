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
			Message: fakeWaitingApprovalMessageRepo{messageID: messageID},
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

type fakeWaitingApprovalMessageRepo struct {
	messageID uuid.UUID
}

func (f fakeWaitingApprovalMessageRepo) GetScoped(ctx context.Context, id, organizationID, accountID uuid.UUID) (*runtimemodel.Message, error) {
	_ = ctx
	_ = organizationID
	_ = accountID
	return &runtimemodel.Message{ID: id, Status: runtimemodel.MessageStatusWaitingApproval}, nil
}

func (f fakeWaitingApprovalMessageRepo) Create(context.Context, *runtimemodel.Message) error {
	panic("not implemented")
}

func (f fakeWaitingApprovalMessageRepo) GetBySourceMessage(context.Context, uuid.UUID) (*runtimemodel.Message, error) {
	panic("not implemented")
}

func (f fakeWaitingApprovalMessageRepo) ListByConversationScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int, int) ([]*runtimemodel.Message, int64, error) {
	panic("not implemented")
}

func (f fakeWaitingApprovalMessageRepo) ListByCallerScoped(context.Context, uuid.UUID, uuid.UUID, string, *uuid.UUID, int, int) ([]*runtimemodel.Message, int64, error) {
	panic("not implemented")
}

func (f fakeWaitingApprovalMessageRepo) ListByCallerSourceScoped(context.Context, uuid.UUID, uuid.UUID, string, *uuid.UUID, string, int, int) ([]*runtimemodel.Message, int64, error) {
	panic("not implemented")
}

func (f fakeWaitingApprovalMessageRepo) ListByCallerLogFilterScoped(context.Context, uuid.UUID, uuid.UUID, string, *uuid.UUID, string, *uuid.UUID, string, int, int) ([]*runtimemodel.Message, int64, error) {
	panic("not implemented")
}

func (f fakeWaitingApprovalMessageRepo) ListBranch(context.Context, uuid.UUID, int) ([]*runtimemodel.Message, error) {
	panic("not implemented")
}

func (f fakeWaitingApprovalMessageRepo) CountByConversation(context.Context, uuid.UUID) (int64, error) {
	panic("not implemented")
}

func (f fakeWaitingApprovalMessageRepo) ReplaceRootForStreaming(context.Context, *runtimemodel.Message) error {
	panic("not implemented")
}

func (f fakeWaitingApprovalMessageRepo) UpdateCompleted(context.Context, uuid.UUID, string, map[string]interface{}) error {
	panic("not implemented")
}

func (f fakeWaitingApprovalMessageRepo) UpdateMetadata(context.Context, uuid.UUID, map[string]interface{}) error {
	panic("not implemented")
}

func (f fakeWaitingApprovalMessageRepo) UpdateWaitingApproval(context.Context, uuid.UUID, map[string]interface{}) error {
	panic("not implemented")
}

func (f fakeWaitingApprovalMessageRepo) UpdateWaitingQuestion(context.Context, uuid.UUID, map[string]interface{}) error {
	panic("not implemented")
}

func (f fakeWaitingApprovalMessageRepo) UpdateError(context.Context, uuid.UUID, string) error {
	panic("not implemented")
}

func (f fakeWaitingApprovalMessageRepo) MarkStopped(context.Context, uuid.UUID) error {
	panic("not implemented")
}

func (f fakeWaitingApprovalMessageRepo) UpdateStoppedAnswer(context.Context, uuid.UUID, string, map[string]interface{}) error {
	panic("not implemented")
}

func (f fakeWaitingApprovalMessageRepo) DeleteSubtreeScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*repository.MessageDeleteResult, error) {
	panic("not implemented")
}

func (f fakeWaitingApprovalMessageRepo) ListStaleActiveIDs(context.Context, time.Time) ([]uuid.UUID, error) {
	panic("not implemented")
}

func (f fakeWaitingApprovalMessageRepo) MarkStaleActiveAsError(context.Context, time.Time, string) (int64, error) {
	panic("not implemented")
}
