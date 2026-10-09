package digitalhuman

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	executionruntime "github.com/zgiai/zgi/api/internal/capabilities/agentexecution/runtime"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/capabilities/runspec"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func linuxExecutionFixture(t *testing.T, surface string) (*Run, *ResolvedExecution, invocation.Event, *chatruntime.PreparedChat) {
	t.Helper()
	scope := testScope()
	principal, user, app, conversation := scope.ActorID, scope.ActorID, uuid.New(), uuid.New()
	event := invocation.Event{ID: uuid.NewString(), Type: invocation.EventTypeManual, Origin: invocation.Origin{Kind: invocation.OriginManual},
		Target: runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman, ID: app, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID},
		Sender: invocation.PrincipalRef{Type: invocation.PrincipalAccount, ID: principal.String()}, OccurredAt: time.Now().UTC(), Context: map[string]any{"surface": surface}}
	kind := "account"
	var actor *uuid.UUID = &user
	if surface != SurfaceConsole {
		event.Type, event.Origin.Kind = invocation.EventTypeAPI, invocation.OriginAPI
	}
	if surface == SurfaceWebApp {
		event.Context["web_app_id"] = uuid.NewString()
	}
	if surface == SurfaceAPI {
		principal, kind, actor = uuid.New(), "api_end_user", nil
		event.Sender = invocation.PrincipalRef{Type: invocation.PrincipalService, ID: principal.String()}
	}
	var err error
	event, err = invocation.ValidateEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := (fakeExecutionResolver{now: event.OccurredAt}).Resolve(t.Context(), event, nil)
	if err != nil {
		t.Fatal(err)
	}
	resolved.Snapshot.ExecutionRuntime = &ExecutionRuntimeConfig{Enabled: true}
	resolved.Snapshot.Bindings = []resourcebindings.Binding{{Type: resourcebindings.BindingTypeSkill, ResourceID: uuid.NewString()}}
	resolved.Snapshot.Revision, err = executionSnapshotRevision(resolved.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	resolved.Spec.Policies = []runspec.VersionRef{{Kind: "execution_snapshot", ID: resolved.Snapshot.DigitalHumanVersion.String(), Revision: resolved.Snapshot.Revision}}
	resolved.Spec.Revision = ""
	resolved.Spec, err = runspec.Seal(resolved.Spec)
	if err != nil {
		t.Fatal(err)
	}
	eventJSON, specJSON, snapshotJSON, err := marshalRunSnapshots(event, resolved)
	if err != nil {
		t.Fatal(err)
	}
	run := &Run{ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, DigitalHumanID: app,
		DigitalHumanVersionID: resolved.Snapshot.DigitalHumanVersion, ActorID: actor, PrincipalID: &principal, PrincipalType: string(event.Sender.Type),
		UserSubjectKind: kind, UserSubjectID: &user, Surface: surface, ConversationID: &conversation, IdempotencyKey: event.IdempotencyKey,
		InvocationEvent: eventJSON, RunSpec: specJSON, RunSpecRevision: resolved.Spec.Revision, ExecutionSnapshot: snapshotJSON, ExecutionSnapshotRevision: resolved.Snapshot.Revision}
	p := &chatruntime.PreparedChat{Scope: chatruntime.Scope{OrganizationID: scope.OrganizationID, WorkspaceID: &scope.WorkspaceID, AccountID: user},
		Caller: chatruntime.Caller{Type: runtimemodel.ConversationCallerDigitalHuman, ID: &app, RunID: &run.ID, Source: digitalHumanConversationSourceForSurface(surface)},
		Conversation: &runtimemodel.Conversation{ID: conversation, OrganizationID: scope.OrganizationID, WorkspaceID: &scope.WorkspaceID, AccountID: user,
			CallerType: runtimemodel.ConversationCallerDigitalHuman, CallerID: &app, Source: digitalHumanConversationSourceForSurface(surface)},
		Message: &runtimemodel.Message{ID: uuid.New(), ConversationID: conversation}}
	if surface == SurfaceWebApp {
		webApp, _ := uuid.Parse(event.Context["web_app_id"].(string))
		p.Caller.SourceWebAppID, p.Conversation.SourceWebAppID = &webApp, &webApp
	}
	if _, _, err := decodeAndVerifyRun(run); err != nil {
		t.Fatal(err)
	}
	return run, resolved, event, p
}

