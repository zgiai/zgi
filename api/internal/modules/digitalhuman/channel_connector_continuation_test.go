package digitalhuman

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	connectorconfirmation "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeconfirmation"
	"gorm.io/datatypes"
)

// connectorContinuationStore records how a continued channel run settles.
type connectorContinuationStore struct {
	*fakeRunStore
	mu          sync.Mutex
	settled     chan string
	finals      []string
	notices     []string
	nextReceipt *uuid.UUID
	waitErr     error
}

func (s *connectorContinuationStore) CompleteRunWithDelivery(ctx context.Context, runID uuid.UUID, summary string, trace datatypes.JSON, now time.Time) error {
	err := s.fakeRunStore.CompleteRun(ctx, runID, summary, trace, now)
	s.mu.Lock()
	if err == nil {
		s.finals = append(s.finals, summary)
	}
	s.mu.Unlock()
	s.settled <- "completed"
	return err
}

func (s *connectorContinuationStore) FailRun(ctx context.Context, runID uuid.UUID, code, message string, now time.Time) error {
	err := s.fakeRunStore.FailRun(ctx, runID, code, message, now)
	s.settled <- "failed"
	return err
}

func (s *connectorContinuationStore) MarkRunWaiting(_ context.Context, runID uuid.UUID, reason string, now time.Time) error {
	if s.waitErr != nil {
		return s.waitErr
	}
	run := s.runs[runID]
	if run == nil || run.Status != RunStatusRunning {
		return ErrRunConflict
	}
	run.Status, run.WaitingReason, run.WaitingReceipt, run.UpdatedAt = RunStatusWaiting, reason, s.nextReceipt, now
	return nil
}

func (s *connectorContinuationStore) ResumeWaitingRun(context.Context, uuid.UUID, time.Time) error {
	return nil
}

func (s *connectorContinuationStore) CancelWaitingRun(context.Context, uuid.UUID, time.Time) error {
	return nil
}

func (s *connectorContinuationStore) EnqueueRunChannelFinalDelivery(_ context.Context, _ uuid.UUID, _ string, text string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notices = append(s.notices, text)
	return nil
}

// connectorContinuationChat is the chat runtime's side of a decided
// confirmation: the claim takes the waiting message once, and the model pass
// answers from the result that replaced the paused tool result.
type connectorContinuationChat struct {
	*fakeDigitalHumanChat
	store       *connectorContinuationStore
	mu          sync.Mutex
	listed      *runtimemodel.Message
	status      string
	claims      int
	toolResults []string
	pass        func(*chatruntime.ConnectorConfirmationContinuation) (*chatruntime.ChatResult, error)
}

// ListConversationMessagesByCaller returns the message as the inbound reply
// read it, so a repeated event can race the first one's claim.
func (c *connectorContinuationChat) ListConversationMessagesByCaller(context.Context, chatruntime.Scope, chatruntime.Caller, uuid.UUID, int, int) ([]*runtimemodel.Message, int64, error) {
	listed := *c.listed
	return []*runtimemodel.Message{&listed}, 1, nil
}

