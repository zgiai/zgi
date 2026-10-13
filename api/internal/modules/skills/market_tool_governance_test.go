package skills

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func marketWriteManifest() toolgovernance.Manifest {
	return toolgovernance.Manifest{
		ToolID: "zgi-market.create_item", SkillID: SkillZGIMarketTools, Domain: "zgi_market",
		Effect: toolgovernance.EffectInvoke, RiskLevel: toolgovernance.RiskLevelMedium,
		DefaultApprovalPolicy:  toolgovernance.ApprovalPolicyAlwaysAsk,
		AllowedPermissionTiers: []toolgovernance.PermissionTier{toolgovernance.PermissionTierBasic},
		AuditRequired:          true, ExternalSideEffect: true,
	}
}

func decideMarketWrite(t *testing.T, callerType, source string, providerType tools.ToolProviderType, skillID string) toolgovernance.Decision {
	t.Helper()
	params := map[string]interface{}{
		governanceRuntimeParametersKey: map[string]interface{}{"caller_type": callerType, "permission_tier": "basic"},
	}
	if source != "" {
		params["agent_runtime_source"] = source
	}
	manifest := marketWriteManifest()
	manifest.SkillID = skillID
	decision, err := NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy()).DecideSkillTool(context.Background(), ToolGovernanceRequest{
		Manifest: manifest, SkillID: skillID, ToolName: "create_item", ProviderType: providerType,
		ProviderID: "agent:1:items", Arguments: map[string]interface{}{"name": "one"},
		ExecutionContext: ExecutionContext{ConversationID: "conversation", OrganizationID: "organization", UserID: "user", RuntimeParameters: params},
	})
	if err != nil {
		t.Fatalf("DecideSkillTool() error = %v", err)
	}
	return decision
}

// A market write asks the person in the console chat of an agent, and nowhere else.
func TestAMarketWriteAsksOnlyInTheConsoleChatOfAnAgent(t *testing.T) {
	decision := decideMarketWrite(t, SkillCallerAgent, "console", tools.ToolProviderTypeMarket, SkillZGIMarketTools)
	if decision.Status != toolgovernance.DecisionStatusNeedsApproval || !decision.RequiresApproval {
		t.Fatalf("console agent decision = %#v, want a pending approval", decision)
	}
	for name, refused := range map[string]toolgovernance.Decision{
		"a published web app":         decideMarketWrite(t, SkillCallerAgent, "webapp", tools.ToolProviderTypeMarket, SkillZGIMarketTools),
		"an API caller":               decideMarketWrite(t, SkillCallerAgent, "external-api", tools.ToolProviderTypeMarket, SkillZGIMarketTools),
		"a run without a source":      decideMarketWrite(t, SkillCallerAgent, "", tools.ToolProviderTypeMarket, SkillZGIMarketTools),
		"a digital human":             decideMarketWrite(t, SkillCallerDigitalHuman, "console", tools.ToolProviderTypeMarket, SkillZGIMarketTools),
		"another provider's tool":     decideMarketWrite(t, SkillCallerAgent, "console", tools.ToolProviderTypeBuiltin, SkillZGIMarketTools),
		"a Skill that borrows the id": decideMarketWrite(t, SkillCallerAgent, "console", tools.ToolProviderTypeMarket, "custom-skill"),
	} {
		if refused.Status != toolgovernance.DecisionStatusDenied || refused.RequiresApproval || refused.ApprovalEvent != nil {
			t.Fatalf("%s: decision = %#v, want a refusal without a pending approval", name, refused)
		}
	}
}

// A read market tool keeps running without anyone being asked, attended or not.
func TestAMarketReadRunsWithoutApproval(t *testing.T) {
	for _, source := range []string{"console", "webapp", ""} {
		params := map[string]interface{}{
			governanceRuntimeParametersKey: map[string]interface{}{"caller_type": SkillCallerDigitalHuman, "permission_tier": "basic"},
			"agent_runtime_source":         source,
		}
		manifest := marketWriteManifest()
		manifest.Effect, manifest.RiskLevel = toolgovernance.EffectRead, toolgovernance.RiskLevelLow
		manifest.DefaultApprovalPolicy, manifest.ExternalSideEffect, manifest.Reversible = toolgovernance.ApprovalPolicyAutoByPermissionTier, false, true
		decision, err := NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy()).DecideSkillTool(context.Background(), ToolGovernanceRequest{
			Manifest: manifest, SkillID: SkillZGIMarketTools, ToolName: "lookup", ProviderType: tools.ToolProviderTypeMarket,
			ExecutionContext: ExecutionContext{RuntimeParameters: params},
		})
		if err != nil || decision.Status != toolgovernance.DecisionStatusAllowed {
			t.Fatalf("source %q: decision = %#v, err = %v, want allowed", source, decision, err)
		}
	}
}

