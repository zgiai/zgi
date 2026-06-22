package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
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

type surfaceAccessRepo struct {
	repository.AccessRepository
}

func (surfaceAccessRepo) IsOrganizationMember(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}

type capturingConversationRepo struct {
	repository.ConversationRepository
	created     *runtimemodel.Conversation
	listSurface string
}

func (r *capturingConversationRepo) Create(_ context.Context, conversation *runtimemodel.Conversation) error {
	r.created = conversation
	return nil
}

func (r *capturingConversationRepo) ListByCallerSurfaceScoped(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ string, _ *uuid.UUID, surface string, _ int, _ int) ([]*runtimemodel.Conversation, int64, error) {
	r.listSurface = surface
	return nil, 0, nil
}