func (c *connectorContinuationChat) ClaimConnectorConfirmationContinuation(_ context.Context, _ connectorconfirmation.Command, result connectorconfirmation.Result) (*chatruntime.ConnectorConfirmationContinuation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.status != runtimemodel.MessageStatusWaitingApproval {
		return nil, connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	c.status, c.claims = runtimemodel.MessageStatusStreaming, c.claims+1
	run := c.store.runs[c.store.runID()]
	receipt := run.WaitingReceipt.String()
	run.Status, run.WaitingReason, run.WaitingReceipt = RunStatusRunning, "", nil
	toolResult := `{"state":"` + string(result.State) + `"}`
	if result.ExecutionResult != nil {
		toolResult = `{"stdout":"` + result.ExecutionResult.Value.(map[string]any)["stdout"].(string) + `"}`
	}
	return &chatruntime.ConnectorConfirmationContinuation{
		OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID, ConversationID: *run.ConversationID,
		MessageID: *run.MessageID, DigitalHumanID: run.DigitalHumanID, RunID: run.ID, RouteReceipt: receipt,
		State: result.State, ToolResult: toolResult,
	}, nil
}

func (c *connectorContinuationChat) RunConfiguredConnectorConfirmationContinuation(_ context.Context, _ chatruntime.Scope, _ chatruntime.Caller, _ chatruntime.RunConfig, claim *chatruntime.ConnectorConfirmationContinuation, _ func(chatruntime.StreamEvent) error) (*chatruntime.ChatResult, error) {
	c.mu.Lock()
	c.toolResults = append(c.toolResults, claim.ToolResult)
	c.mu.Unlock()
	result, err := c.pass(claim)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.status = runtimemodel.MessageStatusError
		return nil, err
	}
	c.status = result.Status
	return result, nil
}

func (s *connectorContinuationStore) runID() uuid.UUID {
	for id := range s.runs {
		return id
	}
	return uuid.Nil
}

// continuingConnectorConfirmer stands for the route confirmer: the
// confirmation service executes the operation for the first decision and
// replays its terminal for a repeat, and the DigitalHuman turn continues.
type continuingConnectorConfirmer struct {
	service    *RuntimeService
	mu         sync.Mutex
	executions int
	decided    *connectorconfirmation.State
	origin     connectorexecution.ConfirmationOrigin
}

func (c *continuingConnectorConfirmer) DecideChannelConnectorConfirmation(ctx context.Context, organizationID, ownerAccountID, receipt uuid.UUID, confirm bool) (ChannelConnectorDecision, error) {
	decision, state := connectorexecution.ConfirmationDecisionCancelled, connectorconfirmation.StateCancelled
	if confirm {
		decision, state = connectorexecution.ConfirmationDecisionConfirmed, connectorconfirmation.StateCompleted
	}
	result := connectorconfirmation.Result{State: state, Origin: c.origin}
	c.mu.Lock()
	if c.decided == nil {
		c.decided = &state
		if confirm {
			c.executions++
			result.ExecutionResult = &connectorexecution.ExecutionResult{
				State: connectorexecution.ExecutionStateCompleted, Value: map[string]any{"stdout": "nginx: active (running)"},
			}
		}
	}
	c.mu.Unlock()
	command := connectorconfirmation.Command{
		OrganizationID: organizationID, OwnerAccountID: ownerAccountID,
		RouteReceipt: connectorexecution.RouteClaimReceipt(receipt.String()), Decision: decision,
	}
	if err := c.service.ResumeAfterConnectorConfirmation(ctx, command, result); err != nil {
		if connectorcore.IsErrorCode(err, connectorcore.ErrorCodeConflict) {
			return ChannelConnectorDecision{}, ErrRunConflict
		}
		return ChannelConnectorDecision{}, err
	}
	return ChannelConnectorDecision{State: string(state), Title: "查看服务状态", Continued: true}, nil
}

type channelConnectorContinuationFixture struct {
	service        *RuntimeService
	store          *connectorContinuationStore
	chat           *connectorContinuationChat
	confirmer      *continuingConnectorConfirmer
	scope          Scope
	digitalHumanID uuid.UUID
	conversationID uuid.UUID
	run            *Run
}