// A costly read asks where a write asks: the console chat of an agent, and nowhere else.
func TestACostlyMarketReadAsksOnlyInTheConsoleChatOfAnAgent(t *testing.T) {
	decide := func(callerType, source string) toolgovernance.Decision {
		t.Helper()
		params := map[string]interface{}{
			governanceRuntimeParametersKey: map[string]interface{}{"caller_type": callerType, "permission_tier": "basic"},
			"agent_runtime_source":         source,
		}
		manifest := toolgovernance.Manifest{
			ToolID: "zgi-market.web_search", SkillID: SkillZGIMarketTools, Domain: "zgi_market",
			Effect: toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelMedium,
			DefaultApprovalPolicy:  toolgovernance.ApprovalPolicyAlwaysAsk,
			AllowedPermissionTiers: []toolgovernance.PermissionTier{toolgovernance.PermissionTierBasic},
			AuditRequired:          true,
		}
		decision, err := NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy()).DecideSkillTool(context.Background(), ToolGovernanceRequest{
			Manifest: manifest, SkillID: SkillZGIMarketTools, ToolName: "web_search", ProviderType: tools.ToolProviderTypeMarket,
			ProviderID: "agent:1:api:web-search", Arguments: map[string]interface{}{"query": "one"},
			ExecutionContext: ExecutionContext{ConversationID: "conversation", OrganizationID: "organization", UserID: "user", RuntimeParameters: params},
		})
		if err != nil {
			t.Fatalf("DecideSkillTool() error = %v", err)
		}
		return decision
	}
	asked := decide(SkillCallerAgent, "console")
	if asked.Status != toolgovernance.DecisionStatusNeedsApproval || !asked.RequiresApproval {
		t.Fatalf("console agent decision = %#v, want a pending approval", asked)
	}
	for name, refused := range map[string]toolgovernance.Decision{
		"a published web app": decide(SkillCallerAgent, "webapp"),
		"a digital human":     decide(SkillCallerDigitalHuman, "console"),
	} {
		if refused.Status != toolgovernance.DecisionStatusDenied || refused.RequiresApproval {
			t.Fatalf("%s: decision = %#v, want a refusal", name, refused)
		}
	}
}

// rebuildingResolver stands for an organization policy that builds the manifest anew and so
// carries over nothing the policy does not know.
type rebuildingResolver struct{}

func (rebuildingResolver) ResolveToolGovernanceManifest(_ context.Context, req ToolGovernanceRequest) (toolgovernance.Manifest, error) {
	manifest := req.Manifest
	manifest.ApprovalReason = ""
	return manifest, nil
}

func costlyMarketRead() toolgovernance.Manifest {
	return toolgovernance.Manifest{
		ToolID: "zgi-market.web_search", SkillID: SkillZGIMarketTools, Domain: "zgi_market",
		Effect: toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelMedium,
		DefaultApprovalPolicy:  toolgovernance.ApprovalPolicyAlwaysAsk,
		AllowedPermissionTiers: []toolgovernance.PermissionTier{toolgovernance.PermissionTierBasic},
		AuditRequired:          true,
		ApprovalReason:         MarketCostApprovalReason("201", "100"),
	}
}

func decideMarketTool(t *testing.T, gateway *PolicyToolGovernanceGateway, manifest toolgovernance.Manifest, callerType, source string) toolgovernance.Decision {
	t.Helper()
	params := map[string]interface{}{
		governanceRuntimeParametersKey: map[string]interface{}{"caller_type": callerType, "permission_tier": "basic"},
		"agent_runtime_source":         source,
	}
	decision, err := gateway.DecideSkillTool(context.Background(), ToolGovernanceRequest{
		Manifest: manifest, SkillID: SkillZGIMarketTools, ToolName: "web_search", ProviderType: tools.ToolProviderTypeMarket,
		ProviderID: "agent:1:api:web-search", Arguments: map[string]interface{}{"query": "one"},
		ExecutionContext: ExecutionContext{ConversationID: "conversation", OrganizationID: "organization", UserID: "user", RuntimeParameters: params},
	})
	if err != nil {
		t.Fatalf("DecideSkillTool() error = %v", err)
	}
	return decision
}

