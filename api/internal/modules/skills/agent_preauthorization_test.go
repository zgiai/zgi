package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
)

func TestPolicyToolGovernanceAgentKnowledgeBinding(t *testing.T) {
	validParams := agentGovernanceRuntimeParameters(map[string]interface{}{
		"knowledge_binding_grant":       true,
		"knowledge_bound_by_account_id": "account-1",
		"knowledge_bound_at_unix":       int64(1_720_000_000),
		"knowledge_dataset_ids":         []string{"dataset-1"},
	})

	decision := decideSystemSkillToolForTest(t, SkillAgentKnowledge, "retrieve_agent_knowledge", map[string]interface{}{"query": "policy"}, validParams)
	assertAgentGovernanceDecision(t, decision, toolgovernance.DecisionStatusAllowed, "")
	if decision.Preauthorization == nil || decision.Preauthorization.Source != agentAuthorizationSourceBinding ||
		decision.Preauthorization.AuthorizedBy != "account-1" || len(decision.Preauthorization.Resources) != 1 {
		t.Fatalf("preauthorization = %#v, want knowledge binding audit evidence", decision.Preauthorization)
	}

	missing := decideSystemSkillToolForTest(t, SkillAgentKnowledge, "retrieve_agent_knowledge", map[string]interface{}{"query": "policy"}, agentGovernanceRuntimeParameters(nil))
	assertAgentGovernanceDecision(t, missing, toolgovernance.DecisionStatusDenied, agentBindingMissingCode)

	invalid := decideSystemSkillToolForTest(t, SkillAgentKnowledge, "retrieve_agent_knowledge", map[string]interface{}{"query": "policy"}, agentGovernanceRuntimeParameters(map[string]interface{}{
		"knowledge_binding_grant": true,
		"knowledge_dataset_ids":   []string{"dataset-1"},
	}))
	assertAgentGovernanceDecision(t, invalid, toolgovernance.DecisionStatusDenied, agentBindingMissingCode)
}

func TestPolicyToolGovernanceAgentDatabaseBindings(t *testing.T) {
	params := agentGovernanceRuntimeParameters(map[string]interface{}{
		"database_binding_grant":       true,
		"database_bound_by_account_id": "account-1",
		"database_bound_at_unix":       int64(1_720_000_000),
		"database_bindings": []map[string]interface{}{{
			"data_source_id":     "database-1",
			"table_ids":          []string{"read-only-table", "writable-table"},
			"writable_table_ids": []string{"writable-table"},
		}},
	})
	tests := []struct {
		name     string
		toolName string
		tableID  string
		want     toolgovernance.DecisionStatus
		code     string
	}{
		{name: "query read-only table", toolName: "query_table_records", tableID: "read-only-table", want: toolgovernance.DecisionStatusAllowed},
		{name: "insert read-only table", toolName: "insert_table_records", tableID: "read-only-table", want: toolgovernance.DecisionStatusDenied, code: agentDatabaseTableReadOnlyCode},
		{name: "update writable table", toolName: "update_table_records", tableID: "writable-table", want: toolgovernance.DecisionStatusAllowed},
		{name: "insert writable table", toolName: "insert_table_records", tableID: "writable-table", want: toolgovernance.DecisionStatusAllowed},
		{name: "delete writable table", toolName: "delete_table_records", tableID: "writable-table", want: toolgovernance.DecisionStatusAllowed},
		{name: "query unbound table", toolName: "query_table_records", tableID: "unbound-table", want: toolgovernance.DecisionStatusDenied, code: agentResourceNotBoundCode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arguments := map[string]interface{}{
				"data_source_id": "database-1",
				"table_id":       tt.tableID,
			}
			if strings.Contains(tt.toolName, "_table_records") && tt.toolName != "query_table_records" {
				arguments["records"] = []map[string]interface{}{{"id": "record-1"}}
			}
			decision := decideSystemSkillToolForTest(t, SkillAgentDatabase, tt.toolName, arguments, params)
			assertAgentGovernanceDecision(t, decision, tt.want, tt.code)
		})
	}
	listed := decideSystemSkillToolForTest(t, SkillAgentDatabase, "list_database_tables", map[string]interface{}{
		"data_source_id": "database-1",
	}, params)
	assertAgentGovernanceDecision(t, listed, toolgovernance.DecisionStatusAllowed, "")
	if listed.Preauthorization == nil || len(listed.Preauthorization.Resources) != 2 {
		t.Fatalf("list preauthorization = %#v, want every currently bound table", listed.Preauthorization)
	}
}

