package skills

import (
	"context"
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