type linuxExecutionTestAccess struct {
	err                               error
	subject, application, publication uuid.UUID
	uploads                           int
	principal                         uuid.UUID
}

func (a *linuxExecutionTestAccess) AuthorizeLinuxExecution(_ context.Context, scope Scope, application, publication uuid.UUID) error {
	if scope.UserSubjectID == nil || *scope.UserSubjectID != a.subject || scope.ActorID != a.subject || application != a.application || publication != a.publication {
		return execution.ErrInvalidIdentity
	}
	a.principal = scope.PrincipalID
	return a.err
}

func (a *linuxExecutionTestAccess) AuthorizeAttachmentAccess(ctx context.Context, scope Scope, application, publication uuid.UUID) error {
	return a.AuthorizeLinuxExecution(ctx, scope, application, publication)
}

func (a *linuxExecutionTestAccess) AuthorizeLinuxContinuation(ctx context.Context, scope Scope, application, publication uuid.UUID) error {
	return a.AuthorizeLinuxExecution(ctx, scope, application, publication)
}

func TestDigitalHumanAttachmentReaderIndependentOfLinuxAndBoundToRun(t *testing.T) {
	for _, surface := range []string{SurfaceConsole, SurfaceWebApp, SurfaceAPI} {
		t.Run(surface, func(t *testing.T) {
			run, resolved, event, prepared := linuxExecutionFixture(t, surface)
			options, access, gateway := linuxExecutionOptionsFor(t, run)
			options.Approvals.Enabled = false
			options.Gateway = nil
			provider := NewLinuxExecutionRuntime(options)
			authorize := provider.AttachmentReadAuthorizer(run, resolved, event, nil)
			if authorize == nil {
				t.Fatal("reader requires Linux admission or Worker")
			}
			workspace := run.WorkspaceID.String()
			file := &dto.UploadFile{ID: uuid.NewString(), Name: "source.txt", IsTemporary: true,
				CreatedBy: prepared.Scope.AccountID.String(), CreatedByRole: dto.CreatedByRoleAccount, WorkspaceID: &workspace}
			if err := authorize(t.Context(), prepared, file); err != nil {
				t.Fatal(err)
			}
			prepared.Caller.ID = new(uuid.UUID)
			if err := authorize(t.Context(), prepared, file); err == nil {
				t.Fatal("different application accepted")
			}
			prepared.Caller.ID = &run.DigitalHumanID
			access.err = errors.New("access revoked")
			if err := authorize(t.Context(), prepared, file); err == nil {
				t.Fatal("revoked read accepted")
			}
			if gateway.creates != 0 {
				t.Fatal("file read allocated a Linux environment")
			}
		})
	}
}

func TestDigitalHumanContinuationCredentialRotationKeepsOriginalUserAndPublication(t *testing.T) {
	run, resolved, event, prepared := linuxExecutionFixture(t, SurfaceAPI)
	options, access, gateway := linuxExecutionOptionsFor(t, run)
	provider := NewLinuxExecutionRuntime(options)
	owner, err := executionAccessFor(run, resolved, event)
	if err != nil {
		t.Fatal(err)
	}
	current := owner.scope()
	oldCredential, replacement := *run.PrincipalID, uuid.New()
	current.PrincipalID = replacement
	factory, err := provider.PrepareContinuation(t.Context(), run, resolved, event, current)
	if err != nil || factory == nil || access.principal != replacement || *run.PrincipalID != oldCredential {
		t.Fatalf("credential rotation rewrote run or reused old authorization: %v", err)
	}
	tools := factory(prepared)
	if len(tools) == 0 {
		t.Fatal("rotated credential lost execution tools")
	}
	tools[0].Handler(t.Context(), adapter.ToolCall{ID: "call", Function: adapter.FunctionCall{Arguments: `{"command":"printf ok"}`}})
	if gateway.scope.SubjectID != owner.user.String() || gateway.scope.Identity == nil || gateway.scope.Identity.PublicationRef != run.DigitalHumanVersionID.String() {
		t.Fatal("rotated credential changed final user or publication")
	}
	other := uuid.New()
	current.UserSubjectID, current.ActorID = &other, other
	if _, err := provider.PrepareContinuation(t.Context(), run, resolved, event, current); err == nil {
		t.Fatal("rotated credential adopted another user")
	}
}

