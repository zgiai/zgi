package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"gorm.io/gorm"
)

func TestStreamConversationEventsForCallerRejectsOtherCallerBeforeMessageLookup(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	agentID := uuid.New()
	otherAgentID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	messageRepo := &callerScopedStreamMessageRepo{}
	conversationRepo := &callerScopedStreamConversationRepo{
		allowedCallerType: runtimemodel.ConversationCallerAgent,
		allowedCallerID:   agentID,
	}
	svc := &service{
		repos: &repository.Repositories{
			Access:       callerScopedStreamAccessRepo{},
			Conversation: conversationRepo,
			Message:      messageRepo,
		},
	}

	err := svc.StreamConversationEventsForCaller(
		context.Background(),
		Scope{OrganizationID: organizationID, AccountID: accountID},
		Caller{Type: runtimemodel.ConversationCallerAgent, ID: &otherAgentID},
		conversationID,
		messageID,
		"",
		func(StreamEvent) error { return nil },
	)

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("StreamConversationEventsForCaller() error = %v, want ErrNotFound", err)
	}
	if !conversationRepo.getByCallerScopedCalled {
		t.Fatalf("conversation caller-scoped lookup was not called")
	}
	if messageRepo.getScopedCalled {
		t.Fatalf("message lookup should not run after caller-scoped conversation denial")
	}
}

func TestBeginWorkflowApprovalContinuationRejectsOtherCallerBeforeMessageLookup(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	agentID := uuid.New()
	otherAgentID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	messageRepo := &callerScopedStreamMessageRepo{}
	conversationRepo := &callerScopedStreamConversationRepo{
		allowedCallerType: runtimemodel.ConversationCallerAgent,
		allowedCallerID:   agentID,
	}
	svc := &service{
		repos: &repository.Repositories{
			Access:       callerScopedStreamAccessRepo{},
			Conversation: conversationRepo,
			Message:      messageRepo,
		},
	}

	_, err := svc.BeginWorkflowApprovalContinuation(
		context.Background(),
		Scope{OrganizationID: organizationID, AccountID: accountID},
		Caller{Type: runtimemodel.ConversationCallerAgent, ID: &otherAgentID},
		RunConfig{},
		conversationID,
		messageID,
	)

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("BeginWorkflowApprovalContinuation() error = %v, want ErrNotFound", err)
	}
	if !conversationRepo.getByCallerScopedCalled {
		t.Fatalf("conversation caller-scoped lookup was not called")
	}
	if messageRepo.getScopedCalled {
		t.Fatalf("message lookup should not run after caller-scoped conversation denial")
	}
}

func TestBeginWorkflowApprovalContinuationRejectsUnavailableBindingBeforeStreaming(t *testing.T) {
	for _, status := range []string{
		runtimemodel.MessageStatusWaitingApproval,
		runtimemodel.MessageStatusWaitingQuestion,
	} {
		t.Run(status, func(t *testing.T) {
			organizationID := uuid.New()
			accountID := uuid.New()
			agentID := uuid.New()
			conversationID := uuid.New()
			messageID := uuid.New()
			targetAgentID := uuid.NewString()
			message := &runtimemodel.Message{
				ID:             messageID,
				ConversationID: conversationID,
				Status:         status,
				Metadata: map[string]interface{}{
					"agent_workflow_continuation": map[string]interface{}{
						"workflow_run_id": "run-1",
						"binding_id":      "removed-binding",
						"agent_id":        targetAgentID,
					},
				},
			}
			messageRepo := &callerScopedStreamMessageRepo{message: message}
			conversationRepo := &callerScopedStreamConversationRepo{
				allowedCallerType: runtimemodel.ConversationCallerAgent,
				allowedCallerID:   agentID,
			}
			svc := &service{repos: &repository.Repositories{
				Access:       callerScopedStreamAccessRepo{},
				Conversation: conversationRepo,
				Message:      messageRepo,
			}}

			_, err := svc.BeginWorkflowApprovalContinuation(
				context.Background(),
				Scope{OrganizationID: organizationID, AccountID: accountID},
				Caller{Type: runtimemodel.ConversationCallerAgent, ID: &agentID},
				RunConfig{WorkflowBindings: []AgentWorkflowBinding{{
					BindingID: "another-binding",
					AgentID:   targetAgentID,
				}}},
				conversationID,
				messageID,
			)

			if !errors.Is(err, ErrWorkflowBindingUnavailable) {
				t.Fatalf("BeginWorkflowApprovalContinuation() error = %v, want ErrWorkflowBindingUnavailable", err)
			}
			if message.Status != status {
				t.Fatalf("message status = %q, want waiting status %q unchanged", message.Status, status)
			}
		})
	}
}

