package digitalhuman

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/runspec"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type backgroundOptInResolver struct{ fakeExecutionResolver }

func (r backgroundOptInResolver) Resolve(ctx context.Context, event invocation.Event, version *uuid.UUID) (*ResolvedExecution, error) {
	resolved, err := r.fakeExecutionResolver.Resolve(ctx, event, version)
	if err != nil {
		return nil, err
	}
	resolved.Snapshot.ExecutionRuntime = &ExecutionRuntimeConfig{Enabled: true}
	resolved.Snapshot.Revision, err = executionSnapshotRevision(resolved.Snapshot)
	if err != nil {
		return nil, err
	}
	resolved.Spec.Identity.Revision = resolved.Snapshot.Revision
	resolved.Spec.Model.Revision = resolved.Snapshot.Revision
	resolved.Spec.Policies = []runspec.VersionRef{{Kind: "execution_snapshot", ID: resolved.Snapshot.DigitalHumanVersion.String(), Revision: resolved.Snapshot.Revision}}
	resolved.Spec.Revision = ""
	resolved.Spec, err = runspec.Seal(resolved.Spec)
	return resolved, err
}

func TestNewLinuxTaskConversationIsFreshAndAcceptanceIsIdempotent(t *testing.T) {
	store, dispatcher, chat := newFakeRunStore(), &fakeRunDispatcher{}, &fakeDigitalHumanChat{}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	service.resolver = backgroundOptInResolver{fakeExecutionResolver{now: service.now()}}
	source, _, event, _ := backgroundLinuxFixture(t, SurfaceTask)
	delete(event.Context, backgroundExecutionEntryKey)
	scope := Scope{OrganizationID: source.OrganizationID, WorkspaceID: source.WorkspaceID, ActorID: *source.ActorID, PrincipalID: *source.ActorID, PrincipalType: source.PrincipalType, Surface: SurfaceTask}
	first, created, err := service.SubmitInvocation(t.Context(), scope, event, "make a report", nil)
	if err != nil || !created || first.ConversationID == nil || digitalHumanConversationSourceForRun(first) != backgroundTaskEntry {
		t.Fatalf("first=%+v created=%t error=%v", first, created, err)
	}
	if _, exists := event.Context[backgroundExecutionEntryKey]; exists {
		t.Fatal("server marker mutated caller-owned context")
	}
	second, created, err := service.SubmitInvocation(t.Context(), scope, event, "make a report", nil)
	if err != nil || created || second.ID != first.ID || *second.ConversationID != *first.ConversationID {
		t.Fatal("retry allocated another task environment")
	}
	if err := service.Execute(t.Context(), first.ID); err != nil {
		t.Fatal(err)
	}
	if chat.ensureCalls != 1 || chat.ensuredConversationID != *first.ConversationID || chat.caller.Source != backgroundTaskEntry {
		t.Fatalf("task persisted under wrong conversation source: %+v", chat.caller)
	}
	event.ID = uuid.NewString()
	event.IdempotencyKey = ""
	event.Context["action_run_id"] = event.ID
	next, _, err := service.SubmitInvocation(t.Context(), scope, event, "make another report", nil)
	if err != nil || next.ConversationID == nil || *next.ConversationID == *first.ConversationID {
		t.Fatal("new scheduled run inherited old business files")
	}
}

func backgroundLinuxFixture(t *testing.T, entry string) (*Run, *ResolvedExecution, invocation.Event, *chatruntime.PreparedChat) {
	t.Helper()
	run, resolved, event, p := linuxExecutionFixture(t, SurfaceConsole)
	run.Surface, run.UserSubjectKind, run.UserSubjectID = entry, "", nil
	p.Caller.Source, p.Conversation.Source = entry, entry
	if entry == SurfaceTask {
		p.Caller.Source, p.Conversation.Source = backgroundTaskEntry, backgroundTaskEntry
	}
	run.MessageID = &p.Message.ID
	if entry == SurfaceChannel {
		event.Type, event.Origin = invocation.EventTypeChannelMessage, invocation.Origin{Kind: invocation.OriginChannel, Provider: "feishu", InstallationID: uuid.NewString()}
		event.Sender = invocation.PrincipalRef{Type: invocation.PrincipalExternalUser, ID: "ou_verified_user"}
		event.Context = map[string]any{"channel_binding_id": uuid.NewString(), "external_conversation_id": "oc_verified"}
		event.ReplyTo = &invocation.ReplyAddress{Provider: event.Origin.Provider, InstallationID: event.Origin.InstallationID, ConversationID: "oc_verified"}
	} else {
		event.Type, event.Origin = invocation.EventTypeSchedule, invocation.Origin{Kind: invocation.OriginSchedule}
		event.Sender = invocation.PrincipalRef{Type: invocation.PrincipalSystem, ID: "digital-human-task-scheduler"}
		event.Context = map[string]any{backgroundExecutionEntryKey: backgroundTaskEntry, "task_id": uuid.NewString(), "task_run_id": uuid.NewString(), "action_run_id": event.ID, "task_owner_kind": string(TaskOwnerDigitalHuman)}
	}
	run.PrincipalType = string(event.Sender.Type)
	event.IdempotencyKey = ""
	var err error
	event, err = invocation.ValidateEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	run.IdempotencyKey = event.IdempotencyKey
	run.InvocationEvent, run.RunSpec, run.ExecutionSnapshot, err = marshalRunSnapshots(event, resolved)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := decodeAndVerifyRun(run); err != nil {
		t.Fatal(err)
	}
	return run, resolved, event, p
}

