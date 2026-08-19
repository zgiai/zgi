package digitalhuman

import (
	"context"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
)

func TestListConversationsUsesDigitalHumanCallerAndWorkspaceScope(t *testing.T) {
	scope, digitalHumanID := testRunScope(), uuid.New()
	chat := &scopedConversationChat{fakeDigitalHumanChat: &fakeDigitalHumanChat{}}
	chat.list = func(_ context.Context, gotScope chatruntime.Scope, caller chatruntime.Caller, page, limit int) ([]*runtimemodel.Conversation, int64, error) {
		if gotScope.OrganizationID != scope.OrganizationID || gotScope.WorkspaceID == nil || *gotScope.WorkspaceID != scope.WorkspaceID || gotScope.AccountID != scope.ActorID {
			t.Fatalf("runtime scope = %#v, want %#v", gotScope, scope)
		}
		if caller.Type != runtimemodel.ConversationCallerDigitalHuman || caller.ID == nil || *caller.ID != digitalHumanID || caller.Source != digitalHumanConversationSource {
			t.Fatalf("caller = %#v", caller)
		}
		if page != 2 || limit != 15 {
			t.Fatalf("pagination = %d/%d", page, limit)
		}
		return []*runtimemodel.Conversation{{ID: uuid.New()}}, 1, nil
	}

	service := NewRuntimeService(newFakeRunStore(), fakeExecutionResolver{}, fakeBrainAdapter{}, chat, &fakeRunDispatcher{})
	items, total, err := service.ListConversations(context.Background(), scope, digitalHumanID, 2, 15)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || total != 1 {
		t.Fatalf("result = %d items, total %d", len(items), total)
	}
}

type scopedConversationChat struct {
	*fakeDigitalHumanChat
	ConversationRuntime
	list func(context.Context, chatruntime.Scope, chatruntime.Caller, int, int) ([]*runtimemodel.Conversation, int64, error)
}

func (c *scopedConversationChat) ListConversationsByCaller(ctx context.Context, scope chatruntime.Scope, caller chatruntime.Caller, page, limit int) ([]*runtimemodel.Conversation, int64, error) {
	return c.list(ctx, scope, caller, page, limit)
}