// newChannelConnectorContinuationFixture is a channel direct-message run that
// waits for the person to confirm an SSH operation.
func newChannelConnectorContinuationFixture(t *testing.T) *channelConnectorContinuationFixture {
	t.Helper()
	store := &connectorContinuationStore{fakeRunStore: newFakeRunStore(), settled: make(chan string, 4)}
	base := &fakeDigitalHumanChat{}
	now := time.Date(2026, 10, 31, 16, 49, 44, 0, time.UTC)
	chat := &connectorContinuationChat{fakeDigitalHumanChat: base, store: store, status: runtimemodel.MessageStatusWaitingApproval}
	service := NewRuntimeService(store, fakeExecutionResolver{now: now}, fakeBrainAdapter{}, chat, &fakeRunDispatcher{},
		WithChannelTransientDispatcher(&channelTransientDispatcherStub{}))
	service.now = func() time.Time { return now }

	scope, digitalHumanID, conversationID, messageID := testRunScope(), uuid.New(), uuid.New(), uuid.New()
	installationID := uuid.NewString()
	event := invocation.Event{
		ID: "feishu-ssh-task", Type: invocation.EventTypeChannelMessage,
		Origin: invocation.Origin{Kind: invocation.OriginChannel, Provider: "feishu", InstallationID: installationID},
		Target: runtimeidentity.SubjectRef{
			Type: runtimeidentity.SubjectTypeDigitalHuman, ID: digitalHumanID,
			OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		},
		Sender:     invocation.PrincipalRef{Type: invocation.PrincipalExternalUser, ID: "ou_requester"},
		OccurredAt: now,
		Context:    map[string]any{"conversation_type": string(channelruntime.ConversationTypeDirect)},
		ReplyTo:    &invocation.ReplyAddress{Provider: "feishu", InstallationID: installationID, ConversationID: "oc_private"},
	}
	run, created, err := service.SubmitInvocation(t.Context(), scope, event, "检查 nginx 状态并告诉我结论", &conversationID)
	if err != nil || !created {
		t.Fatalf("SubmitInvocation() = created %v, err %v", created, err)
	}
	run = store.runs[run.ID]
	receipt, actor := uuid.New(), scope.ActorID
	run.Status, run.WaitingReason, run.WaitingReceipt = RunStatusWaiting, runtimemodel.MessageStatusWaitingApproval, &receipt
	run.ConversationID, run.MessageID, run.ActorID = &conversationID, &messageID, &actor
	chat.listed = &runtimemodel.Message{ID: messageID, ConversationID: conversationID, Status: runtimemodel.MessageStatusWaitingApproval, Metadata: map[string]interface{}{}}
	chat.pass = func(claim *chatruntime.ConnectorConfirmationContinuation) (*chatruntime.ChatResult, error) {
		return &chatruntime.ChatResult{Status: runtimemodel.MessageStatusCompleted, Answer: "nginx 正在运行，服务正常。依据：" + claim.ToolResult}, nil
	}
	confirmer := &continuingConnectorConfirmer{service: service, origin: connectorexecution.ConfirmationOrigin{
		Surface: connectorcore.TargetTypeDigitalHuman, ConversationID: conversationID.String(), MessageID: messageID.String(),
	}}
	service.SetChannelConnectorConfirmer(confirmer)
	return &channelConnectorContinuationFixture{
		service: service, store: store, chat: chat, confirmer: confirmer,
		scope: scope, digitalHumanID: digitalHumanID, conversationID: conversationID, run: run,
	}
}

func (f *channelConnectorContinuationFixture) reply(t *testing.T, text string) {
	t.Helper()
	handled, err := f.service.ContinueChannelTaskApproval(t.Context(), f.scope, f.digitalHumanID, f.conversationID, ChannelApprovalReply{
		Text: text, SenderExternalID: "ou_requester", ConversationType: channelruntime.ConversationTypeDirect,
	})
	if err != nil || !handled {
		t.Fatalf("reply %q: handled=%v err=%v", text, handled, err)
	}
}

func (f *channelConnectorContinuationFixture) channelScope() Scope {
	scope := f.scope
	scope.Surface, scope.PrincipalType, scope.PrincipalID = SurfaceChannel, string(invocation.PrincipalAccount), scope.ActorID
	return scope
}

func (f *channelConnectorContinuationFixture) awaitSettled(t *testing.T) string {
	t.Helper()
	select {
	case outcome := <-f.store.settled:
		return outcome
	case <-time.After(5 * time.Second):
		t.Fatal("the continued run did not settle")
		return ""
	}
}

