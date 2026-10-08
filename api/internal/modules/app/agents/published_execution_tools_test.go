package agents

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	executionruntime "github.com/zgiai/zgi/api/internal/capabilities/agentexecution/runtime"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type publishedExecutionTestService struct {
	*stubWebAppStatusHandlerService
	credentialRevoked                       bool
	keyID, application, credentialWorkspace uuid.UUID
}

func (s *publishedExecutionTestService) GetPublishedAgentRuntimeConfig(context.Context, string) (*dto.AgentWebAppRuntimeConfigResponse, error) {
	return s.publishedConfigResp, s.err
}

func (s *publishedExecutionTestService) authorizePublishedExecutionAPI(_ context.Context, application, keyID, workspace uuid.UUID) error {
	if s.credentialRevoked || application != s.application || keyID != s.keyID || workspace != s.credentialWorkspace {
		return execution.ErrInvalidIdentity
	}
	return nil
}

type publishedExecutionTestGateway struct {
	executionruntime.Gateway
	creates int
	scope   execution.Scope
}

func (g *publishedExecutionTestGateway) CreateBinding(_ context.Context, scope execution.Scope, _, _, _, _ string, _ ...string) (executionruntime.Binding, error) {
	g.creates++
	g.scope = scope
	// Other gateway calls must not occur after this deliberate test failure.
	return executionruntime.Binding{}, errors.New("test binding unavailable")
}

func TestPublishedAgentAdapterRequiresReadinessAndLivePublication(t *testing.T) {
	previous := tool_file.GlobalToolFileManager
	tool_file.GlobalToolFileManager = &tool_file.ToolFileManager{}
	t.Cleanup(func() { tool_file.GlobalToolFileManager = previous })
	for _, entry := range []string{"webapp", "external-api"} {
		t.Run(entry, func(t *testing.T) {
			p, published := publishedExecutionFixture(entry)
			t.Setenv("ZGI_AGENT_RUNTIME_ENTRY_ALLOWLIST_JSON", `{"agent:`+entry+`":["`+p.Scope.OrganizationID.String()+`"]}`)
			service := &publishedExecutionTestService{stubWebAppStatusHandlerService: &stubWebAppStatusHandlerService{
				publishedConfigResp: published, runtimeCapabilityResp: &dto.AgentWebAppRuntimeCapabilityResponse{Allowed: true}},
				keyID: uuid.New(), application: *p.Caller.ID, credentialWorkspace: *p.Scope.WorkspaceID}
			gateway := &publishedExecutionTestGateway{}
			h := &AgentsHandler{appService: service, fileService: &executionUploadTestFiles{}, executionGateway: gateway,
				executionApprovals: agentExecutionApprovals{Enabled: true, GatewayReady: true, Profiles: map[string]agentExecutionApproval{
					"standard": {Backend: "local-gvisor", OrganizationIDs: []string{p.Scope.OrganizationID.String()}, PolicyIDs: []string{"offline"}},
				}}}
			config := runtimeservice.RunConfig{}
			attach := func(authenticated bool) error {
				return h.attachPublishedAgentExecutionTools(t.Context(), &config, p.Scope, p.Caller, published, authenticated, service.keyID)
			}
			if err := attach(true); err != nil || config.RuntimeToolFactory != nil {
				t.Fatalf("allowlist released unfinished entry: %v", err)
			}
			h.executionEntryReady = func(string, string) bool { return true }
			if entry == "webapp" {
				if err := attach(false); err != nil || config.RuntimeToolFactory != nil {
					t.Fatal("anonymous visitor acquired Linux")
				}
			}
			if err := attach(true); err != nil {
				t.Fatal(err)
			}
			if config.RuntimeToolFactory == nil {
				t.Fatal("verified entry did not attach factory")
			}
			tools := config.RuntimeToolFactory(p)
			if len(tools) != 2 || tools[0].PlatformInteractiveApproval || tools[0].BindPlatform != nil {
				t.Fatalf("tools=%+v", tools)
			}
			call := adapter.ToolCall{ID: "call", Function: adapter.FunctionCall{Arguments: `{"command":"printf ok"}`}}
			tools[0].Handler(t.Context(), call)
			if gateway.creates != 1 || gateway.scope.Identity == nil || gateway.scope.Identity.Entry != entry {
				t.Fatal("adapter lost verified identity")
			}
			if entry == "external-api" && gateway.scope.SubjectID != p.Scope.ExternalUserID.String() {
				t.Fatal("billing owner replaced final user")
			}
			denied := func() {
				t.Helper()
				for _, tool := range tools {
					result := tool.Handler(t.Context(), call)
					if result.Result["error_code"] != "execution_denied" || gateway.creates != 1 {
						t.Fatalf("revoked entry reached worker: %+v", result)
					}
				}
			}
			version := published.VersionUUID
			published.VersionUUID = uuid.NewString()
			denied()
			published.VersionUUID = version
			published.Config.ExecutionRuntime.Enabled = false
			denied()
			published.Config.ExecutionRuntime.Enabled = true
			if entry == "webapp" {
				service.runtimeCapabilityResp.Allowed = false
				denied()
				service.runtimeCapabilityResp.Allowed = true
				if service.runtimeCapabilityAccountID != p.Scope.AccountID.String() || !service.runtimeCapabilityAuthenticated {
					t.Fatal("live authorization did not use verified login")
				}
			} else {
				service.credentialRevoked = true
				denied()
				service.credentialRevoked = false
				if err := h.authorizePublishedExecution(t.Context(), publishedAccessForTest(t, p, published), uuid.Nil); err == nil {
					t.Fatal("missing API key accepted")
				}
			}
			service.err = errors.New("authorization store unavailable")
			denied()
		})
	}
}

func publishedAccessForTest(t *testing.T, p *runtimeservice.PreparedChat, published *dto.AgentWebAppRuntimeConfigResponse) publishedExecutionAccess {
	t.Helper()
	access, err := publishedExecutionAccessFor(p.Scope, p.Caller, published, true)
	if err != nil {
		t.Fatal(err)
	}
	return access
}

func TestPublishedAdapterCannotGrantRuntimeWorkspaceExecution(t *testing.T) {
	p, published := publishedExecutionFixture("external-api")
	p.Scope.RuntimeWorkspaceID = uuidPtr(uuid.New())
	h := &AgentsHandler{executionEntryReady: func(string, string) bool { return true }}
	config := runtimeservice.RunConfig{}
	if err := h.attachPublishedAgentExecutionTools(t.Context(), &config, p.Scope, p.Caller, published, false, uuid.New()); err != nil || config.RuntimeToolFactory != nil {
		t.Fatalf("Runtime Workspace received Linux: %v", err)
	}
}
