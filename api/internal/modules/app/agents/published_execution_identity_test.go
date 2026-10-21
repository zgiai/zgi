package agents

import (
	"testing"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
)

func publishedExecutionFixture(entry string) (*runtimeservice.PreparedChat, *dto.AgentWebAppRuntimeConfigResponse) {
	org, workspace, app, account, conversation, webApp := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	p := &runtimeservice.PreparedChat{
		Scope:        runtimeservice.Scope{OrganizationID: org, AccountID: account, WorkspaceID: &workspace},
		Caller:       runtimeservice.Caller{Type: "agent", ID: &app, Source: entry},
		Conversation: &runtimemodel.Conversation{ID: conversation, OrganizationID: org, AccountID: account, WorkspaceID: &workspace, CallerType: "agent", CallerID: &app, Source: entry},
		Message:      &runtimemodel.Message{ID: uuid.New(), ConversationID: conversation},
	}
	if entry == "webapp" {
		p.Caller.SourceWebAppID, p.Conversation.SourceWebAppID = &webApp, &webApp
	}
	if entry == "external-api" {
		user := uuid.New()
		p.Scope.ExternalUserID, p.Conversation.ExternalUserID = &user, &user
	}
	published := &dto.AgentWebAppRuntimeConfigResponse{AgentType: "AGENT", AgentID: app.String(), WebAppID: webApp.String(), OrganizationID: org.String(), WorkspaceID: workspace.String(), VersionUUID: uuid.NewString(),
		Config: dto.AgentConfigResponse{ExecutionRuntime: &dto.AgentExecutionRuntime{Enabled: true}}}
	return p, published
}

func TestPublishedExecutionIdentityFreezesOwnerAndPublication(t *testing.T) {
	for _, entry := range []string{"webapp", "external-api"} {
		t.Run(entry, func(t *testing.T) {
			p, published := publishedExecutionFixture(entry)
			// API data workspace and published resource workspace are separate concepts.
			if entry == "external-api" {
				resource := uuid.New()
				p.Scope.ResourceWorkspaceID, published.WorkspaceID = &resource, resource.String()
			}
			access, err := publishedExecutionAccessFor(p.Scope, p.Caller, published, true)
			if err != nil {
				t.Fatal(err)
			}
			owner := p.Scope.AccountID
			if entry == "external-api" {
				owner = *p.Scope.ExternalUserID
			}
			version := published.VersionUUID
			published.VersionUUID = uuid.NewString()
			scope, err := access.resolve(p)
			if err != nil || scope.SubjectID != owner.String() || scope.Identity.PublicationRef != version || scope.Identity.WorkspaceID != published.WorkspaceID {
				t.Fatalf("frozen scope=%+v err=%v", scope, err)
			}
			if entry == "external-api" && scope.SubjectID == p.Scope.AccountID.String() {
				t.Fatal("billing account became final user")
			}
			*p.Caller.ID = uuid.New()
			if _, err := access.resolve(p); err == nil {
				t.Fatal("request-owned caller pointer changed frozen owner")
			}
		})
	}
}