func (a *linuxExecutionTestAccess) AuthorizeBackgroundLinuxExecution(_ context.Context, run *Run, event invocation.Event) error {
	if run.DigitalHumanID != a.application || run.DigitalHumanVersionID != a.publication || (run.Surface == SurfaceTask && event.Origin.Kind != invocation.OriginSchedule) || (run.Surface == SurfaceChannel && event.Origin.Kind != invocation.OriginChannel) {
		return execution.ErrInvalidIdentity
	}
	return a.err
}

func TestBackgroundLinuxToolsUseIsolatedSubjectAndRecheckRevocation(t *testing.T) {
	for _, entry := range []string{SurfaceChannel, SurfaceTask} {
		t.Run(entry, func(t *testing.T) {
			run, resolved, event, prepared := backgroundLinuxFixture(t, entry)
			// Build ordinary options first, then supply the explicit background
			// admission. The billing account must never become the execution user.
			run.UserSubjectID = &prepared.Scope.AccountID
			options, access, gateway := linuxExecutionOptionsFor(t, run)
			run.UserSubjectID = nil
			var err error
			options.Entries, err = execution.ParseEntryAdmission(`{"digital_human:` + prepared.Caller.Source + `":["` + run.OrganizationID.String() + `"]}`)
			if err != nil {
				t.Fatal(err)
			}
			options.PlatformCLI = true
			provider := NewLinuxExecutionRuntime(options)
			factory, err := provider.Prepare(t.Context(), run, resolved, event)
			if err != nil || factory == nil {
				t.Fatalf("prepare=%v", err)
			}
			tools := factory(prepared)
			if len(tools) != 2 || tools[0].PlatformInteractiveApproval || tools[0].BindPlatform != nil {
				t.Fatalf("background tools=%+v", tools)
			}
			call := adapter.ToolCall{ID: "background", Function: adapter.FunctionCall{Arguments: `{"command":"printf ok"}`}}
			tools[0].Handler(t.Context(), call)
			subject := execution.BackgroundSubject(run.OrganizationID, run.WorkspaceID, run.DigitalHumanID, *run.ConversationID, prepared.Caller.Source)
			if gateway.creates != 1 || gateway.scope.SubjectID != subject.ID.String() || gateway.scope.SubjectID == prepared.Scope.AccountID.String() || gateway.scope.Identity.Version != execution.BackgroundIdentityVersion {
				t.Fatalf("unexpected owner: %+v", gateway.scope)
			}
			access.err = errors.New("revoked")
			tools[0].Handler(t.Context(), call)
			if gateway.creates != 1 {
				t.Fatal("revoked background execution allocated environment")
			}
			prepared.Caller.RunID = uuidPointer(uuid.New())
			if len(factory(prepared)) != 0 {
				t.Fatal("another run adopted background scope")
			}
		})
	}
}

func TestBackgroundIdentityRejectsUnverifiedRoutingAndScheduler(t *testing.T) {
	for _, entry := range []string{SurfaceChannel, SurfaceTask} {
		t.Run(entry, func(t *testing.T) {
			run, resolved, event, _ := backgroundLinuxFixture(t, entry)
			if _, err := executionAccessFor(run, resolved, event); err != nil {
				t.Fatal(err)
			}
			for _, mutate := range []func(*Run, *invocation.Event){
				func(r *Run, _ *invocation.Event) { r.PrincipalType = "account" },
				func(_ *Run, e *invocation.Event) { e.Sender.Type = invocation.PrincipalAccount },
				func(_ *Run, e *invocation.Event) { e.Context = nil },
				func(r *Run, _ *invocation.Event) { r.DigitalHumanID = uuid.New() },
				func(r *Run, _ *invocation.Event) { r.ConversationID = nil },
			} {
				r, e := *run, event
				mutate(&r, &e)
				if _, err := executionAccessFor(&r, resolved, e); err == nil {
					t.Fatal("unverified background scope accepted")
				}
			}
		})
	}
}
