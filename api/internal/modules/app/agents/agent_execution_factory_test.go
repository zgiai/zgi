package agents

import (
	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	"testing"
)

type consoleFactoryTestGateway struct{ agentExecutionGateway }

func TestConsoleAgentExecutionToolFactoryIsCallerScoped(t *testing.T) {
	agentID := uuid.New()
	orgID := uuid.New()
	accountID := uuid.New()
	h := &AgentsHandler{
		executionApprovals: agentExecutionApprovals{Enabled: true, GatewayReady: true, Profiles: map[string]agentExecutionApproval{
			"standard": {Backend: "local-gvisor", OrganizationIDs: []string{orgID.String()}, PolicyIDs: []string{"offline"}},
		}},
		executionGateway: &consoleFactoryTestGateway{},
	}
	scope := runtimeservice.Scope{OrganizationID: orgID, AccountID: accountID}
	cfg := dto.AgentConfigResponse{ExecutionRuntime: &dto.AgentExecutionRuntime{Enabled: true, ProfileID: "standard", PolicyID: "offline"}}
	runConfig := runtimeservice.RunConfig{}
	if err := h.attachConsoleAgentExecutionTools(&runConfig, scope, agentID, cfg); err != nil {
		t.Fatalf("attach console tools: %v", err)
	}
	prepared := &runtimeservice.PreparedChat{
		Scope:        scope,
		Caller:       runtimeservice.Caller{Type: runtimemodel.ConversationCallerAgent, ID: &agentID, Source: runtimemodel.ConversationSourceConsole},
		Conversation: &runtimemodel.Conversation{ID: uuid.New()}, Message: &runtimemodel.Message{ID: uuid.New()},
	}
	if got := runConfig.RuntimeToolFactory(prepared); len(got) != 2 || got[0].Definition.Function.Name != "shell_exec" || got[1].Definition.Function.Name != "workspace_export_file" {
		t.Fatalf("console tools = %#v", got)
	}
	prepared.Caller.Source = runtimemodel.ConversationSourceWebApp
	if got := runConfig.RuntimeToolFactory(prepared); len(got) != 0 {
		t.Fatalf("WebApp received console tools: %#v", got)
	}
	prepared.Caller.Source = runtimemodel.ConversationSourceConsole
	prepared.Scope.AccountID = uuid.New()
	if got := runConfig.RuntimeToolFactory(prepared); len(got) != 0 {
		t.Fatalf("different account received console tools: %#v", got)
	}
}

func TestConsoleAgentExecutionDependencyToolRequiresProfileApproval(t *testing.T) {
	agentID := uuid.New()
	orgID := uuid.New()
	accountID := uuid.New()
	h := &AgentsHandler{
		executionApprovals: agentExecutionApprovals{Enabled: true, GatewayReady: true, Profiles: map[string]agentExecutionApproval{
			"standard": {Backend: "local-gvisor", OrganizationIDs: []string{orgID.String()}, PolicyIDs: []string{"offline"}, DependencyPreparation: true},
		}},
		executionGateway: &consoleFactoryTestGateway{},
	}
	runConfig := runtimeservice.RunConfig{}
	if err := h.attachConsoleAgentExecutionTools(&runConfig, runtimeservice.Scope{OrganizationID: orgID, AccountID: accountID}, agentID, dto.AgentConfigResponse{ExecutionRuntime: &dto.AgentExecutionRuntime{Enabled: true, ProfileID: "standard", PolicyID: "offline"}}); err != nil {
		t.Fatalf("attach console tools: %v", err)
	}
	prepared := &runtimeservice.PreparedChat{
		Scope:        runtimeservice.Scope{OrganizationID: orgID, AccountID: accountID},
		Caller:       runtimeservice.Caller{Type: runtimemodel.ConversationCallerAgent, ID: &agentID, Source: runtimemodel.ConversationSourceConsole},
		Conversation: &runtimemodel.Conversation{ID: uuid.New()}, Message: &runtimemodel.Message{ID: uuid.New()},
	}
	tools := runConfig.RuntimeToolFactory(prepared)
	if len(tools) != 3 || tools[2].Definition.Function.Name != "workspace_prepare_dependencies" {
		t.Fatalf("approved console tools = %#v", tools)
	}

	h.executionApprovals.Profiles["standard"] = agentExecutionApproval{Backend: "local-gvisor", OrganizationIDs: []string{orgID.String()}, PolicyIDs: []string{"offline"}}
	runConfig = runtimeservice.RunConfig{}
	if err := h.attachConsoleAgentExecutionTools(&runConfig, runtimeservice.Scope{OrganizationID: orgID, AccountID: accountID}, agentID, dto.AgentConfigResponse{ExecutionRuntime: &dto.AgentExecutionRuntime{Enabled: true, ProfileID: "standard", PolicyID: "offline"}}); err != nil {
		t.Fatalf("attach console tools without dependency approval: %v", err)
	}
	if tools := runConfig.RuntimeToolFactory(prepared); len(tools) != 2 {
		t.Fatalf("unapproved dependency tool was exposed: %#v", tools)
	}

	for _, backend := range []string{"local-gvisor", "e2b"} {
		t.Run(backend+"-standard-installation", func(t *testing.T) {
			h.executionApprovals.Profiles["standard"] = agentExecutionApproval{
				Backend: backend, OrganizationIDs: []string{orgID.String()}, PolicyIDs: []string{"package-install"},
				DependencyPreparation: true, DependencyInstallation: true, PackageNetworkMode: "package-install",
			}
			runConfig := runtimeservice.RunConfig{}
			if err := h.attachConsoleAgentExecutionTools(&runConfig, prepared.Scope, agentID, dto.AgentConfigResponse{ExecutionRuntime: &dto.AgentExecutionRuntime{Enabled: true}}); err != nil {
				t.Fatal(err)
			}
			tools := runConfig.RuntimeToolFactory(prepared)
			if len(tools) != 2 || tools[0].Definition.Function.Name != "shell_exec" || tools[1].Definition.Function.Name != "workspace_export_file" {
				t.Fatalf("standard installation tools = %#v, want shell and export only", tools)
			}
		})
	}
}
