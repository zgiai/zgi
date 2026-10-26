package digitalhuman

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
)

type taskApprovalScopeRecorder struct {
	RuntimeHandlerService
	scopes *[]Scope
}

func (r taskApprovalScopeRecorder) ContinueTaskApproval(_ context.Context, scope Scope, _, _, _ uuid.UUID, _ runtimeservice.DigitalHumanTaskApprovalRequest, _ func(runtimeservice.StreamEvent) error) (*runtimeservice.ChatResult, error) {
	*r.scopes = append(*r.scopes, scope)
	return &runtimeservice.ChatResult{}, nil
}

// Console task approvals must carry the account UserSubject, because the
// continuation re-checks run ownership (including Linux execution
// re-authorization) through continuationRunOwnedByScope.
func TestConsoleTaskApprovalScopeOwnsTheConsoleRun(t *testing.T) {
	scope := testScope()
	digitalHumanID, conversationID, messageID := uuid.New(), uuid.New(), uuid.New()
	scopes := []Scope{}
	router := digitalHumanRuntimeHandlerRouter(t, taskApprovalScopeRecorder{scopes: &scopes}, scope)
	recorder := httptest.NewRecorder()
	path := "/digital-humans/" + digitalHumanID.String() + "/runtime/conversations/" + conversationID.String() + "/messages/" + messageID.String() + "/workflow-continuation"
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{"type":"approval","approval_token":"`+uuid.NewString()+`","action":"approve","inputs":{}}`)))
	if len(scopes) != 1 {
		t.Fatalf("continuation was not called: status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	got := scopes[0]
	if got.UserSubjectID == nil || *got.UserSubjectID != scope.ActorID || got.UserSubjectKind != string(usersubject.Account) {
		t.Fatalf("approval scope subject = %v/%q, want account %s", got.UserSubjectID, got.UserSubjectKind, scope.ActorID)
	}
	actor := scope.ActorID
	consoleRun := &Run{
		OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, DigitalHumanID: digitalHumanID,
		Surface: SurfaceConsole, UserSubjectKind: string(usersubject.Account), UserSubjectID: &actor,
	}
	if !continuationRunOwnedByScope(consoleRun, got, digitalHumanID) {
		t.Fatal("the console approval scope does not own the console run it continues")
	}
	otherActor := uuid.New()
	otherRun := *consoleRun
	otherRun.UserSubjectID = &otherActor
	if continuationRunOwnedByScope(&otherRun, got, digitalHumanID) {
		t.Fatal("the console approval scope owns another account's run")
	}
}

type taskApprovalContinuationChat struct {
	*scopedConversationChat
	callers []runtimeservice.Caller
}

func (c *taskApprovalContinuationChat) RunDigitalHumanTaskApprovalContinuation(_ context.Context, _ runtimeservice.Scope, caller runtimeservice.Caller, _ runtimeservice.RunConfig,
	_, _ uuid.UUID, _ runtimeservice.DigitalHumanTaskApprovalRequest, _ runtimeservice.DigitalHumanTaskApprovalHandler, _ func(runtimeservice.StreamEvent) error) (*runtimeservice.ChatResult, error) {
	c.callers = append(c.callers, caller)
	return &runtimeservice.ChatResult{Status: "completed"}, nil
}

// The follow-up answer after a task approval resolves run-scoped capabilities,
// such as delegated Connector access, which require the caller to carry the
// sealed run. Without it the follow-up fails with connector_access_denied.
func TestConsoleTaskApprovalContinuesTheSealedRun(t *testing.T) {
	run, resolved, event, prepared := linuxExecutionFixture(t, SurfaceConsole)
	run.MessageID = &prepared.Message.ID
	access, err := executionAccessFor(run, resolved, event)
	if err != nil {
		t.Fatal(err)
	}
	store := newFakeRunStore()
	store.runs[run.ID] = run
	chat := &taskApprovalContinuationChat{scopedConversationChat: &scopedConversationChat{fakeDigitalHumanChat: &fakeDigitalHumanChat{}}}
	service := NewRuntimeService(store, fakeExecutionResolver{}, fakeBrainAdapter{}, chat, &fakeRunDispatcher{},
		WithTaskRuntime(NewTaskRuntimeProvider(taskRuntimeService{})))
	_, err = service.ContinueTaskApproval(t.Context(), access.scope(), run.DigitalHumanID, *run.ConversationID, *run.MessageID,
		runtimeservice.DigitalHumanTaskApprovalRequest{ApprovalToken: uuid.NewString(), Action: "approve"}, func(runtimeservice.StreamEvent) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(chat.callers) != 1 || chat.callers[0].RunID == nil || *chat.callers[0].RunID != run.ID {
		t.Fatalf("task approval continuation caller = %#v, want run %s", chat.callers, run.ID)
	}
}
