package digitalhuman

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
)

func TestChannelReportReviewActionReadsOnlyExplicitReplies(t *testing.T) {
	cases := []struct {
		text     string
		action   TaskReportReviewAction
		feedback string
		ok       bool
	}{
		{"采纳", TaskReportAccept, "", true},
		{"通过。", TaskReportAccept, "", true},
		{"返工：补充北京的数据", TaskReportRework, "补充北京的数据", true},
		{"返工: add Beijing", TaskReportRework, "add Beijing", true},
		{"返工", TaskReportRework, "", true},
		{"我觉得可以采纳", "", "", false},
		{"今天天气怎么样", "", "", false},
	}
	for _, tc := range cases {
		action, feedback, ok := channelReportReviewAction(tc.text)
		if action != tc.action || feedback != tc.feedback || ok != tc.ok {
			t.Fatalf("%q = (%q, %q, %v), want (%q, %q, %v)", tc.text, action, feedback, ok, tc.action, tc.feedback, tc.ok)
		}
	}
}

type channelReportServiceStub struct {
	TaskHandlerService
	reports  []*TaskReport
	listed   TaskAccessContext
	reviewed *ReviewTaskReportRequest
}

func (s *channelReportServiceStub) SubmitTaskReport(context.Context, TaskAccessContext, SubmitTaskReportRequest) (*TaskReport, error) {
	return nil, nil
}

func (s *channelReportServiceStub) GetTaskReport(context.Context, TaskAccessContext, uuid.UUID) (*TaskReport, error) {
	return nil, nil
}

func (s *channelReportServiceStub) ListTaskReports(_ context.Context, access TaskAccessContext, _ TaskReportListQuery) (*TaskReportList, error) {
	s.listed = access
	return &TaskReportList{Items: s.reports}, nil
}

func (s *channelReportServiceStub) ReviewTaskReport(_ context.Context, _ TaskAccessContext, _ uuid.UUID, req ReviewTaskReportRequest, _ TaskMutationContext) (*TaskReport, error) {
	s.reviewed = &req
	return s.reports[0], nil
}

func TestChannelReplyReviewsTheConversationsLatestReport(t *testing.T) {
	conversationID, runID := uuid.New(), uuid.New()
	scope := Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ActorID: uuid.New()}
	newService := func(reports ...*TaskReport) (*RuntimeService, *channelReportServiceStub, *channelApprovalStoreStub) {
		tasks := &channelReportServiceStub{reports: reports}
		store := &channelApprovalStoreStub{}
		return &RuntimeService{taskService: tasks, store: store, now: time.Now}, tasks, store
	}
	reply := func(text string) ChannelApprovalReply {
		return ChannelApprovalReply{Text: text, SenderExternalID: "ou_1", ConversationType: channelruntime.ConversationTypeGroup}
	}
	report := &TaskReport{ID: uuid.New(), TaskName: "晨报", DigitalHumanRunID: &runID, ReviewState: TaskReportNew}

	service, tasks, store := newService(report)
	handled, err := service.ReviewChannelTaskReport(context.Background(), scope, uuid.New(), conversationID, reply("采纳"))
	if err != nil || !handled || tasks.reviewed == nil || tasks.reviewed.Action != TaskReportAccept {
		t.Fatalf("accept: handled=%v err=%v reviewed=%+v", handled, err, tasks.reviewed)
	}
	if tasks.listed.Owner.Kind != TaskOwnerConversation || tasks.listed.Owner.ConversationID != conversationID {
		t.Fatalf("reports were read as %+v, want the conversation's own", tasks.listed.Owner)
	}
	if len(store.delivered) != 1 || !strings.Contains(store.delivered[0], "已采纳「晨报」") {
		t.Fatalf("accept delivered = %v", store.delivered)
	}

	service, tasks, store = newService(report)
	if handled, err := service.ReviewChannelTaskReport(context.Background(), scope, uuid.New(), conversationID, reply("返工：补充北京的数据")); err != nil || !handled ||
		tasks.reviewed == nil || tasks.reviewed.Action != TaskReportRework || tasks.reviewed.Feedback != "补充北京的数据" {
		t.Fatalf("rework: handled=%v err=%v reviewed=%+v", handled, err, tasks.reviewed)
	}

	service, tasks, store = newService(report)
	if handled, err := service.ReviewChannelTaskReport(context.Background(), scope, uuid.New(), conversationID, reply("返工")); err != nil || !handled || tasks.reviewed != nil ||
		len(store.delivered) != 1 || store.delivered[0] != channelReportReworkAskHint {
		t.Fatalf("rework without feedback: handled=%v err=%v reviewed=%+v delivered=%v", handled, err, tasks.reviewed, store.delivered)
	}

	// Without a report to review, 采纳 is an ordinary message.
	service, tasks, _ = newService()
	if handled, err := service.ReviewChannelTaskReport(context.Background(), scope, uuid.New(), conversationID, reply("采纳")); err != nil || handled || tasks.reviewed != nil {
		t.Fatalf("no report: handled=%v err=%v", handled, err)
	}
	service, tasks, _ = newService(report)
	if handled, _ := service.ReviewChannelTaskReport(context.Background(), scope, uuid.New(), conversationID, reply("明天几点开会")); handled || tasks.reviewed != nil {
		t.Fatalf("ordinary message was taken as a review")
	}
}

func TestChannelTaskResultSaysHowToReviewIt(t *testing.T) {
	encoded, err := json.Marshal(invocation.Event{
		Origin:  invocation.Origin{Kind: invocation.OriginSchedule},
		Context: map[string]any{"task_name": "晨报", "task_owner_kind": string(TaskOwnerConversation)},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := decorateScheduledTaskResult(&Run{InvocationEvent: encoded}, "今天三件事。")
	if !strings.Contains(got, "任务“晨报”执行完成") || !strings.HasSuffix(got, channelReportReviewHint) {
		t.Fatalf("decorated = %q", got)
	}
}