func TestListConversationMessagesByCallerRejectsOtherCallerBeforeMessageList(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	agentID := uuid.New()
	otherAgentID := uuid.New()
	conversationID := uuid.New()
	messageRepo := &callerScopedStreamMessageRepo{}
	conversationRepo := &callerScopedStreamConversationRepo{
		allowedCallerType: runtimemodel.ConversationCallerAgent,
		allowedCallerID:   agentID,
	}
	svc := &service{
		repos: &repository.Repositories{
			Access:       callerScopedStreamAccessRepo{},
			Conversation: conversationRepo,
			Message:      messageRepo,
		},
	}

	_, _, err := svc.ListConversationMessagesByCaller(
		context.Background(),
		Scope{OrganizationID: organizationID, AccountID: accountID},
		Caller{Type: runtimemodel.ConversationCallerAgent, ID: &otherAgentID},
		conversationID,
		1,
		20,
	)

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("ListConversationMessagesByCaller() error = %v, want ErrNotFound", err)
	}
	if !conversationRepo.getByCallerScopedCalled {
		t.Fatalf("conversation caller-scoped lookup was not called")
	}
	if messageRepo.listByConversationScopedCalled {
		t.Fatalf("message list should not run after caller-scoped conversation denial")
	}
}

func TestUpdateConversationByCallerRejectsOtherCallerBeforeUpdate(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	agentID := uuid.New()
	otherAgentID := uuid.New()
	conversationID := uuid.New()
	conversationRepo := &callerScopedStreamConversationRepo{
		allowedCallerType: runtimemodel.ConversationCallerAgent,
		allowedCallerID:   agentID,
	}
	svc := &service{
		repos: &repository.Repositories{
			Access:       callerScopedStreamAccessRepo{},
			Conversation: conversationRepo,
			Message:      &callerScopedStreamMessageRepo{},
		},
	}
	title := "renamed"

	_, err := svc.UpdateConversationByCaller(
		context.Background(),
		Scope{OrganizationID: organizationID, AccountID: accountID},
		Caller{Type: runtimemodel.ConversationCallerAgent, ID: &otherAgentID},
		conversationID,
		runtimedto.UpdateConversationRequest{Title: &title},
	)

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateConversationByCaller() error = %v, want ErrNotFound", err)
	}
	if !conversationRepo.getByCallerScopedCalled {
		t.Fatalf("conversation caller-scoped lookup was not called")
	}
	if conversationRepo.updateScopedCalled {
		t.Fatalf("conversation update should not run after caller-scoped denial")
	}
}

func TestUpdateConversationByCallerReturnsThroughCallerScope(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	agentID := uuid.New()
	conversationID := uuid.New()
	conversationRepo := &callerScopedStreamConversationRepo{
		allowedCallerType: runtimemodel.ConversationCallerAgent,
		allowedCallerID:   agentID,
	}
	svc := &service{
		repos: &repository.Repositories{
			Access:       callerScopedStreamAccessRepo{},
			Conversation: conversationRepo,
			Message:      &callerScopedStreamMessageRepo{},
		},
	}
	title := "renamed"

	_, err := svc.UpdateConversationByCaller(
		context.Background(),
		Scope{OrganizationID: organizationID, AccountID: accountID},
		Caller{Type: runtimemodel.ConversationCallerAgent, ID: &agentID},
		conversationID,
		runtimedto.UpdateConversationRequest{Title: &title},
	)

	if err != nil {
		t.Fatalf("UpdateConversationByCaller() error = %v", err)
	}
	if !conversationRepo.updateScopedCalled {
		t.Fatal("conversation update was not called")
	}
	if conversationRepo.getScopedCalled {
		t.Fatal("caller-scoped update should not fall back to an untyped conversation lookup")
	}
}

func TestDeleteConversationByCallerRejectsOtherCallerBeforeDelete(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	agentID := uuid.New()
	otherAgentID := uuid.New()
	conversationID := uuid.New()
	conversationRepo := &callerScopedStreamConversationRepo{
		allowedCallerType: runtimemodel.ConversationCallerAgent,
		allowedCallerID:   agentID,
	}
	svc := &service{
		repos: &repository.Repositories{
			Access:       callerScopedStreamAccessRepo{},
			Conversation: conversationRepo,
		},
	}

	err := svc.DeleteConversationByCaller(
		context.Background(),
		Scope{OrganizationID: organizationID, AccountID: accountID},
		Caller{Type: runtimemodel.ConversationCallerAgent, ID: &otherAgentID},
		conversationID,
	)

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteConversationByCaller() error = %v, want ErrNotFound", err)
	}
	if !conversationRepo.getByCallerScopedCalled {
		t.Fatalf("conversation caller-scoped lookup was not called")
	}
	if conversationRepo.deleteScopedCalled {
		t.Fatalf("conversation delete should not run after caller-scoped denial")
	}
}

func TestStopConversationByCallerRejectsOtherCallerBeforeStop(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	agentID := uuid.New()
	otherAgentID := uuid.New()
	conversationID := uuid.New()
	conversationRepo := &callerScopedStreamConversationRepo{
		allowedCallerType: runtimemodel.ConversationCallerAgent,
		allowedCallerID:   agentID,
	}
	svc := &service{
		repos: &repository.Repositories{
			Access:       callerScopedStreamAccessRepo{},
			Conversation: conversationRepo,
			Message:      &callerScopedStreamMessageRepo{},
		},
	}

	_, err := svc.StopConversationByCaller(
		context.Background(),
		Scope{OrganizationID: organizationID, AccountID: accountID},
		Caller{Type: runtimemodel.ConversationCallerAgent, ID: &otherAgentID},
		conversationID,
	)

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("StopConversationByCaller() error = %v, want ErrNotFound", err)
	}
	if !conversationRepo.getByCallerScopedCalled {
		t.Fatalf("conversation caller-scoped lookup was not called")
	}
	if conversationRepo.getScopedCalled {
		t.Fatalf("conversation stop should not load bare conversation after caller-scoped denial")
	}
}

