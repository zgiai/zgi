package digitalhuman

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
)

func TestDigitalHumanGovernanceConsoleRouteUsesDedicatedSSEContinuation(t *testing.T) {
	run, resolved, event, prepared := linuxExecutionFixture(t, SurfaceConsole)
	run.MessageID = &prepared.Message.ID
	access, err := executionAccessFor(run, resolved, event)
	if err != nil {
		t.Fatal(err)
	}
	store := newFakeRunStore()
	store.runs[run.ID] = run
	chat := &governanceContinuationChat{scopedConversationChat: &scopedConversationChat{fakeDigitalHumanChat: &fakeDigitalHumanChat{}}}
	service := NewRuntimeService(store, fakeExecutionResolver{}, fakeBrainAdapter{}, chat, &fakeRunDispatcher{})
	router := digitalHumanRuntimeHandlerRouter(t, service, access.scope())
	path := "/digital-humans/" + run.DigitalHumanID.String() + "/runtime/conversations/" + run.ConversationID.String() + "/messages/" + run.MessageID.String() + "/tool-governance/pending/continue"
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"action":"reject"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Header().Get("Content-Type"), "text/event-stream") || chat.calls != 1 || chat.caller.Type != "digital_human" || chat.decision != "reject" {
		t.Fatalf("continuation route: %d %s", response.Code, response.Body)
	}
	forged := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"action":"reject","caller_type":"agent","source":"console"}`))
	forged.Header.Set("Content-Type", "application/json")
	rejected := httptest.NewRecorder()
	router.ServeHTTP(rejected, forged)
	if rejected.Code != http.StatusBadRequest || chat.calls != 1 {
		t.Fatalf("client-selected identity reached continuation: %d", rejected.Code)
	}
}

type governanceContinuationChat struct {
	*scopedConversationChat
	calls    int
	decision string
}

func (c *governanceContinuationChat) RunConfiguredToolGovernanceDecisionStream(_ context.Context, scope chatruntime.Scope, caller chatruntime.Caller, config chatruntime.RunConfig,
	conversation, message uuid.UUID, correlation string, request runtimedto.ToolGovernanceDecisionRequest, onEvent func(chatruntime.StreamEvent) error) (*chatruntime.ChatResult, error) {
	c.calls++
	c.caller, c.config, c.decision = caller, config, request.Action
	return &chatruntime.ChatResult{Status: "completed"}, onEvent(chatruntime.StreamEvent{EventType: "message_end", Payload: map[string]interface{}{"status": "completed"}})
}

type continuationLinuxProvider struct {
	calls            int
	run, publication uuid.UUID
}

func (p *continuationLinuxProvider) Prepare(_ context.Context, run *Run, resolved *ResolvedExecution, _ invocation.Event) (func(*chatruntime.PreparedChat) []skillloop.RuntimeTool, error) {
	p.calls++
	p.run, p.publication = run.ID, resolved.Snapshot.DigitalHumanVersion
	return func(*chatruntime.PreparedChat) []skillloop.RuntimeTool { return nil }, nil
}

func TestDigitalHumanGovernanceUsesSealedRunAcrossInteractiveEntries(t *testing.T) {
	for _, surface := range []string{SurfaceConsole, SurfaceWebApp, SurfaceAPI} {
		t.Run(surface, func(t *testing.T) {
			run, resolved, event, prepared := linuxExecutionFixture(t, surface)
			run.MessageID = &prepared.Message.ID
			access, err := executionAccessFor(run, resolved, event)
			if err != nil {
				t.Fatal(err)
			}
			scope := access.scope()
			store := newFakeRunStore()
			store.runs[run.ID] = run
			chat := &governanceContinuationChat{scopedConversationChat: &scopedConversationChat{fakeDigitalHumanChat: &fakeDigitalHumanChat{}}}
			linux := &continuationLinuxProvider{}
			service := NewRuntimeService(store, fakeExecutionResolver{}, fakeBrainAdapter{}, chat, &fakeRunDispatcher{}, WithLinuxExecutionRuntime(linux))
			for _, action := range []string{"approve", "reject"} {
				if action == "reject" {
					service.linux = nil
				}
				_, err := service.ContinueToolGovernance(t.Context(), scope, run.DigitalHumanID, *run.ConversationID, *run.MessageID, "pending", runtimedto.ToolGovernanceDecisionRequest{Action: action}, func(chatruntime.StreamEvent) error { return nil })
				if err != nil {
					t.Fatal(err)
				}
				if chat.caller.Type != "digital_human" || chat.caller.Source != prepared.Caller.Source || chat.caller.RunID == nil || *chat.caller.RunID != run.ID || chat.decision != action {
					t.Fatal("continuation replaced original run/entry")
				}
			}
			if linux.calls != 1 || linux.publication != run.DigitalHumanVersionID {
				t.Fatal("rejection prepared compute or approval switched publication")
			}
			other := scope
			user := uuid.New()
			other.UserSubjectID = &user
			other.ActorID = user
			if _, err := service.ContinueToolGovernance(t.Context(), other, run.DigitalHumanID, *run.ConversationID, *run.MessageID, "pending", runtimedto.ToolGovernanceDecisionRequest{Action: "reject"}, func(chatruntime.StreamEvent) error { return nil }); err == nil {
				t.Fatal("other final user continued approval")
			}
			if chat.calls != 2 {
				t.Fatal("cross-user request reached ChatRuntime")
			}
		})
	}
}