func (a *linuxExecutionTestAccess) AuthorizeExecutionUpload(context.Context, Scope, uuid.UUID, *dto.UploadFile) error {
	a.uploads++
	return a.err
}

type linuxExecutionTestFiles struct{}

func (*linuxExecutionTestFiles) GetFileByID(context.Context, string) (*dto.UploadFile, error) {
	return nil, ErrNotFound
}
func (*linuxExecutionTestFiles) DownloadFile(context.Context, string) ([]byte, error) {
	return nil, ErrNotFound
}

type linuxExecutionTestGateway struct {
	executionruntime.Gateway
	scope   execution.Scope
	agent   string
	creates int
}

func (g *linuxExecutionTestGateway) CreateBinding(_ context.Context, scope execution.Scope, agent, _, _, _ string, _ ...string) (executionruntime.Binding, error) {
	g.scope, g.agent, g.creates = scope, agent, g.creates+1
	return executionruntime.Binding{}, errors.New("test worker unavailable")
}

func linuxExecutionOptionsFor(t *testing.T, run *Run) (LinuxExecutionOptions, *linuxExecutionTestAccess, *linuxExecutionTestGateway) {
	t.Helper()
	entries, err := execution.ParseEntryAdmission(`{"digital_human:` + digitalHumanConversationSourceForSurface(run.Surface) + `":["` + run.OrganizationID.String() + `"]}`)
	if err != nil {
		t.Fatal(err)
	}
	access := &linuxExecutionTestAccess{subject: *run.UserSubjectID, application: run.DigitalHumanID, publication: run.DigitalHumanVersionID}
	gateway := &linuxExecutionTestGateway{}
	return LinuxExecutionOptions{Approvals: executionruntime.Approvals{Enabled: true, GatewayReady: true, Profiles: map[string]executionruntime.ProfileApproval{
		"standard": {Backend: "local-gvisor", OrganizationIDs: []string{run.OrganizationID.String()}, PolicyIDs: []string{"offline"}},
	}}, Gateway: gateway, Files: &linuxExecutionTestFiles{}, Access: access, Entries: entries,
		EntryReady: func(string, string) bool { return true }, AuthorizeConsole: func(context.Context, Scope) error { return nil },
		ArtifactManager: func() *tool_file.ToolFileManager { return &tool_file.ToolFileManager{} }}, access, gateway
}

func TestDigitalHumanLinuxExecutionUsesFrozenUserAndApplication(t *testing.T) {
	for _, surface := range []string{SurfaceConsole, SurfaceWebApp, SurfaceAPI} {
		t.Run(surface, func(t *testing.T) {
			run, resolved, event, p := linuxExecutionFixture(t, surface)
			options, access, gateway := linuxExecutionOptionsFor(t, run)
			ready := true
			options.EntryReady = func(application, entry string) bool {
				return ready && application == "digital_human" && entry == p.Caller.Source
			}
			provider := NewLinuxExecutionRuntime(options)
			factory, err := provider.Prepare(t.Context(), run, resolved, event)
			if err != nil || factory == nil {
				t.Fatalf("prepare: %v", err)
			}
			tools := factory(p)
			if len(tools) != 2 || tools[0].PlatformInteractiveApproval || tools[0].BindPlatform != nil {
				t.Fatalf("tools=%+v", tools)
			}
			call := adapter.ToolCall{ID: "call", Function: adapter.FunctionCall{Arguments: `{"command":"printf ok"}`}}
			tools[0].Handler(t.Context(), call)
			if gateway.creates != 1 || gateway.agent != "" || gateway.scope.SubjectID != access.subject.String() || gateway.scope.Identity == nil ||
				gateway.scope.Identity.ApplicationType != "digital_human" || gateway.scope.Identity.PublicationRef != access.publication.String() {
				t.Fatal("DigitalHuman borrowed an Agent/credential identity")
			}
			if surface == SurfaceAPI && (run.ActorID != nil || gateway.scope.SubjectID == run.PrincipalID.String()) {
				t.Fatal("API execution fell back to credential principal")
			}
			// Later caller-owned run/snapshot changes cannot mutate this factory.
			run.DigitalHumanVersionID = uuid.New()
			resolved.Snapshot.ExecutionRuntime.Enabled = false
			tools[0].Handler(t.Context(), call)
			if gateway.creates != 2 || gateway.scope.Identity.PublicationRef != access.publication.String() {
				t.Fatal("caller mutation changed frozen publication")
			}
			denied := func() {
				t.Helper()
				for _, tool := range tools {
					result := tool.Handler(t.Context(), call)
					if result.Result["error_code"] != "execution_denied" || gateway.creates != 2 {
						t.Fatal("revoked invocation reached Worker")
					}
				}
			}
			access.err = ErrNotFound
			denied()
			access.err, ready = nil, false
			denied()
			ready = true
			otherRun := uuid.New()
			p.Caller.RunID = &otherRun
			denied()
		})
	}
}