func TestPolicyToolGovernanceUsesTargetBindingAuthorization(t *testing.T) {
	params := agentGovernanceRuntimeParameters(map[string]interface{}{
		"database_binding_grant": true,
		"database_bindings": []map[string]interface{}{{
			"data_source_id": "database-1",
			"table_ids":      []string{"table-old", "table-new"},
		}},
		"agent_binding_authorizations": []map[string]interface{}{
			{"binding_type": "database", "resource_id": "database-1", "access_mode": "read", "bound_by_account_id": "binder-old", "bound_at_unix": int64(100)},
			{"binding_type": "database_table", "parent_resource_id": "database-1", "resource_id": "table-old", "access_mode": "read", "bound_by_account_id": "binder-old", "bound_at_unix": int64(100)},
			{"binding_type": "database_table", "parent_resource_id": "database-1", "resource_id": "table-new", "access_mode": "read", "bound_by_account_id": "binder-new", "bound_at_unix": int64(200)},
		},
	})

	decision := decideSystemSkillToolForTest(t, SkillAgentDatabase, "query_table_records", map[string]interface{}{
		"data_source_id": "database-1",
		"table_id":       "table-new",
	}, params)
	assertAgentGovernanceDecision(t, decision, toolgovernance.DecisionStatusAllowed, "")
	if decision.Preauthorization == nil || decision.Preauthorization.AuthorizedBy != "binder-new" {
		t.Fatalf("preauthorization = %#v, want target table binder", decision.Preauthorization)
	}
}

func TestPolicyToolGovernanceAgentWorkflowBinding(t *testing.T) {
	params := agentGovernanceRuntimeParameters(map[string]interface{}{
		"workflow_binding_grant":       true,
		"workflow_bound_by_account_id": "account-1",
		"workflow_bound_at_unix":       int64(1_720_000_000),
		"workflow_bindings": []map[string]interface{}{{
			"binding_id":  "binding-1",
			"agent_id":    "workflow-agent-1",
			"workflow_id": "workflow-1",
		}},
	})

	allowed := decideSystemSkillToolForTest(t, SkillAgentWorkflow, "run_agent_workflow", map[string]interface{}{
		"binding_id": "binding-1",
		"inputs":     map[string]interface{}{"query": "run"},
	}, params)
	assertAgentGovernanceDecision(t, allowed, toolgovernance.DecisionStatusAllowed, "")

	unbound := decideSystemSkillToolForTest(t, SkillAgentWorkflow, "run_agent_workflow", map[string]interface{}{
		"binding_id": "binding-2",
		"inputs":     map[string]interface{}{"query": "run"},
	}, params)
	assertAgentGovernanceDecision(t, unbound, toolgovernance.DecisionStatusDenied, agentResourceNotBoundCode)
}

func TestPolicyToolGovernanceRechecksCurrentAgentBindingBeforeInvocation(t *testing.T) {
	params := agentGovernanceRuntimeParameters(map[string]interface{}{
		"database_binding_grant":       true,
		"database_bound_by_account_id": "account-1",
		"database_bound_at_unix":       int64(1_720_000_000),
		"database_bindings": []map[string]interface{}{{
			"data_source_id":     "database-1",
			"table_ids":          []string{"table-1"},
			"writable_table_ids": []string{"table-1"},
		}},
	})
	var got AgentBindingCheck
	params = WithAgentBindingVerifier(params, func(_ context.Context, check AgentBindingCheck) (bool, error) {
		got = check
		return false, nil
	})

	decision := decideSystemSkillToolForTest(t, SkillAgentDatabase, "delete_table_records", map[string]interface{}{
		"data_source_id": "database-1",
		"table_id":       "table-1",
		"record_ids":     []string{"record-1"},
	}, params)
	assertAgentGovernanceDecision(t, decision, toolgovernance.DecisionStatusDenied, agentResourceNotBoundCode)
	if got.BindingType != "database_table" || got.ResourceID != "table-1" ||
		got.ParentResourceID != "database-1" || got.AccessMode != "write" {
		t.Fatalf("binding check = %#v", got)
	}
}

