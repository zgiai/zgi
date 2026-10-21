package digitalhuman

import (
	"testing"
	"time"

	"github.com/google/uuid"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
)

func TestDigitalHumanLinuxRejectsMissingConflictingOrBackgroundIdentity(t *testing.T) {
	cases := map[string]func(*Run, *ResolvedExecution, *invocation.Event){
		"missing user": func(r *Run, _ *ResolvedExecution, _ *invocation.Event) { r.UserSubjectID = nil },
		"empty user":   func(r *Run, _ *ResolvedExecution, _ *invocation.Event) { r.UserSubjectID = new(uuid.UUID) },
		"credential fallback": func(r *Run, _ *ResolvedExecution, _ *invocation.Event) {
			r.UserSubjectID = cloneUUIDPointer(r.PrincipalID)
			r.UserSubjectKind = ""
		},
		"sender mismatch": func(_ *Run, _ *ResolvedExecution, e *invocation.Event) { e.Sender.ID = uuid.NewString() },
		"principal class": func(r *Run, _ *ResolvedExecution, _ *invocation.Event) {
			r.PrincipalType = string(invocation.PrincipalSystem)
		},
		"snapshot application": func(_ *Run, s *ResolvedExecution, _ *invocation.Event) { s.Snapshot.Subject.ID = uuid.New() },
		"snapshot publication": func(_ *Run, s *ResolvedExecution, _ *invocation.Event) { s.Snapshot.DigitalHumanVersion = uuid.New() },
		"snapshot revision":    func(_ *Run, s *ResolvedExecution, _ *invocation.Event) { s.Snapshot.Revision = "" },
		"event workspace":      func(_ *Run, _ *ResolvedExecution, e *invocation.Event) { e.Target.WorkspaceID = uuid.New() },
		"event organization":   func(_ *Run, _ *ResolvedExecution, e *invocation.Event) { e.Target.OrganizationID = uuid.New() },
		"event application":    func(_ *Run, _ *ResolvedExecution, e *invocation.Event) { e.Target.ID = uuid.New() },
		"event surface":        func(_ *Run, _ *ResolvedExecution, e *invocation.Event) { delete(e.Context, "surface") },
		"run spec":             func(_ *Run, s *ResolvedExecution, _ *invocation.Event) { s.Spec.InvocationID = uuid.NewString() },
		"scheduled":            func(_ *Run, _ *ResolvedExecution, e *invocation.Event) { e.Origin.Kind = invocation.OriginSchedule },
		"channel":              func(_ *Run, _ *ResolvedExecution, e *invocation.Event) { e.Origin.Kind = invocation.OriginChannel },
	}
	for _, surface := range []string{SurfaceConsole, SurfaceWebApp, SurfaceAPI} {
		for name, mutate := range cases {
			t.Run(surface+"/"+name, func(t *testing.T) {
				r, s, e, _ := linuxExecutionFixture(t, surface)
				mutate(r, s, &e)
				if _, err := executionAccessFor(r, s, e); err == nil {
					t.Fatal("unverified identity accepted")
				}
			})
		}
		t.Run(surface+"/actor", func(t *testing.T) {
			r, s, e, _ := linuxExecutionFixture(t, surface)
			other := uuid.New()
			r.ActorID = &other
			if _, err := executionAccessFor(r, s, e); err == nil {
				t.Fatal("conflicting actor accepted")
			}
		})
	}
}

func TestDigitalHumanLinuxBindsPreparedConversationAndRun(t *testing.T) {
	cases := map[string]func(*chatruntime.PreparedChat){
		"subject":                   func(p *chatruntime.PreparedChat) { p.Scope.AccountID = uuid.New() },
		"workspace":                 func(p *chatruntime.PreparedChat) { p.Scope.WorkspaceID = nil },
		"resource workspace":        func(p *chatruntime.PreparedChat) { p.Scope.ResourceWorkspaceID = new(uuid.UUID) },
		"organization":              func(p *chatruntime.PreparedChat) { p.Scope.OrganizationID = uuid.New() },
		"external subject":          func(p *chatruntime.PreparedChat) { p.Scope.ExternalUserID = new(uuid.UUID) },
		"runtime workspace":         func(p *chatruntime.PreparedChat) { p.Scope.RuntimeWorkspaceID = new(uuid.UUID) },
		"runtime subject":           func(p *chatruntime.PreparedChat) { p.Scope.RuntimeSubjectID = new(uuid.UUID) },
		"Agent impersonation":       func(p *chatruntime.PreparedChat) { p.Caller.Type = "agent" },
		"missing run":               func(p *chatruntime.PreparedChat) { p.Caller.RunID = nil },
		"conversation owner":        func(p *chatruntime.PreparedChat) { p.Conversation.AccountID = uuid.New() },
		"conversation organization": func(p *chatruntime.PreparedChat) { p.Conversation.OrganizationID = uuid.New() },
		"conversation source":       func(p *chatruntime.PreparedChat) { p.Conversation.Source = "channel" },
		"conversation deleted":      func(p *chatruntime.PreparedChat) { now := time.Now(); p.Conversation.DeletedAt = &now },
		"message conversation":      func(p *chatruntime.PreparedChat) { p.Message.ConversationID = uuid.New() },
		"message deleted":           func(p *chatruntime.PreparedChat) { now := time.Now(); p.Message.DeletedAt = &now },
	}
	for _, surface := range []string{SurfaceConsole, SurfaceWebApp, SurfaceAPI} {
		for name, mutate := range cases {
			t.Run(surface+"/"+name, func(t *testing.T) {
				r, s, e, p := linuxExecutionFixture(t, surface)
				a, err := executionAccessFor(r, s, e)
				if err != nil {
					t.Fatal(err)
				}
				mutate(p)
				if _, err := a.resolve(p); err == nil {
					t.Fatal("foreign prepared conversation accepted")
				}
			})
		}
	}
}

func TestDigitalHumanLinuxWebAppAndNewConversationBoundaries(t *testing.T) {
	for _, raw := range []any{nil, "", "not-a-uuid", uuid.Nil.String(), 42} {
		r, s, e, _ := linuxExecutionFixture(t, SurfaceWebApp)
		e.Context["web_app_id"] = raw
		if _, err := executionAccessFor(r, s, e); err == nil {
			t.Fatalf("accepted invalid WebApp identity: %v", raw)
		}
	}
	for _, surface := range []string{SurfaceConsole, SurfaceWebApp, SurfaceAPI} {
		r, s, e, p := linuxExecutionFixture(t, surface)
		r.ConversationID = nil // First turn: ChatRuntime has not allocated it yet.
		a, err := executionAccessFor(r, s, e)
		if err != nil {
			t.Fatal(err)
		}
		if scope, err := a.resolve(p); err != nil || scope.Identity.ConversationID != p.Conversation.ID.String() {
			t.Fatalf("new conversation failed: %v", err)
		}
		if surface == SurfaceWebApp {
			other := uuid.New()
			p.Caller.SourceWebAppID = &other
			if _, err := a.resolve(p); err == nil {
				t.Fatal("different WebApp accepted")
			}
		} else {
			e.Context["web_app_id"] = uuid.NewString()
			if _, err := executionAccessFor(r, s, e); err == nil {
				t.Fatal("non-WebApp invocation accepted a WebApp identity")
			}
		}
	}
}
