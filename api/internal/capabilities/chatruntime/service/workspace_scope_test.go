package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
)

func TestEnsureConversationWorkspaceScope(t *testing.T) {
	workspaceA := uuid.New()
	workspaceB := uuid.New()
	tests := []struct {
		name                  string
		scopeWorkspace        *uuid.UUID
		conversationWorkspace *uuid.UUID
		wantDenied            bool
	}{
		{name: "organization scope", scopeWorkspace: nil, conversationWorkspace: nil},
		{name: "same workspace", scopeWorkspace: &workspaceA, conversationWorkspace: &workspaceA},
		{name: "different workspaces", scopeWorkspace: &workspaceB, conversationWorkspace: &workspaceA, wantDenied: true},
		{name: "workspace request and organization conversation", scopeWorkspace: &workspaceA, conversationWorkspace: nil, wantDenied: true},
		{name: "organization request and workspace conversation", scopeWorkspace: nil, conversationWorkspace: &workspaceA, wantDenied: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ensureConversationWorkspaceScope(
				Scope{WorkspaceID: test.scopeWorkspace},
				&runtimemodel.Conversation{WorkspaceID: test.conversationWorkspace},
			)
			if test.wantDenied {
				if !errors.Is(err, ErrPermissionDenied) {
					t.Fatalf("error = %v, want permission denied", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v, want nil", err)
			}
		})
	}
}

type workspaceScopeConversationRepository struct {
	repository.ConversationRepository
	conversation *runtimemodel.Conversation
}

func (repo workspaceScopeConversationRepository) GetScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*runtimemodel.Conversation, error) {
	return repo.conversation, nil
}

func (repo workspaceScopeConversationRepository) GetByCallerScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string, *uuid.UUID, string) (*runtimemodel.Conversation, error) {
	return repo.conversation, nil
}

type workspaceScopeMessageRepository struct {
	repository.MessageRepository
	message *runtimemodel.Message
}

func (repo workspaceScopeMessageRepository) GetScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*runtimemodel.Message, error) {
	return repo.message, nil
}

func TestExistingChatAndRegenerationRejectConversationWorkspaceMismatch(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	workspaceA := uuid.New()
	workspaceB := uuid.New()
	conversation := &runtimemodel.Conversation{
		ID: uuid.New(), OrganizationID: organizationID, AccountID: accountID,
		WorkspaceID: &workspaceA,
	}
	message := &runtimemodel.Message{ID: uuid.New(), ConversationID: conversation.ID}
	resolverCalls := 0
	svc := &service{
		repos: &repository.Repositories{
			Conversation: workspaceScopeConversationRepository{conversation: conversation},
			Message:      workspaceScopeMessageRepository{message: message},
		},
		integrationPrefs: AIChatIntegrationPreferenceResolverFunc(func(context.Context, Scope) (AIChatIntegrationRuntimePreferences, error) {
			resolverCalls++
			return AIChatIntegrationRuntimePreferences{}, nil
		}),
	}
	scope := Scope{
		OrganizationID: organizationID, AccountID: accountID,
		WorkspaceID: &workspaceB, SkipAccessCheck: true,
	}
	caller := Caller{Type: runtimemodel.ConversationCallerAIChat}

	_, err := svc.resolveChatConversation(
		context.Background(), scope, caller,
		runtimedto.ChatRequest{ConversationID: conversation.ID.String()},
		&chatRequestParts{},
	)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("existing chat error = %v, want workspace permission denial", err)
	}
	_, err = svc.prepareRootRegeneration(
		context.Background(), scope, caller, RunConfig{}, message.ID,
		runtimedto.RegenerateMessageRequest{}, false,
	)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("regeneration error = %v, want workspace permission denial", err)
	}
	if resolverCalls != 1 {
		t.Fatalf("resolver calls = %d, want authoritative refresh at regeneration boundary", resolverCalls)
	}
}

func TestContinuationPrepareRejectsConversationWorkspaceMismatchBeforePreferenceRefresh(t *testing.T) {
	workspaceA := uuid.New()
	workspaceB := uuid.New()
	conversation := &runtimemodel.Conversation{ID: uuid.New(), WorkspaceID: &workspaceA}
	message := &runtimemodel.Message{ID: uuid.New(), ConversationID: conversation.ID}
	resolverCalls := 0
	svc := &service{integrationPrefs: AIChatIntegrationPreferenceResolverFunc(func(context.Context, Scope) (AIChatIntegrationRuntimePreferences, error) {
		resolverCalls++
		return AIChatIntegrationRuntimePreferences{}, nil
	})}
	scope := Scope{WorkspaceID: &workspaceB}
	caller := Caller{Type: runtimemodel.ConversationCallerAIChat}

	tests := []struct {
		name string
		call func() error
	}{
		{name: "tool governance", call: func() error {
			_, err := svc.prepareToolGovernanceContinuationChat(context.Background(), scope, &ToolGovernanceContinuation{Conversation: conversation, Message: message})
			return err
		}},
		{name: "client action", call: func() error {
			_, err := svc.prepareClientActionContinuationChat(context.Background(), scope, &ClientActionContinuation{Conversation: conversation, Message: message}, runtimedto.ClientActionResultRequest{})
			return err
		}},
		{name: "user input", call: func() error {
			_, err := svc.prepareUserInputContinuationChat(context.Background(), scope, caller, RunConfig{}, &UserInputContinuation{Conversation: conversation, Message: message}, runtimedto.UserInputContinuationRequest{})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, ErrPermissionDenied) {
				t.Fatalf("continuation error = %v, want workspace permission denial", err)
			}
		})
	}
	if resolverCalls != 0 {
		t.Fatalf("resolver calls = %d, want none after workspace mismatch", resolverCalls)
	}
}
