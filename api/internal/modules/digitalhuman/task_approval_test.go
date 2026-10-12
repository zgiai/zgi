package digitalhuman

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	approvalruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/approval"
	"gorm.io/gorm"
)

type channelApprovalChatStub struct {
	ChatRuntime
	messages []*runtimemodel.Message
	err      error
}

func (s channelApprovalChatStub) ListConversationMessagesByCaller(
	context.Context,
	chatruntime.Scope,
	chatruntime.Caller,
	uuid.UUID,
	int,
	int,
) ([]*runtimemodel.Message, int64, error) {
	return s.messages, int64(len(s.messages)), s.err
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

type channelApprovalStoreStub struct {
	RunStore
	run       *Run
	delivered []string
}

func (s *channelApprovalStoreStub) RunByMessageID(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) (*Run, error) {
	return s.run, nil
}

func (s *channelApprovalStoreStub) EnqueueRunChannelFinalDelivery(_ context.Context, _ uuid.UUID, _ string, text string, _ time.Time) error {
	s.delivered = append(s.delivered, text)
	return nil
}

type channelApprovalRunnerStub struct{ calls int }

func (r *channelApprovalRunnerStub) ResumeApprovalWorkflow(context.Context, *approvalruntime.Form) error {
	r.calls++
	return nil
}

func (r *channelApprovalRunnerStub) ResumeQuestionAnswerWorkflow(context.Context, string, map[string]interface{}) error {
	r.calls++
	return nil
}

func TestContinueChannelWorkflowApprovalAssignedToDesignatedApproversAsksUserToWait(t *testing.T) {
	for _, tt := range []struct {
		name  string
		text  string
		state map[string]interface{}
	}{
		{name: "external approvers", text: "确认", state: map[string]interface{}{"ui_approval_allowed": false}},
		{name: "token withheld", text: "确认", state: map[string]interface{}{"ui_approval_allowed": true}},
		{name: "non confirmation text", text: "什么时候能好", state: map[string]interface{}{"ui_approval_allowed": false}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			state := map[string]interface{}{"kind": "workflow", "workflow_run_id": "run-1"}
			for key, value := range tt.state {
				state[key] = value
			}
			store := &channelApprovalStoreStub{run: &Run{ID: uuid.New()}}
			runner := &channelApprovalRunnerStub{}
			service := &RuntimeService{
				chat: channelApprovalChatStub{messages: []*runtimemodel.Message{{
					ID: uuid.New(), Status: runtimemodel.MessageStatusWaitingApproval,
					Metadata: map[string]interface{}{"agent_workflow_continuation": state},
				}}},
				store:          store,
				workflowDB:     &gorm.DB{},
				workflowRunner: runner,
			}

			handled, err := service.ContinueChannelTaskApproval(
				context.Background(),
				Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ActorID: uuid.New()},
				uuid.New(), uuid.New(), tt.text,
			)
			if err != nil || !handled {
				t.Fatalf("ContinueChannelTaskApproval() = %v, %v; want handled", handled, err)
			}
			if len(store.delivered) != 1 || store.delivered[0] != channelWorkflowApprovalWaitingNotice {
				t.Fatalf("delivered = %#v, want the waiting notice", store.delivered)
			}
			if runner.calls != 0 {
				t.Fatalf("workflow resumed %d times, want an approval assigned to others left untouched", runner.calls)
			}
		})
	}
}
