package digitalhuman

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"gorm.io/datatypes"
)

func TestChannelWaitingPromptSaysHowToAnswer(t *testing.T) {
	receipt := uuid.New()
	cases := []struct {
		name, status, answer string
		run                  *Run
		metadata             map[string]interface{}
		want                 []string
	}{
		{"task approval", runtimemodel.MessageStatusWaitingApproval, "我准备创建任务「喝水提醒」。", nil,
			map[string]interface{}{"agent_workflow_continuation": map[string]interface{}{"kind": "digital_human_task", "task_name": "喝水提醒"}},
			[]string{"喝水提醒", "回复「确认」执行"}},
		{"connector", runtimemodel.MessageStatusWaitingApproval, "", &Run{WaitingReceipt: &receipt}, map[string]interface{}{},
			[]string{"外部操作", "回复「确认」执行这个操作"}},
		{"question", runtimemodel.MessageStatusWaitingQuestion, "还差两个信息。", nil,
			map[string]interface{}{"user_input_request": map[string]interface{}{"questions": []interface{}{
				map[string]interface{}{"id": "a", "question": "几点提醒？"}, map[string]interface{}{"id": "b", "question": "提醒谁？"},
			}}},
			[]string{"还差两个信息", "1. 几点提醒？", "2. 提醒谁？", "分行依次回答"}},
	}
	for _, tc := range cases {
		text := channelWaitingPromptText(tc.run, tc.status, tc.answer, tc.metadata)
		for _, want := range tc.want {
			if !strings.Contains(text, want) {
				t.Fatalf("%s prompt = %q, want it to contain %q", tc.name, text, want)
			}
		}
	}
}

func TestChannelQuestionAnswersMatchLinesToQuestions(t *testing.T) {
	request := map[string]interface{}{"questions": []interface{}{
		map[string]interface{}{"id": "time"}, map[string]interface{}{"id": "who"},
	}}
	got := channelQuestionAnswers(request, "1. 早上八点\n2. 我自己")
	if got["time"] != "早上八点" || got["who"] != "我自己" {
		t.Fatalf("line answers = %#v", got)
	}
	got = channelQuestionAnswers(request, "早上八点提醒我自己")
	if got["time"] != "早上八点提醒我自己" || got["who"] != "早上八点提醒我自己" {
		t.Fatalf("one-line answers = %#v", got)
	}
}

type channelConnectorStoreStub struct {
	*channelApprovalStoreStub
	completed []string
}

func (s *channelConnectorStoreStub) CompleteRun(_ context.Context, _ uuid.UUID, summary string, _ datatypes.JSON, _ time.Time) error {
	s.completed = append(s.completed, summary)
	return nil
}

type channelConnectorConfirmerStub struct {
	owner, receipt uuid.UUID
	confirm        *bool
}

func (c *channelConnectorConfirmerStub) DecideChannelConnectorConfirmation(_ context.Context, _, owner, receipt uuid.UUID, confirm bool) (ChannelConnectorDecision, error) {
	c.owner, c.receipt, c.confirm = owner, receipt, &confirm
	if confirm {
		return ChannelConnectorDecision{State: "completed", Title: "发送邮件"}, nil
	}
	return ChannelConnectorDecision{State: "cancelled", Title: "发送邮件"}, nil
}