// Issue 2925: confirming an operation from a channel continues the same turn
// with its result, and the channel receives the model's answer, not a
// generic receipt. A repeated reply executes nothing and delivers nothing.
func TestChannelConnectorConfirmationContinuesTurnToItsAnswer(t *testing.T) {
	f := newChannelConnectorContinuationFixture(t)
	stale := *f.run
	f.reply(t, "确认")
	if outcome := f.awaitSettled(t); outcome != "completed" {
		t.Fatalf("continued run settled as %s", outcome)
	}
	if len(f.chat.toolResults) != 1 || !strings.Contains(f.chat.toolResults[0], "active (running)") {
		t.Fatalf("the model saw %v, want the operation's result", f.chat.toolResults)
	}
	if f.run.Status != RunStatusSucceeded || !strings.Contains(f.run.ResultSummary, "nginx 正在运行") {
		t.Fatalf("run = %s with summary %q", f.run.Status, f.run.ResultSummary)
	}
	if len(f.store.finals) != 1 || f.store.finals[0] != f.run.ResultSummary {
		t.Fatalf("channel final deliveries = %q", f.store.finals)
	}
	if len(f.store.notices) != 0 {
		t.Fatalf("the channel also received %q", f.store.notices)
	}

	// The same reply delivered again, which read the run before the first
	// reply's claim, finds the operation decided and the turn claimed.
	handled, err := f.service.continueChannelConnectorConfirmation(t.Context(), f.channelScope(), f.digitalHumanID, f.chat.listed, &stale, ChannelApprovalReply{
		Text: "确认", SenderExternalID: "ou_requester", ConversationType: channelruntime.ConversationTypeDirect,
	}, "approve")
	if err != nil || !handled {
		t.Fatalf("repeated reply: handled=%v err=%v", handled, err)
	}
	// A reply after the run ended is not an answer to the decided operation.
	if handled, err := f.service.ContinueChannelTaskApproval(t.Context(), f.scope, f.digitalHumanID, f.conversationID, ChannelApprovalReply{
		Text: "确认", SenderExternalID: "ou_requester", ConversationType: channelruntime.ConversationTypeDirect,
	}); err != nil || handled {
		t.Fatalf("late reply: handled=%v err=%v", handled, err)
	}
	select {
	case outcome := <-f.store.settled:
		t.Fatalf("a repeated reply settled the run again: %s", outcome)
	case <-time.After(100 * time.Millisecond):
	}
	if f.confirmer.executions != 1 || f.chat.claims != 1 || len(f.chat.toolResults) != 1 ||
		len(f.store.finals) != 1 || len(f.store.notices) != 0 {
		t.Fatalf("repeat: executions=%d claims=%d passes=%d finals=%q notices=%q",
			f.confirmer.executions, f.chat.claims, len(f.chat.toolResults), f.store.finals, f.store.notices)
	}
}