func TestDigitalHumanLinuxPreparationRequiresAllGates(t *testing.T) {
	cases := map[string]func(*LinuxExecutionOptions){
		"missing readiness":        func(o *LinuxExecutionOptions) { o.EntryReady = nil },
		"not ready":                func(o *LinuxExecutionOptions) { o.EntryReady = func(string, string) bool { return false } },
		"not admitted":             func(o *LinuxExecutionOptions) { o.Entries = execution.EntryAdmission{} },
		"gateway":                  func(o *LinuxExecutionOptions) { o.Gateway = nil },
		"file source":              func(o *LinuxExecutionOptions) { o.Files = nil },
		"access":                   func(o *LinuxExecutionOptions) { o.Access = nil },
		"persister":                func(o *LinuxExecutionOptions) { o.ArtifactManager = nil },
		"missing artifact manager": func(o *LinuxExecutionOptions) { o.ArtifactManager = func() *tool_file.ToolFileManager { return nil } },
		"deployment disabled":      func(o *LinuxExecutionOptions) { o.Approvals.Enabled = false },
		"unapproved organization":  func(o *LinuxExecutionOptions) { o.Approvals.Profiles = nil },
		"console authorizer":       func(o *LinuxExecutionOptions) { o.AuthorizeConsole = nil },
		"console revoked": func(o *LinuxExecutionOptions) {
			o.AuthorizeConsole = func(context.Context, Scope) error { return ErrNotFound }
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			run, resolved, event, _ := linuxExecutionFixture(t, SurfaceConsole)
			options, _, gateway := linuxExecutionOptionsFor(t, run)
			mutate(&options)
			factory, err := NewLinuxExecutionRuntime(options).Prepare(t.Context(), run, resolved, event)
			closed := name == "missing readiness" || name == "not ready" || name == "deployment disabled"
			if factory != nil || (err == nil) != closed || gateway.creates != 0 {
				t.Fatalf("factory=%v err=%v creates=%d", factory != nil, err, gateway.creates)
			}
		})
	}
}

func TestDigitalHumanLinuxRejectsUnverifiedBackgroundAndAnonymousRuns(t *testing.T) {
	for _, surface := range []string{SurfaceWebApp, SurfaceTask, SurfaceChannel} {
		t.Run(surface, func(t *testing.T) {
			run, resolved, event, _ := linuxExecutionFixture(t, SurfaceWebApp)
			options, _, gateway := linuxExecutionOptionsFor(t, run)
			run.Surface = surface
			run.UserSubjectID, run.UserSubjectKind, run.PrincipalType = nil, "", string(invocation.PrincipalExternalUser)
			factory, err := NewLinuxExecutionRuntime(options).Prepare(t.Context(), run, resolved, event)
			if (surface != SurfaceTask && err == nil) || factory != nil || gateway.creates != 0 {
				t.Fatalf("unsupported surface granted Linux: %v", err)
			}
		})
	}
}

func TestDigitalHumanLinuxLegacySnapshotDoesNotAcquireTools(t *testing.T) {
	for _, config := range []*ExecutionRuntimeConfig{nil, {Enabled: false}} {
		run, resolved, event, _ := linuxExecutionFixture(t, SurfaceConsole)
		options, _, gateway := linuxExecutionOptionsFor(t, run)
		resolved.Snapshot.ExecutionRuntime = config
		factory, err := NewLinuxExecutionRuntime(options).Prepare(t.Context(), run, resolved, event)
		if err != nil || factory != nil || gateway.creates != 0 {
			t.Fatalf("legacy/disabled snapshot gained tools: %v", err)
		}
	}
}