// The member who asked confirms a connector operation by replying; anything
// but an explicit answer is asked again, and nothing is decided.
func TestChannelReplyDecidesConnectorConfirmation(t *testing.T) {
	encoded, err := json.Marshal(invocation.Event{
		Origin: invocation.Origin{Kind: invocation.OriginChannel},
		Sender: invocation.PrincipalRef{Type: invocation.PrincipalExternalUser, ID: "ou_initiator"},
	})
	if err != nil {
		t.Fatal(err)
	}
	actor, receipt := uuid.New(), uuid.New()
	newService := func() (*RuntimeService, *channelConnectorStoreStub, *channelConnectorConfirmerStub) {
		store := &channelConnectorStoreStub{channelApprovalStoreStub: &channelApprovalStoreStub{run: &Run{
			ID: uuid.New(), ActorID: &actor, WaitingReceipt: &receipt, Status: RunStatusWaiting,
			WaitingReason: runtimemodel.MessageStatusWaitingApproval, InvocationEvent: encoded,
		}}}
		confirmer := &channelConnectorConfirmerStub{}
		return &RuntimeService{
			chat: channelApprovalChatStub{messages: []*runtimemodel.Message{{
				ID: uuid.New(), Status: runtimemodel.MessageStatusWaitingApproval, Metadata: map[string]interface{}{},
			}}},
			store:             store,
			channelConnectors: confirmer,
			now:               time.Now,
		}, store, confirmer
	}
	scope := Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ActorID: uuid.New()}
	reply := func(text, sender string) ChannelApprovalReply {
		return ChannelApprovalReply{Text: text, SenderExternalID: sender, ConversationType: channelruntime.ConversationTypeGroup}
	}

	service, store, confirmer := newService()
	handled, err := service.ContinueChannelTaskApproval(context.Background(), scope, uuid.New(), uuid.New(), reply("确认", "ou_initiator"))
	if err != nil || !handled || confirmer.confirm == nil || !*confirmer.confirm || confirmer.owner != actor || confirmer.receipt != receipt {
		t.Fatalf("confirm: handled=%v err=%v confirmer=%+v", handled, err, confirmer)
	}
	if len(store.delivered) != 1 || !strings.Contains(store.delivered[0], "「发送邮件」已执行完成") || len(store.completed) != 1 {
		t.Fatalf("confirm delivered=%v completed=%v", store.delivered, store.completed)
	}

	service, store, confirmer = newService()
	if _, err := service.ContinueChannelTaskApproval(context.Background(), scope, uuid.New(), uuid.New(), reply("取消", "ou_initiator")); err != nil || confirmer.confirm == nil || *confirmer.confirm {
		t.Fatalf("cancel: err=%v confirmer=%+v", err, confirmer)
	}
	if len(store.delivered) != 1 || !strings.Contains(store.delivered[0], "不会执行") || len(store.completed) != 0 {
		t.Fatalf("cancel delivered=%v completed=%v", store.delivered, store.completed)
	}

	service, store, confirmer = newService()
	if _, err := service.ContinueChannelTaskApproval(context.Background(), scope, uuid.New(), uuid.New(), reply("先等等", "ou_initiator")); err != nil || confirmer.confirm != nil {
		t.Fatalf("unclear reply: err=%v confirmer=%+v", err, confirmer)
	}
	if len(store.delivered) != 1 || store.delivered[0] != channelConnectorReplyNotice {
		t.Fatalf("unclear reply delivered=%v", store.delivered)
	}

	service, store, confirmer = newService()
	if _, err := service.ContinueChannelTaskApproval(context.Background(), scope, uuid.New(), uuid.New(), reply("确认", "ou_other")); err != nil || confirmer.confirm != nil {
		t.Fatalf("other member: err=%v confirmer=%+v", err, confirmer)
	}
	if len(store.delivered) != 1 || store.delivered[0] != channelApprovalOtherSenderNotice {
		t.Fatalf("other member delivered=%v", store.delivered)
	}
}

// A run that has just stopped holds its question as Go values; the channel
// still sees the model's lead-in and every question.
func TestChannelWaitingPromptReadsQuestionsHeldInMemory(t *testing.T) {
	metadata := map[string]interface{}{"user_input_request": map[string]interface{}{
		"request_id": "call_1",
		"message":    "渠道测试第 1 题与第 2 题如下，请分别作答。",
		"questions": []map[string]interface{}{
			{"id": "test_q1", "question": "消息显示与发送是否都正常？"},
			{"id": "test_q2", "question": "提醒希望以什么形式送达？"},
		},
	}}
	text := channelWaitingPromptText(nil, runtimemodel.MessageStatusWaitingQuestion, "", metadata)
	for _, want := range []string{"渠道测试第 1 题与第 2 题如下", "1. 消息显示与发送是否都正常？", "2. 提醒希望以什么形式送达？", channelReplyQuestion} {
		if !strings.Contains(text, want) {
			t.Fatalf("prompt = %q, want it to contain %q", text, want)
		}
	}
}