// The approval a person is asked for says why: the cost reason reaches the decision and the
// feedback the page reads, also when an organization policy rebuilt the manifest on the way.
func TestTheCostReasonOfAMarketToolReachesTheApprovalAlsoThroughAnOrganizationPolicy(t *testing.T) {
	const reason = "one call can cost up to 201 points, above the confirmation threshold of 100 points"
	for name, gateway := range map[string]*PolicyToolGovernanceGateway{
		"without an organization policy": NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy()),
		"with a policy that rebuilds the manifest": NewPolicyToolGovernanceGateway(
			toolgovernance.DefaultPolicy()).WithManifestResolver(rebuildingResolver{}),
	} {
		decision := decideMarketTool(t, gateway, costlyMarketRead(), SkillCallerAgent, "console")
		if decision.Status != toolgovernance.DecisionStatusNeedsApproval || decision.ApprovalEvent == nil {
			t.Fatalf("%s: decision = %#v, want a pending approval", name, decision)
		}
		if decision.Reason != reason {
			t.Fatalf("%s: reason = %q, want the cost reason", name, decision.Reason)
		}
		feedback, _ := governanceToolFeedback(decision)["governance"].(map[string]interface{})
		if feedback["reason"] != reason {
			t.Fatalf("%s: the feedback the page reads says %q", name, feedback["reason"])
		}
	}
}

// Where nobody can approve, the end user is told why in a sentence, not in an internal code. The
// code stays in the decision for diagnostics and is kept from the model, which would repeat it.
func TestAnUnattendedMarketToolIsRefusedInAPersonsWords(t *testing.T) {
	gateway := NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy())
	for name, want := range map[string]struct {
		manifest toolgovernance.Manifest
		chinese  string
		english  string
	}{
		"a tool that writes": {
			manifest: marketWriteManifest(),
			chinese:  "该工具会写入数据，需要人工确认，无人值守时不会执行",
			english:  "This tool writes data and needs a person's confirmation, so it does not run unattended",
		},
		"a tool that costs much": {
			manifest: costlyMarketRead(),
			chinese:  "该工具单次最高 201 点，超过确认阈值 100 点，无人值守时不会执行",
			english:  "One call of this tool can cost up to 201 points, above the confirmation threshold of 100 points, so it does not run unattended",
		},
	} {
		for _, caller := range [][2]string{{SkillCallerDigitalHuman, "console"}, {SkillCallerAgent, "webapp"}} {
			decision := decideMarketTool(t, gateway, want.manifest, caller[0], caller[1])
			if decision.Status != toolgovernance.DecisionStatusDenied || decision.RequiresApproval {
				t.Fatalf("%s: decision = %#v, want a refusal", name, decision)
			}
			if decision.Reason != want.english {
				t.Fatalf("%s: reason = %q", name, decision.Reason)
			}
			messages := marketToolUserMessages(decision)
			if messages["zh-CN"] != want.chinese || messages["en-US"] != want.english {
				t.Fatalf("%s: user message = %#v", name, messages)
			}
			if decision.Preauthorization == nil || decision.Preauthorization.Code != agentToolNotPreauthorizedCode {
				t.Fatalf("%s: the code is gone from the decision: %#v", name, decision.Preauthorization)
			}
			if code := governanceToolFeedback(decision)["governance"].(map[string]interface{})["authorization_code"]; code != agentToolNotPreauthorizedCode {
				t.Fatalf("%s: the trace lost the code: %v", name, code)
			}
			told, _ := modelFacingGovernanceFeedback(decision)["governance"].(map[string]interface{})
			for _, internal := range []string{"authorization_code", "authorization_source", "preauthorization", "asset_operation_audit"} {
				if _, shown := told[internal]; shown {
					t.Fatalf("%s: the model is shown %s", name, internal)
				}
			}
			instruction, _ := told["instruction"].(string)
			if !strings.Contains(instruction, want.chinese) || !strings.Contains(instruction, want.english) {
				t.Fatalf("%s: instruction = %q", name, instruction)
			}
			if strings.Contains(fmt.Sprint(told), agentToolNotPreauthorizedCode) {
				t.Fatalf("%s: the internal code reaches the model: %v", name, told)
			}
		}
	}
	// Any other Skill's refusal keeps the code and the general instruction.
	other := marketWriteManifest()
	other.SkillID = "custom-skill"
	params := map[string]interface{}{governanceRuntimeParametersKey: map[string]interface{}{"caller_type": SkillCallerDigitalHuman, "permission_tier": "basic"}}
	decision, err := gateway.DecideSkillTool(context.Background(), ToolGovernanceRequest{
		Manifest: other, SkillID: "custom-skill", ToolName: "create_item", ProviderType: tools.ToolProviderTypeBuiltin,
		ExecutionContext: ExecutionContext{ConversationID: "c", OrganizationID: "o", UserID: "u", RuntimeParameters: params},
	})
	if err != nil {
		t.Fatalf("DecideSkillTool() error = %v", err)
	}
	if marketToolUserMessages(decision) != nil {
		t.Fatalf("another Skill's refusal was reworded: %#v", decision)
	}
}
