package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
)

func TestListConversationsByCallerUsesRuntimeSourceScope(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	agentID := uuid.New()
	webAppID := uuid.New()
	repo := &sourceScopedConversationRepo{}
	svc := &service{repos: &repository.Repositories{Conversation: repo}}
	caller := Caller{
		Type:           runtimemodel.ConversationCallerAgent,
		ID:             &agentID,
		Source:         runtimemodel.ConversationSourceWebApp,
		SourceWebAppID: &webAppID,
	}

	if _, _, err := svc.ListConversationsByCaller(
		context.Background(),
		Scope{OrganizationID: organizationID, AccountID: accountID, SkipAccessCheck: true},
		caller,
		1,
		20,
	); err != nil {
		t.Fatalf("ListConversationsByCaller() error = %v", err)
	}
	if repo.source != runtimemodel.ConversationSourceWebApp || repo.sourceWebAppID == nil || *repo.sourceWebAppID != webAppID {
		t.Fatalf("source scope = %q/%v, want webapp/%s", repo.source, repo.sourceWebAppID, webAppID)
	}
	if repo.unscopedListCalled {
		t.Fatal("ListConversationsByCaller() used caller-only history instead of source-scoped history")
	}
}

func TestGetConversationByCallerRejectsRuntimeSourceMismatch(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	agentID := uuid.New()
	conversationID := uuid.New()
	repo := &sourceScopedConversationRepo{conversation: &runtimemodel.Conversation{
		ID:             conversationID,
		OrganizationID: organizationID,
		AccountID:      accountID,
		CallerType:     runtimemodel.ConversationCallerAgent,
		CallerID:       &agentID,
		Source:         runtimemodel.ConversationSourceWebApp,
		SourceWebAppID: uuidPtr(uuid.New()),
	}}
	svc := &service{repos: &repository.Repositories{Conversation: repo}}

	_, err := svc.GetConversationByCaller(
		context.Background(),
		Scope{OrganizationID: organizationID, AccountID: accountID, SkipAccessCheck: true},
		Caller{Type: runtimemodel.ConversationCallerAgent, ID: &agentID, Source: runtimemodel.ConversationSourceConsole},
		conversationID,
	)
	if err != ErrNotFound {
		t.Fatalf("GetConversationByCaller() error = %v, want ErrNotFound", err)
	}
}

type sourceScopedConversationRepo struct {
	repository.ConversationRepository
	conversation       *runtimemodel.Conversation
	source             string
	sourceWebAppID     *uuid.UUID
	unscopedListCalled bool
}

func (r *sourceScopedConversationRepo) GetByCallerScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string, *uuid.UUID, string) (*runtimemodel.Conversation, error) {
	return r.conversation, nil
}

func (r *sourceScopedConversationRepo) ListByCallerScoped(context.Context, uuid.UUID, uuid.UUID, string, *uuid.UUID, string, int, int) ([]*runtimemodel.Conversation, int64, error) {
	r.unscopedListCalled = true
	return nil, 0, nil
}

func (r *sourceScopedConversationRepo) ListByCallerSourceScoped(_ context.Context, _, _ uuid.UUID, _ string, _ *uuid.UUID, _ string, source string, sourceWebAppID *uuid.UUID, _, _ int) ([]*runtimemodel.Conversation, int64, error) {
	r.source = source
	r.sourceWebAppID = normalizeCallerID(sourceWebAppID)
	return nil, 0, nil
}

func uuidPtr(id uuid.UUID) *uuid.UUID {
	return &id
}
