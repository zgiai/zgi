package executionruntime

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type factoryTestGateway struct {
	*fakeAgentExecutionGateway
	agentID string
	creates int
}

func (gateway *factoryTestGateway) CreateBinding(ctx context.Context, scope agentExecutionScope, agent, conversation, profile, backend string, options ...string) (agentExecutionBinding, error) {
	gateway.agentID, gateway.creates = agent, gateway.creates+1
	return gateway.fakeAgentExecutionGateway.CreateBinding(ctx, scope, agent, conversation, profile, backend, options...)
}

func TestSharedFactoryPreservesApplicationIdentityAndBilling(t *testing.T) {
	for _, application := range []string{"agent", "digital_human"} {
		for _, entry := range []string{"console", "webapp", "external-api"} {
			t.Run(application+"/"+entry, func(t *testing.T) {
				org, app, user, billing, conversation, workspace := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
				prepared := &runtimeservice.PreparedChat{
					Scope:        runtimeservice.Scope{OrganizationID: org, AccountID: billing, WorkspaceID: &workspace},
					Caller:       runtimeservice.Caller{Type: application, ID: &app, Source: entry},
					Conversation: &runtimemodel.Conversation{ID: conversation, OrganizationID: org, WorkspaceID: &workspace, CallerType: application, CallerID: &app, Source: entry},
					Message:      &runtimemodel.Message{ID: uuid.New(), ConversationID: conversation},
				}
				kind := "account"
				if entry == "external-api" {
					kind = "api_end_user"
				}
				if application == "agent" && entry == "external-api" {
					prepared.Scope.ExternalUserID, prepared.Conversation.ExternalUserID = &user, &user
				} else {
					prepared.Scope.AccountID = user
				}
				prepared.Conversation.AccountID = prepared.Scope.AccountID
				if entry == "webapp" {
					webApp := uuid.New()
					prepared.Caller.SourceWebAppID, prepared.Conversation.SourceWebAppID = &webApp, &webApp
				}
				identity := &execution.Identity{Version: 1, WorkspaceID: workspace.String(), ApplicationType: application, ApplicationID: app.String(),
					Entry: entry, SubjectKind: kind, ConversationID: conversation.String(), PublicationRef: "published-v1"}
				scope := execution.Scope{OrganizationID: org.String(), SubjectID: user.String(), Identity: identity}
				entries, err := execution.ParseEntryAdmission(`{"` + application + `:` + entry + `":["` + org.String() + `"]}`)
				if err != nil {
					t.Fatal(err)
				}
				gateway := &factoryTestGateway{fakeAgentExecutionGateway: &fakeAgentExecutionGateway{
					binding: agentExecutionBinding{ID: "binding", Status: "active"}, job: agentExecutionJob{ID: "job", Status: "succeeded"}}}
				revoked := false
				options := FactoryOptions{Gateway: gateway, Persister: &fakeAgentExecutionArtifactPersister{}, Entries: entries,
					ProfileID: "standard", Backend: "fake", NetworkPolicy: "offline",
					ResolveScope: func(*runtimeservice.PreparedChat) (execution.Scope, error) { return scope, nil },
					ResolveFiles: func(p *runtimeservice.PreparedChat, s execution.Scope) (*ScopedAttachmentReader, error) {
						return NewScopedAttachmentReader(&inputTestSource{}, s, p.RuntimeAttachmentRefs(), allowInputTestFile)
					},
					Authorize: func(context.Context) error {
						if revoked {
							return errors.New("access revoked")
						}
						return nil
					},
				}
				factory := NewToolFactory(options)
				tools := factory(prepared)
				if len(tools) != 2 || tools[0].PlatformInteractiveApproval {
					t.Fatalf("shared tools = %+v", tools)
				}
				call := adapter.ToolCall{ID: "call", Function: adapter.FunctionCall{Arguments: `{"command":"printf ok"}`}}
				if result := tools[0].Handler(t.Context(), call); result.Error != nil {
					t.Fatal(result.Error)
				}
				if gateway.scope.SubjectID != user.String() || gateway.scope.Identity == nil || *gateway.scope.Identity != *identity {
					t.Fatal("verified subject or application was lost at gateway")
				}
				if application == "digital_human" && gateway.agentID != "" {
					t.Fatal("DigitalHuman impersonated Agent")
				}
				if application == "agent" && entry == "external-api" && prepared.Scope.AccountID != billing {
					t.Fatal("execution changed the existing billing account")
				}
				resolver := options.ResolveFiles
				options.ResolveFiles = nil
				if len(NewToolFactory(options)(prepared)) != 0 {
					t.Fatal("identity entry used an unscoped input reader")
				}
				options.ResolveFiles = func(*runtimeservice.PreparedChat, execution.Scope) (*ScopedAttachmentReader, error) { return nil, nil }
				if len(NewToolFactory(options)(prepared)) != 0 {
					t.Fatal("missing scoped reader fell back to raw file service")
				}
				options.ResolveFiles = func(p *runtimeservice.PreparedChat, s execution.Scope) (*ScopedAttachmentReader, error) {
					s.SubjectID = uuid.NewString()
					return NewScopedAttachmentReader(&inputTestSource{}, s, p.RuntimeAttachmentRefs(), allowInputTestFile)
				}
				if len(NewToolFactory(options)(prepared)) != 0 {
					t.Fatal("factory accepted another user's reader")
				}
				options.ResolveFiles = resolver
				authorize := options.Authorize
				options.Authorize = nil
				if len(NewToolFactory(options)(prepared)) != 0 {
					t.Fatal("identity entry omitted live application authorization")
				}
				options.Authorize = authorize
				revoked = true
				before := gateway.creates
				if result := tools[0].Handler(t.Context(), call); result.Result["error_code"] != "execution_denied" || gateway.creates != before {
					t.Fatal("revoked user reached gateway")
				}
				revoked = false
				messageID := prepared.Message.ID
				prepared.Message.ID = uuid.New()
				if result := tools[0].Handler(t.Context(), call); result.Result["error_code"] != "execution_denied" || gateway.creates != before {
					t.Fatal("tool invocation switched its frozen message")
				}
				prepared.Message.ID = messageID
				conversationOwner := prepared.Conversation.AccountID
				prepared.Conversation.AccountID = uuid.New()
				if result := tools[0].Handler(t.Context(), call); result.Result["error_code"] != "execution_denied" || gateway.creates != before {
					t.Fatal("conversation owner change reached gateway")
				}
				prepared.Conversation.AccountID = conversationOwner
				identity.PublicationRef = "published-v2"
				if result := tools[0].Handler(t.Context(), call); result.Result["error_code"] != "execution_denied" {
					t.Fatal("invocation silently changed frozen publication")
				}
				options.Persister = nil
				if got := NewToolFactory(options)(prepared); len(got) != 0 {
					t.Fatal("identity-scoped entry received public signed artifact persistence")
				}
				options.Entries = execution.EntryAdmission{}
				options.Persister = &fakeAgentExecutionArtifactPersister{}
				if entry != "console" || application != "agent" {
					if got := NewToolFactory(options)(prepared); len(got) != 0 {
						t.Fatal("new entry bypassed default allowlist")
					}
				}
			})
		}
	}
}

func TestSharedFactoryCannotAdoptLegacyConsoleBindingFromOtherEntry(t *testing.T) {
	org, user, app := uuid.New(), uuid.New(), uuid.New()
	prepared := &runtimeservice.PreparedChat{Scope: runtimeservice.Scope{OrganizationID: org, AccountID: user},
		Caller:       runtimeservice.Caller{Type: "agent", ID: &app, Source: "external-api"},
		Conversation: &runtimemodel.Conversation{ID: uuid.New()}, Message: &runtimemodel.Message{ID: uuid.New()}}
	entries, err := execution.ParseEntryAdmission(`{"agent:external-api":["` + org.String() + `"]}`)
	if err != nil {
		t.Fatal(err)
	}
	factory := NewToolFactory(FactoryOptions{Gateway: &fakeAgentExecutionGateway{}, LegacyAgentID: app.String(), Entries: entries,
		ResolveScope: func(*runtimeservice.PreparedChat) (execution.Scope, error) {
			return execution.Scope{OrganizationID: org.String(), SubjectID: user.String()}, nil
		}})
	if tools := factory(prepared); len(tools) != 0 {
		t.Fatal("external request acquired legacy console identity")
	}
}
