package executionruntime

import (
	"testing"
	"time"

	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
)

func TestIdentityFactoryMatchesPersistedConversationAndMessage(t *testing.T) {
	changes := map[string]func(*runtimeservice.PreparedChat, *execution.Scope){
		"organization": func(p *runtimeservice.PreparedChat, _ *execution.Scope) { p.Conversation.OrganizationID = uuid.New() },
		"data workspace": func(p *runtimeservice.PreparedChat, _ *execution.Scope) {
			other := uuid.New()
			p.Conversation.WorkspaceID = &other
		},
		"missing data workspace": func(p *runtimeservice.PreparedChat, _ *execution.Scope) { p.Scope.WorkspaceID = nil },
		"resource workspace": func(p *runtimeservice.PreparedChat, _ *execution.Scope) {
			other := uuid.New()
			p.Scope.ResourceWorkspaceID = &other
		},
		"billing owner":    func(p *runtimeservice.PreparedChat, _ *execution.Scope) { p.Conversation.AccountID = uuid.New() },
		"application type": func(p *runtimeservice.PreparedChat, _ *execution.Scope) { p.Conversation.CallerType = "aichat" },
		"application ID": func(p *runtimeservice.PreparedChat, _ *execution.Scope) {
			other := uuid.New()
			p.Conversation.CallerID = &other
		},
		"missing application ID": func(p *runtimeservice.PreparedChat, _ *execution.Scope) { p.Conversation.CallerID = nil },
		"entry":                  func(p *runtimeservice.PreparedChat, _ *execution.Scope) { p.Conversation.Source = "channel" },
		"conversation ID":        func(p *runtimeservice.PreparedChat, _ *execution.Scope) { p.Conversation.ID = uuid.New() },
		"deleted conversation": func(p *runtimeservice.PreparedChat, _ *execution.Scope) {
			now := time.Now()
			p.Conversation.DeletedAt = &now
		},
		"other message conversation": func(p *runtimeservice.PreparedChat, _ *execution.Scope) { p.Message.ConversationID = uuid.New() },
		"deleted message": func(p *runtimeservice.PreparedChat, _ *execution.Scope) {
			now := time.Now()
			p.Message.DeletedAt = &now
		},
		"missing message ID":   func(p *runtimeservice.PreparedChat, _ *execution.Scope) { p.Message.ID = uuid.Nil },
		"missing conversation": func(p *runtimeservice.PreparedChat, _ *execution.Scope) { p.Conversation = nil },
		"missing message":      func(p *runtimeservice.PreparedChat, _ *execution.Scope) { p.Message = nil },
		"runtime workspace credential": func(p *runtimeservice.PreparedChat, _ *execution.Scope) {
			other := uuid.New()
			p.Scope.RuntimeWorkspaceID = &other
		},
		"runtime subject credential": func(p *runtimeservice.PreparedChat, _ *execution.Scope) {
			other := uuid.New()
			p.Scope.RuntimeSubjectID = &other
		},
		"another execution user": func(_ *runtimeservice.PreparedChat, s *execution.Scope) { s.SubjectID = uuid.NewString() },
		"external user": func(p *runtimeservice.PreparedChat, _ *execution.Scope) {
			other := uuid.New()
			p.Scope.ExternalUserID = &other
		},
		"persisted external user": func(p *runtimeservice.PreparedChat, _ *execution.Scope) {
			other := uuid.New()
			p.Conversation.ExternalUserID = &other
		},
		"WebApp": func(p *runtimeservice.PreparedChat, _ *execution.Scope) {
			other := uuid.New()
			p.Conversation.SourceWebAppID = &other
		},
	}
	for _, application := range []string{"agent", "digital_human"} {
		for _, entry := range []string{"console", "webapp", "external-api"} {
			for name, change := range changes {
				t.Run(application+"/"+entry+"/"+name, func(t *testing.T) {
					prepared, scope := identityPreparedFixture(application, entry)
					if !factoryScopeMatches(scope, "", prepared) {
						t.Fatal("valid identity rejected")
					}
					change(prepared, &scope)
					if factoryScopeMatches(scope, "", prepared) {
						t.Fatal("mismatched conversation/message accepted")
					}
				})
			}
		}
	}
}

func TestIdentityFactorySeparatesAgentBillingAndResourceWorkspace(t *testing.T) {
	prepared, scope := identityPreparedFixture("agent", "external-api")
	billing, dataWorkspace := prepared.Scope.AccountID, *prepared.Scope.WorkspaceID
	resource := uuid.New()
	prepared.Scope.ResourceWorkspaceID = &resource
	scope.Identity.WorkspaceID = resource.String()
	if !factoryScopeMatches(scope, "", prepared) {
		t.Fatal("separate resource workspace was rejected")
	}
	if prepared.Scope.AccountID != billing || *prepared.Conversation.WorkspaceID != dataWorkspace {
		t.Fatal("execution validation changed billing/data ownership")
	}
	scope.SubjectID = billing.String()
	if factoryScopeMatches(scope, "", prepared) {
		t.Fatal("billing owner was substituted for the external user")
	}
}

func identityPreparedFixture(application, entry string) (*runtimeservice.PreparedChat, execution.Scope) {
	org, workspace, app, user, conversation := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	prepared := &runtimeservice.PreparedChat{
		Scope:        runtimeservice.Scope{OrganizationID: org, AccountID: user, WorkspaceID: &workspace},
		Caller:       runtimeservice.Caller{Type: application, ID: &app, Source: entry},
		Conversation: &runtimemodel.Conversation{ID: conversation, OrganizationID: org, WorkspaceID: &workspace, AccountID: user, CallerType: application, CallerID: &app, Source: entry},
		Message:      &runtimemodel.Message{ID: uuid.New(), ConversationID: conversation},
	}
	kind := "account"
	if entry == "external-api" {
		kind = "api_end_user"
	}
	if entry == "external-api" && application == "agent" {
		prepared.Scope.AccountID = uuid.New()
		prepared.Conversation.AccountID = prepared.Scope.AccountID
		prepared.Scope.ExternalUserID, prepared.Conversation.ExternalUserID = &user, &user
	}
	if entry == "webapp" {
		webApp := uuid.New()
		prepared.Caller.SourceWebAppID, prepared.Conversation.SourceWebAppID = &webApp, &webApp
	}
	scope := execution.Scope{OrganizationID: org.String(), SubjectID: user.String(), Identity: &execution.Identity{Version: execution.IdentityVersion,
		WorkspaceID: workspace.String(), ApplicationType: application, ApplicationID: app.String(), Entry: entry, SubjectKind: kind, ConversationID: conversation.String(), PublicationRef: "published-v1"}}
	return prepared, scope
}
