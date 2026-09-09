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

func TestListConversationsUsesExternalUserSubjectAndSkipsMembershipForAPI(t *testing.T) {
	scope, digitalHumanID := testRunScope(), uuid.New()
	subjectID := uuid.New()
	scope.ActorID = subjectID
	scope.UserSubjectID = &subjectID
	scope.Surface = SurfaceAPI
	chat := &scopedConversationChat{fakeDigitalHumanChat: &fakeDigitalHumanChat{}}
	chat.list = func(_ context.Context, gotScope chatruntime.Scope, caller chatruntime.Caller, _, _ int) ([]*runtimemodel.Conversation, int64, error) {
		if gotScope.AccountID != subjectID || !gotScope.SkipAccessCheck {
			t.Fatalf("runtime scope = %#v, want external user subject with access already checked", gotScope)
		}
		if caller.Source != digitalHumanAPIConversationSource {
			t.Fatalf("caller source = %q, want %q", caller.Source, digitalHumanAPIConversationSource)
		}
		return []*runtimemodel.Conversation{}, 0, nil
	}
	service := NewRuntimeService(newFakeRunStore(), fakeExecutionResolver{}, fakeBrainAdapter{}, chat, &fakeRunDispatcher{})
	if _, _, err := service.ListConversations(context.Background(), scope, digitalHumanID, 1, 20); err != nil {
		t.Fatal(err)
	}
}

func TestListConversationsCarriesWebAppIdentity(t *testing.T) {
	scope, digitalHumanID := testRunScope(), uuid.New()
	webAppID := uuid.New()
	scope.Surface = SurfaceWebApp
	scope.WebAppID = &webAppID
	chat := &scopedConversationChat{fakeDigitalHumanChat: &fakeDigitalHumanChat{}}
	chat.list = func(_ context.Context, gotScope chatruntime.Scope, caller chatruntime.Caller, _, _ int) ([]*runtimemodel.Conversation, int64, error) {
		if !gotScope.SkipAccessCheck || caller.Source != digitalHumanWebAppConversationSource || caller.SourceWebAppID == nil || *caller.SourceWebAppID != webAppID {
			t.Fatalf("webapp runtime context = %#v / %#v", gotScope, caller)
		}
		return []*runtimemodel.Conversation{}, 0, nil
	}
	service := NewRuntimeService(newFakeRunStore(), fakeExecutionResolver{}, fakeBrainAdapter{}, chat, &fakeRunDispatcher{})
	if _, _, err := service.ListConversations(context.Background(), scope, digitalHumanID, 1, 20); err != nil {
		t.Fatal(err)
	}
}

type scopedConversationChat struct {
	*fakeDigitalHumanChat
	ConversationRuntime
	list func(context.Context, chatruntime.Scope, chatruntime.Caller, int, int) ([]*runtimemodel.Conversation, int64, error)
}

// Resolve the two methods implemented by both embedded test doubles so this
// composite continues to satisfy ConversationRuntime as that surface grows.
func (c *scopedConversationChat) GetConversationByCaller(ctx context.Context, scope chatruntime.Scope, caller chatruntime.Caller, conversationID uuid.UUID) (*runtimemodel.Conversation, error) {
	return c.fakeDigitalHumanChat.GetConversationByCaller(ctx, scope, caller, conversationID)
}

func (c *scopedConversationChat) SubmitRuntimeInput(ctx context.Context, scope chatruntime.Scope, caller chatruntime.Caller, config chatruntime.RunConfig, request chatruntime.SubmitRuntimeInputRequest) (*chatruntime.SubmitRuntimeInputResult, error) {
	return c.fakeDigitalHumanChat.SubmitRuntimeInput(ctx, scope, caller, config, request)
}

func (c *scopedConversationChat) ListConversationsByCaller(ctx context.Context, scope chatruntime.Scope, caller chatruntime.Caller, page, limit int) ([]*runtimemodel.Conversation, int64, error) {
	return c.list(ctx, scope, caller, page, limit)
}