func TestAgentWorkflowBindingCheckUsesTargetAgentAsParentResource(t *testing.T) {
	checks := agentBindingChecks(&toolgovernance.Preauthorization{
		BindingType: "workflow",
		Resources: []toolgovernance.AssetRef{{
			ID:   "binding-1",
			Type: "workflow",
			Metadata: map[string]interface{}{
				"agent_id":    "workflow-agent-1",
				"workflow_id": "workflow-definition-1",
			},
		}},
	}, toolgovernance.Manifest{Effect: toolgovernance.EffectInvoke})
	if len(checks) != 1 || checks[0].ParentResourceID != "workflow-agent-1" || checks[0].AccessMode != "execute" {
		t.Fatalf("checks = %#v", checks)
	}
}

func TestPolicyToolGovernanceNonBoundAgentToolCannotWaitForApproval(t *testing.T) {
	manifest := toolgovernance.Manifest{
		ToolID:                 "custom.send",
		SkillID:                "custom-skill",
		Domain:                 "custom",
		Effect:                 toolgovernance.EffectExternalSend,
		RiskLevel:              toolgovernance.RiskLevelHigh,
		DefaultApprovalPolicy:  toolgovernance.ApprovalPolicyAlwaysAsk,
		AllowedPermissionTiers: []toolgovernance.PermissionTier{toolgovernance.PermissionTierBasic},
		AuditRequired:          true,
	}
	gateway := NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy())
	decision, err := gateway.DecideSkillTool(context.Background(), ToolGovernanceRequest{
		Manifest:         manifest,
		SkillID:          "custom-skill",
		ToolName:         "send",
		ExecutionContext: ExecutionContext{RuntimeParameters: agentGovernanceRuntimeParameters(nil)},
	})
	if err != nil {
		t.Fatalf("DecideSkillTool() error = %v", err)
	}
	assertAgentGovernanceDecision(t, decision, toolgovernance.DecisionStatusDenied, agentToolNotPreauthorizedCode)

	autoAllowedManifest := manifest
	autoAllowedManifest.ToolID = "custom.read"
	autoAllowedManifest.Effect = toolgovernance.EffectRead
	autoAllowedManifest.RiskLevel = toolgovernance.RiskLevelLow
	autoAllowedManifest.DefaultApprovalPolicy = toolgovernance.ApprovalPolicyAutoByPermissionTier
	decision, err = gateway.DecideSkillTool(context.Background(), ToolGovernanceRequest{
		Manifest:         autoAllowedManifest,
		SkillID:          "custom-skill",
		ToolName:         "read",
		ExecutionContext: ExecutionContext{RuntimeParameters: agentGovernanceRuntimeParameters(nil)},
	})
	if err != nil {
		t.Fatalf("DecideSkillTool() error = %v", err)
	}
	if decision.Status != toolgovernance.DecisionStatusAllowed || decision.Preauthorization != nil {
		t.Fatalf("auto-allowed Agent decision = %#v, want original policy without binding denial", decision)
	}

	decision, err = gateway.DecideSkillTool(context.Background(), ToolGovernanceRequest{
		Manifest: manifest,
		SkillID:  "custom-skill",
		ToolName: "send",
		ExecutionContext: ExecutionContext{RuntimeParameters: map[string]interface{}{
			"tool_governance": map[string]interface{}{
				"caller_type":     SkillCallerAIChat,
				"permission_tier": "basic",
			},
		}},
	})
	if err != nil {
		t.Fatalf("DecideSkillTool() error = %v", err)
	}
	if decision.Status != toolgovernance.DecisionStatusNeedsApproval || !decision.RequiresApproval || decision.ApprovalEvent == nil {
		t.Fatalf("AIChat decision = %#v, want interactive approval", decision)
	}
}