func TestPrepareConfiguredRootRegenerationRejectsOtherCallerBeforeReplacement(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	agentID := uuid.New()
	otherAgentID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	messageRepo := &callerScopedStreamMessageRepo{
		message: &runtimemodel.Message{
			ID:             messageID,
			ConversationID: conversationID,
			Query:          "hello",
		},
	}
	conversationRepo := &callerScopedStreamConversationRepo{
		allowedCallerType: runtimemodel.ConversationCallerAgent,
		allowedCallerID:   agentID,
	}
	svc := &service{
		repos: &repository.Repositories{
			Access:       callerScopedStreamAccessRepo{},
			Conversation: conversationRepo,
			Message:      messageRepo,
		},
	}

	_, err := svc.PrepareConfiguredRootRegeneration(
		context.Background(),
		Scope{OrganizationID: organizationID, AccountID: accountID},
		Caller{Type: runtimemodel.ConversationCallerAgent, ID: &otherAgentID},
		RunConfig{},
		messageID,
		runtimedto.RegenerateMessageRequest{},
	)

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("PrepareConfiguredRootRegeneration() error = %v, want ErrNotFound", err)
	}
	if !conversationRepo.getByCallerScopedCalled {
		t.Fatalf("conversation caller-scoped lookup was not called")
	}
	if !messageRepo.getScopedCalled {
		t.Fatalf("message ownership lookup was not called before caller-scoped conversation denial")
	}
	if messageRepo.countByConversationCalled {
		t.Fatalf("message replacement should not start after caller-scoped conversation denial")
	}
	if messageRepo.replaceRootForStreamingCalled {
		t.Fatalf("root replacement should not run after caller-scoped conversation denial")
	}
}

type callerScopedStreamAccessRepo struct {
	repository.AccessRepository
}

func (callerScopedStreamAccessRepo) IsOrganizationMember(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}

type callerScopedStreamConversationRepo struct {
	repository.ConversationRepository

	allowedCallerType       string
	allowedCallerID         uuid.UUID
	getByCallerScopedCalled bool
	getScopedCalled         bool
	updateScopedCalled      bool
	deleteScopedCalled      bool
}

func (r *callerScopedStreamConversationRepo) GetByCallerScoped(_ context.Context, id, organizationID, accountID uuid.UUID, callerType string, callerID *uuid.UUID, _ string) (*runtimemodel.Conversation, error) {
	r.getByCallerScopedCalled = true
	if callerType != r.allowedCallerType || callerID == nil || *callerID != r.allowedCallerID {
		return nil, gorm.ErrRecordNotFound
	}
	return &runtimemodel.Conversation{
		ID:             id,
		OrganizationID: organizationID,
		AccountID:      accountID,
		CallerType:     callerType,
		CallerID:       callerID,
	}, nil
}

func (r *callerScopedStreamConversationRepo) GetScoped(_ context.Context, id, organizationID, accountID uuid.UUID) (*runtimemodel.Conversation, error) {
	r.getScopedCalled = true
	return &runtimemodel.Conversation{
		ID:             id,
		OrganizationID: organizationID,
		AccountID:      accountID,
	}, nil
}

func (r *callerScopedStreamConversationRepo) UpdateScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, map[string]interface{}) error {
	r.updateScopedCalled = true
	return nil
}

func (r *callerScopedStreamConversationRepo) DeleteScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	r.deleteScopedCalled = true
	return nil
}

type callerScopedStreamMessageRepo struct {
	repository.MessageRepository

	message                        *runtimemodel.Message
	getScopedCalled                bool
	listByConversationScopedCalled bool
	countByConversationCalled      bool
	replaceRootForStreamingCalled  bool
}

func (r *callerScopedStreamMessageRepo) GetScoped(_ context.Context, id, organizationID, accountID uuid.UUID) (*runtimemodel.Message, error) {
	_ = organizationID
	_ = accountID
	r.getScopedCalled = true
	if r.message != nil {
		message := *r.message
		message.ID = id
		return &message, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *callerScopedStreamMessageRepo) ListByConversationScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int, int) ([]*runtimemodel.Message, int64, error) {
	r.listByConversationScopedCalled = true
	return nil, 0, nil
}

func (r *callerScopedStreamMessageRepo) CountByConversation(context.Context, uuid.UUID) (int64, error) {
	r.countByConversationCalled = true
	return 0, nil
}

func (r *callerScopedStreamMessageRepo) ReplaceRootForStreaming(context.Context, *runtimemodel.Message) error {
	r.replaceRootForStreamingCalled = true
	return nil
}
