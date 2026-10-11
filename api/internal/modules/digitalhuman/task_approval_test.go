package digitalhuman

import (
	"context"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
)

type channelApprovalChatStub struct {
	ChatRuntime
	err error
}

func (s channelApprovalChatStub) ListConversationMessagesByCaller(
	context.Context,
	chatruntime.Scope,
	chatruntime.Caller,
	uuid.UUID,
	int,
	int,
) ([]*runtimemodel.Message, int64, error) {
	return nil, 0, s.err
}

func TestContinueChannelTaskApprovalTreatsMissingConversationAsNoPendingApproval(t *testing.T) {
	digitalHumanID := uuid.New()
	service := &RuntimeService{
		chat: channelApprovalChatStub{err: chatruntime.ErrNotFound},
	}

	handled, err := service.ContinueChannelTaskApproval(
		context.Background(),
		Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ActorID: uuid.New()},
		digitalHumanID,
		uuid.New(),
		"hello",
	)
	if err != nil {
		t.Fatalf("ContinueChannelTaskApproval() error = %v, want nil", err)
	}
	if handled {
		t.Fatal("ContinueChannelTaskApproval() handled missing conversation, want false")
	}
}