func TestCallSkillToolAgentDenialReturnsModelFeedbackWithoutPendingApproval(t *testing.T) {
	runtime, resolved := governedRuntimeForTest(t)
	invocation, err := runtime.CallSkillTool(
		context.Background(),
		resolved,
		"governed-files",
		"delete_file",
		map[string]interface{}{"file_id": "file-1"},
		ExecutionContext{
			ConversationID: "conversation-1",
			RuntimeParameters: agentGovernanceRuntimeParameters(map[string]interface{}{
				"tool_governance_assets": []map[string]interface{}{{"id": "file-1", "type": "file"}},
			}),
		},
		"call_delete",
	)
	if err != nil {
		t.Fatalf("CallSkillTool() error = %v", err)
	}
	if invocation == nil || invocation.Trace.Status != string(toolgovernance.DecisionStatusDenied) {
		t.Fatalf("invocation = %#v, want denied governance result", invocation)
	}
	if invocation.Trace.Governance == nil || invocation.Trace.Governance.ApprovalEvent != nil || invocation.Trace.Governance.RequiresApproval {
		t.Fatalf("governance = %#v, Agent denial must not create pending approval", invocation.Trace.Governance)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(fmt.Sprint(invocation.ToolMessage.Content)), &payload); err != nil {
		t.Fatalf("tool message content is not JSON: %v", err)
	}
	feedback, ok := payload["governance"].(map[string]interface{})
	if !ok || feedback["authorization_code"] != agentToolNotPreauthorizedCode {
		t.Fatalf("tool feedback = %#v, want stable Agent authorization code", payload)
	}
	instruction, _ := feedback["instruction"].(string)
	if !strings.Contains(instruction, "Do not retry with unchanged arguments") {
		t.Fatalf("instruction = %q, want Agent recovery guidance", instruction)
	}
}

func decideSystemSkillToolForTest(
	t *testing.T,
	skillID string,
	toolName string,
	arguments map[string]interface{},
	runtimeParameters map[string]interface{},
) toolgovernance.Decision {
	t.Helper()
	manifest, ok := SystemSkillToolGovernanceManifest(skillID, toolName)
	if !ok {
		t.Fatalf("SystemSkillToolGovernanceManifest(%q, %q) = false", skillID, toolName)
	}
	decision, err := NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy()).DecideSkillTool(context.Background(), ToolGovernanceRequest{
		Manifest:         manifest,
		SkillID:          skillID,
		ToolName:         toolName,
		Arguments:        arguments,
		ExecutionContext: ExecutionContext{RuntimeParameters: runtimeParameters},
	})
	if err != nil {
		t.Fatalf("DecideSkillTool() error = %v", err)
	}
	return decision
}

func agentGovernanceRuntimeParameters(values map[string]interface{}) map[string]interface{} {
	params := make(map[string]interface{}, len(values)+1)
	for key, value := range values {
		params[key] = value
	}
	params[governanceRuntimeParametersKey] = map[string]interface{}{
		"caller_type":     SkillCallerAgent,
		"permission_tier": "basic",
		"runtime_surface": "external_page_chat",
	}
	return params
}

func assertAgentGovernanceDecision(t *testing.T, decision toolgovernance.Decision, want toolgovernance.DecisionStatus, code string) {
	t.Helper()
	if decision.Status != want {
		t.Fatalf("decision = %#v, want status %s", decision, want)
	}
	if decision.Status == toolgovernance.DecisionStatusNeedsApproval || decision.RequiresApproval || decision.ApprovalEvent != nil {
		t.Fatalf("Agent decision = %#v, must not create pending Tool Governance approval", decision)
	}
	if got, _ := decision.ModelFeedback["authorization_code"].(string); got != code {
		t.Fatalf("authorization_code = %q, want %q in %#v", got, code, decision.ModelFeedback)
	}
}