// A continued turn that stops at a further confirmation waits again and asks
// the channel for it, under the new receipt.
func TestChannelConnectorConfirmationWaitsAgainForTheNextOperation(t *testing.T) {
	f := newChannelConnectorContinuationFixture(t)
	next := uuid.New()
	f.store.nextReceipt = &next
	f.chat.pass = func(*chatruntime.ConnectorConfirmationContinuation) (*chatruntime.ChatResult, error) {
		return &chatruntime.ChatResult{Status: runtimemodel.MessageStatusWaitingApproval, Answer: "nginx 正常，接下来需要重启 php-fpm。", Metadata: map[string]interface{}{}}, nil
	}
	f.reply(t, "确认")
	deadline := time.Now().Add(5 * time.Second)
	for {
		f.store.mu.Lock()
		delivered := len(f.store.notices)
		f.store.mu.Unlock()
		if delivered > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if f.run.Status != RunStatusWaiting || f.run.WaitingReceipt == nil || *f.run.WaitingReceipt != next {
		t.Fatalf("run = %s waiting on %v, want waiting on %s", f.run.Status, f.run.WaitingReceipt, next)
	}
	if len(f.store.notices) != 1 || !strings.Contains(f.store.notices[0], "接下来需要重启 php-fpm") ||
		!strings.Contains(f.store.notices[0], channelReplyConnector) {
		t.Fatalf("channel prompts = %q", f.store.notices)
	}
	if len(f.store.finals) != 0 {
		t.Fatalf("a waiting run delivered a final answer: %q", f.store.finals)
	}
}

// A cancelled operation also continues the turn, which says it was not
// executed; a pass that fails ends the run failed and tells the channel.
func TestChannelConnectorConfirmationCancelledAndFailedContinuations(t *testing.T) {
	f := newChannelConnectorContinuationFixture(t)
	f.chat.pass = func(claim *chatruntime.ConnectorConfirmationContinuation) (*chatruntime.ChatResult, error) {
		return &chatruntime.ChatResult{Status: runtimemodel.MessageStatusCompleted, Answer: "已取消，没有执行检查。" + string(claim.State)}, nil
	}
	f.reply(t, "取消")
	if outcome := f.awaitSettled(t); outcome != "completed" {
		t.Fatalf("cancelled continuation settled as %s", outcome)
	}
	if f.confirmer.executions != 0 || len(f.chat.toolResults) != 1 || !strings.Contains(f.chat.toolResults[0], "cancelled") {
		t.Fatalf("cancel: executions=%d model saw %v", f.confirmer.executions, f.chat.toolResults)
	}
	if f.run.Status != RunStatusSucceeded || len(f.store.finals) != 1 || !strings.Contains(f.store.finals[0], "已取消") || len(f.store.notices) != 0 {
		t.Fatalf("cancel: run=%s finals=%q notices=%q", f.run.Status, f.store.finals, f.store.notices)
	}

	f = newChannelConnectorContinuationFixture(t)
	f.chat.pass = func(*chatruntime.ConnectorConfirmationContinuation) (*chatruntime.ChatResult, error) {
		return nil, errors.New("model unavailable")
	}
	f.reply(t, "确认")
	if outcome := f.awaitSettled(t); outcome != "failed" {
		t.Fatalf("failed continuation settled as %s", outcome)
	}
	if f.run.Status != RunStatusFailed || f.chat.status != runtimemodel.MessageStatusError || len(f.store.finals) != 0 {
		t.Fatalf("failure: run=%s message=%s finals=%q", f.run.Status, f.chat.status, f.store.finals)
	}
	if notices := f.awaitNotices(t, 1); len(notices) != 1 || notices[0] != channelContinuationFailedNotice {
		t.Fatalf("failure notices = %q", f.store.notices)
	}
}

// An API confirmation that continues the turn leaves the run to its answer.
func TestAPIConnectorConfirmationLeavesAContinuedRunToItsAnswer(t *testing.T) {
	subject, receipt, conversationID, messageID, digitalHumanID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	scope := testRunScope()
	scope.Surface, scope.UserSubjectID, scope.UserSubjectKind = SurfaceAPI, &subject, "api_end_user"
	store := &channelConnectorStoreStub{channelApprovalStoreStub: &channelApprovalStoreStub{run: &Run{
		ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, DigitalHumanID: digitalHumanID,
		Surface: SurfaceAPI, UserSubjectID: &subject, UserSubjectKind: "api_end_user",
		ConversationID: &conversationID, MessageID: &messageID, WaitingReceipt: &receipt, Status: RunStatusWaiting,
	}}}
	service := &RuntimeService{store: store, channelConnectors: &channelConnectorConfirmerStub{continued: true}, now: time.Now}
	decision, err := service.DecideAPIConnectorConfirmation(t.Context(), scope, digitalHumanID, conversationID, messageID, true)
	if err != nil || !decision.Continued {
		t.Fatalf("DecideAPIConnectorConfirmation() = %+v, %v", decision, err)
	}
	if len(store.completed) != 0 {
		t.Fatalf("a continued API run was completed with %q", store.completed)
	}
}

// awaitNotices waits for the channel notices a failed continuation sends after
// failing its run, then returns them.
func (f *channelConnectorContinuationFixture) awaitNotices(t *testing.T, want int) []string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		f.store.mu.Lock()
		notices := append([]string(nil), f.store.notices...)
		f.store.mu.Unlock()
		if len(notices) >= want || time.Now().After(deadline) {
			// A second notice would arrive right after the first.
			time.Sleep(50 * time.Millisecond)
			f.store.mu.Lock()
			defer f.store.mu.Unlock()
			return append([]string(nil), f.store.notices...)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type failingRunConfigBrain struct{ fakeBrainAdapter }

func (failingRunConfigBrain) RunConfig(*ResolvedExecution) (chatruntime.RunConfig, error) {
	return chatruntime.RunConfig{}, errors.New("model configuration unavailable")
}

// Once the decided operation has run, every way the continuation stops
// (preparing the pass, settling it, or a panic) fails the run and tells the
// channel once, instead of leaving the person with neither answer nor error.
func TestChannelConnectorConfirmationEveryContinuationFailureTellsTheChannel(t *testing.T) {
	cases := map[string]func(*channelConnectorContinuationFixture){
		"preparing the pass": func(f *channelConnectorContinuationFixture) {
			f.service.brain = failingRunConfigBrain{}
		},
		"settling the pass": func(f *channelConnectorContinuationFixture) {
			f.store.waitErr = errors.New("database unavailable")
			f.chat.pass = func(*chatruntime.ConnectorConfirmationContinuation) (*chatruntime.ChatResult, error) {
				return &chatruntime.ChatResult{Status: runtimemodel.MessageStatusWaitingApproval, Metadata: map[string]interface{}{}}, nil
			}
		},
		"a panic in the pass": func(f *channelConnectorContinuationFixture) {
			f.chat.pass = func(*chatruntime.ConnectorConfirmationContinuation) (*chatruntime.ChatResult, error) {
				panic("unexpected state")
			}
		},
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			f := newChannelConnectorContinuationFixture(t)
			breakIt(f)
			f.reply(t, "确认")
			if outcome := f.awaitSettled(t); outcome != "failed" {
				t.Fatalf("continuation settled as %s", outcome)
			}
			notices := f.awaitNotices(t, 1)
			if f.confirmer.executions != 1 || f.run.Status != RunStatusFailed || len(f.store.finals) != 0 {
				t.Fatalf("executions=%d run=%s finals=%q", f.confirmer.executions, f.run.Status, f.store.finals)
			}
			if len(notices) != 1 || notices[0] != channelContinuationFailedNotice {
				t.Fatalf("channel notices = %q", notices)
			}
		})
	}
}

// A continuation whose run was stopped meanwhile ends with the stop: the
// channel is not told it failed.
func TestChannelConnectorConfirmationStoppedRunIsNotReportedAsFailed(t *testing.T) {
	f := newChannelConnectorContinuationFixture(t)
	f.chat.pass = func(*chatruntime.ConnectorConfirmationContinuation) (*chatruntime.ChatResult, error) {
		now := time.Now()
		f.run.CancelRequestedAt = &now
		return nil, chatruntime.ErrMessageStopped
	}
	f.reply(t, "确认")
	if outcome := f.awaitSettled(t); outcome != "failed" {
		t.Fatalf("continuation settled as %s", outcome)
	}
	if notices := f.awaitNotices(t, 1); f.run.Status != RunStatusCanceled || len(notices) != 0 {
		t.Fatalf("run=%s notices=%q", f.run.Status, notices)
	}
}
