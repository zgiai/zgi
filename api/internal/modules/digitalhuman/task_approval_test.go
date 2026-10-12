package digitalhuman

import (
	"context"
	"encoding/json"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"gorm.io/datatypes"
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
		ChannelApprovalReply{Text: "hello", ConversationType: channelruntime.ConversationTypeDirect},
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
			// Production shape: generic workflow approvals carry workflow and
			// approval fields but no explicit kind.
			state := map[string]interface{}{
				"workflow_run_id": "run-1", "workflow_id": "workflow-1",
				"approval_form": map[string]interface{}{"title": "审批"},
			}
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
				uuid.New(), uuid.New(), ChannelApprovalReply{Text: tt.text, ConversationType: channelruntime.ConversationTypeDirect},
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

func TestContinueChannelTaskApprovalIgnoresWaitingMessageWithoutWorkflowContinuation(t *testing.T) {
	store := &channelApprovalStoreStub{run: &Run{ID: uuid.New()}}
	service := &RuntimeService{
		chat: channelApprovalChatStub{messages: []*runtimemodel.Message{{
			ID: uuid.New(), Status: runtimemodel.MessageStatusWaitingApproval,
			Metadata: map[string]interface{}{"agent_workflow_continuation": map[string]interface{}{"approval_form": map[string]interface{}{}}},
		}}},
		store:          store,
		workflowDB:     &gorm.DB{},
		workflowRunner: &channelApprovalRunnerStub{},
	}
	handled, err := service.ContinueChannelTaskApproval(
		context.Background(),
		Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ActorID: uuid.New()},
		uuid.New(), uuid.New(), ChannelApprovalReply{Text: "确认", ConversationType: channelruntime.ConversationTypeDirect},
	)
	if err != nil || handled || len(store.delivered) != 0 {
		t.Fatalf("handled=%v err=%v delivered=%v, want the message left to normal admission", handled, err, store.delivered)
	}
}

// A group conversation is shared: only the member whose message started the
// waiting run may answer its approval.
func TestChannelApprovalInGroupOnlyAcceptsTheInitiatingSender(t *testing.T) {
	encoded, err := json.Marshal(invocation.Event{
		Origin: invocation.Origin{Kind: invocation.OriginChannel},
		Sender: invocation.PrincipalRef{Type: invocation.PrincipalExternalUser, ID: "ou_initiator"},
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingApproval := func(run *Run) (*RuntimeService, *channelApprovalStoreStub, *channelApprovalRunnerStub) {
		store := &channelApprovalStoreStub{run: run}
		runner := &channelApprovalRunnerStub{}
		return &RuntimeService{
			chat: channelApprovalChatStub{messages: []*runtimemodel.Message{{
				ID: uuid.New(), Status: runtimemodel.MessageStatusWaitingApproval,
				Metadata: map[string]interface{}{"agent_workflow_continuation": map[string]interface{}{
					"workflow_run_id": "run-1", "ui_approval_allowed": false,
				}},
			}}},
			store: store, workflowDB: &gorm.DB{}, workflowRunner: runner,
		}, store, runner
	}
	scope := Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ActorID: uuid.New()}
	for _, tt := range []struct {
		name   string
		run    *Run
		reply  ChannelApprovalReply
		notice string
	}{
		{
			name:   "initiator passes the sender check",
			run:    &Run{ID: uuid.New(), InvocationEvent: datatypes.JSON(encoded)},
			reply:  ChannelApprovalReply{Text: "确认", SenderExternalID: "ou_initiator", ConversationType: channelruntime.ConversationTypeGroup},
			notice: channelWorkflowApprovalWaitingNotice,
		},
		{
			name:   "another member is told to wait",
			run:    &Run{ID: uuid.New(), InvocationEvent: datatypes.JSON(encoded)},
			reply:  ChannelApprovalReply{Text: "确认", SenderExternalID: "ou_other_member", ConversationType: channelruntime.ConversationTypeGroup},
			notice: channelApprovalOtherSenderNotice,
		},
		{
			name:   "group run without a member initiator",
			run:    &Run{ID: uuid.New()},
			reply:  ChannelApprovalReply{Text: "确认", SenderExternalID: "ou_other_member", ConversationType: channelruntime.ConversationTypeGroup},
			notice: channelApprovalUnknownInitiatorNotice,
		},
		{
			name:   "direct run without a member initiator",
			run:    &Run{ID: uuid.New()},
			reply:  ChannelApprovalReply{Text: "确认", SenderExternalID: "ou_member", ConversationType: channelruntime.ConversationTypeDirect},
			notice: channelWorkflowApprovalWaitingNotice,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service, store, runner := pendingApproval(tt.run)
			handled, err := service.ContinueChannelTaskApproval(context.Background(), scope, uuid.New(), uuid.New(), tt.reply)
			if err != nil || !handled {
				t.Fatalf("ContinueChannelTaskApproval() = %v, %v; want handled", handled, err)
			}
			if len(store.delivered) != 1 || store.delivered[0] != tt.notice {
				t.Fatalf("delivered = %#v, want %q", store.delivered, tt.notice)
			}
			if runner.calls != 0 {
				t.Fatalf("workflow resumed %d times", runner.calls)
			}
		})
	}
}
