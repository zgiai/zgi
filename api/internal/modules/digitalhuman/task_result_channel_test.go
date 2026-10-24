package digitalhuman

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
)

type channelRouteRunStore struct {
	RunStore
	route *ChannelRoute
	err   error
}

func (s channelRouteRunStore) ResolveChannelRoute(context.Context, uuid.UUID, string) (*ChannelRoute, error) {
	return s.route, s.err
}

type completedMessageRecorder struct {
	ChatRuntime
	scope   chatruntime.Scope
	request *chatruntime.CreateCompletedMessageRequest
}

func (r *completedMessageRecorder) CreateCompletedMessageIfAbsent(_ context.Context, scope chatruntime.Scope, request chatruntime.CreateCompletedMessageRequest) (*runtimemodel.Message, error) {
	r.scope, r.request = scope, &request
	return &runtimemodel.Message{}, nil
}

func TestChannelTaskReportJoinsItsConversationHistory(t *testing.T) {
	org, workspace, owner, local := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	installation := &channelruntime.Installation{ID: uuid.New(), OrganizationID: org, WorkspaceID: workspace, CreatedBy: owner}
	request := TaskResultDeliveryRequest{
		OrganizationID: org, WorkspaceID: workspace, DigitalHumanID: uuid.New(), TaskID: uuid.New(), TaskRunID: uuid.New(), TaskName: "晨报",
		ReplyTo: &invocation.ReplyAddress{Provider: "dingtalk", InstallationID: installation.ID.String(), ConversationID: "group:cid-1"},
		Report:  &TaskReportDelivery{ID: uuid.New(), Outcome: TaskReportDone, Summary: "已汇总"},
	}
	chat := &completedMessageRecorder{}
	service := &RuntimeService{chat: chat, store: channelRouteRunStore{route: &ChannelRoute{
		Installation: installation, Mapping: &channelruntime.ConversationMapping{LocalConversationID: &local},
	}, err: ErrChannelBindingNotFound}}
	if err := service.DeliverTaskResult(t.Context(), request); err != nil {
		t.Fatalf("DeliverTaskResult() = %v", err)
	}
	if chat.request == nil || chat.request.ConversationID != local || chat.scope.AccountID != owner || chat.request.Query != "任务汇报：晨报" {
		t.Fatalf("written message = %#v in %#v", chat.request, chat.scope)
	}

	// A channel conversation that has no local one yet has nothing to join.
	chat.request = nil
	service.store = channelRouteRunStore{route: &ChannelRoute{Installation: installation}, err: ErrChannelBindingNotFound}
	if err := service.DeliverTaskResult(t.Context(), request); err != nil || chat.request != nil {
		t.Fatalf("without a local conversation: %v, wrote %#v", err, chat.request)
	}
	// Another workspace's installation is refused.
	service.store = channelRouteRunStore{route: &ChannelRoute{
		Installation: &channelruntime.Installation{ID: installation.ID, OrganizationID: org, WorkspaceID: uuid.New(), CreatedBy: owner},
		Mapping:      &channelruntime.ConversationMapping{LocalConversationID: &local},
	}}
	if err := service.DeliverTaskResult(t.Context(), request); err == nil {
		t.Fatal("an installation of another workspace was accepted")
	}
}