func TestPublishedExecutionRejectsUnverifiedIdentity(t *testing.T) {
	tests := map[string]func(*runtimeservice.PreparedChat, *dto.AgentWebAppRuntimeConfigResponse){
		"organization": func(p *runtimeservice.PreparedChat, _ *dto.AgentWebAppRuntimeConfigResponse) {
			p.Scope.OrganizationID = uuid.Nil
		},
		"workspace": func(p *runtimeservice.PreparedChat, _ *dto.AgentWebAppRuntimeConfigResponse) {
			p.Scope.WorkspaceID = nil
		},
		"resource workspace": func(p *runtimeservice.PreparedChat, _ *dto.AgentWebAppRuntimeConfigResponse) {
			p.Scope.ResourceWorkspaceID = new(uuid.UUID)
		},
		"runtime workspace": func(p *runtimeservice.PreparedChat, _ *dto.AgentWebAppRuntimeConfigResponse) {
			p.Scope.RuntimeWorkspaceID = uuidPtr(uuid.New())
		},
		"runtime subject": func(p *runtimeservice.PreparedChat, _ *dto.AgentWebAppRuntimeConfigResponse) {
			p.Scope.RuntimeSubjectID = uuidPtr(uuid.New())
		},
		"application": func(_ *runtimeservice.PreparedChat, c *dto.AgentWebAppRuntimeConfigResponse) {
			c.AgentID = uuid.NewString()
		},
		"organization mismatch": func(_ *runtimeservice.PreparedChat, c *dto.AgentWebAppRuntimeConfigResponse) {
			c.OrganizationID = uuid.NewString()
		},
		"publication": func(_ *runtimeservice.PreparedChat, c *dto.AgentWebAppRuntimeConfigResponse) { c.VersionUUID = "v1" },
		"non agent": func(_ *runtimeservice.PreparedChat, c *dto.AgentWebAppRuntimeConfigResponse) {
			c.AgentType = "digital_human"
		},
		"unsupported entry": func(p *runtimeservice.PreparedChat, _ *dto.AgentWebAppRuntimeConfigResponse) {
			p.Caller.Source = "console"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			p, published := publishedExecutionFixture("webapp")
			mutate(p, published)
			if _, err := publishedExecutionAccessFor(p.Scope, p.Caller, published, true); err == nil {
				t.Fatal("invalid execution identity accepted")
			}
		})
	}
	p, published := publishedExecutionFixture("webapp")
	if _, err := publishedExecutionAccessFor(p.Scope, p.Caller, published, false); err == nil {
		t.Fatal("anonymous execution accepted")
	}
	p.Scope.ExternalUserID = uuidPtr(uuid.New())
	if _, err := publishedExecutionAccessFor(p.Scope, p.Caller, published, true); err == nil {
		t.Fatal("conflicting WebApp identity accepted")
	}
	p, published = publishedExecutionFixture("external-api")
	p.Scope.ExternalUserID = nil
	if _, err := publishedExecutionAccessFor(p.Scope, p.Caller, published, false); err == nil {
		t.Fatal("API fell back to credential owner")
	}
}

func TestPublishedExecutionPreparedConversationCannotChangeOwner(t *testing.T) {
	tests := map[string]func(*runtimeservice.PreparedChat){
		"organization":           func(p *runtimeservice.PreparedChat) { p.Conversation.OrganizationID = uuid.New() },
		"application":            func(p *runtimeservice.PreparedChat) { p.Conversation.CallerID = uuidPtr(uuid.New()) },
		"billing":                func(p *runtimeservice.PreparedChat) { p.Scope.AccountID = uuid.New() },
		"conversation owner":     func(p *runtimeservice.PreparedChat) { p.Conversation.AccountID = uuid.New() },
		"conversation workspace": func(p *runtimeservice.PreparedChat) { p.Conversation.WorkspaceID = uuidPtr(uuid.New()) },
		"entry":                  func(p *runtimeservice.PreparedChat) { p.Conversation.Source = "console" },
		"deleted":                func(p *runtimeservice.PreparedChat) { now := time.Now(); p.Conversation.DeletedAt = &now },
		"message":                func(p *runtimeservice.PreparedChat) { p.Message.ConversationID = uuid.New() },
		"deleted message":        func(p *runtimeservice.PreparedChat) { now := time.Now(); p.Message.DeletedAt = &now },
		"runtime subject":        func(p *runtimeservice.PreparedChat) { p.Scope.RuntimeSubjectID = uuidPtr(uuid.New()) },
	}
	for _, entry := range []string{"webapp", "external-api"} {
		for name, mutate := range tests {
			t.Run(entry+"/"+name, func(t *testing.T) {
				p, published := publishedExecutionFixture(entry)
				access, err := publishedExecutionAccessFor(p.Scope, p.Caller, published, true)
				if err != nil {
					t.Fatal(err)
				}
				mutate(p)
				if _, err := access.resolve(p); err == nil {
					t.Fatal("changed prepared owner accepted")
				}
			})
		}
	}
}
