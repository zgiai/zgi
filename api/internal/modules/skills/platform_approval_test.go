package skills

import (
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
)

func TestPlatformApprovalRequiresTypedMatchingIdentityAndDoesNotGrantResources(t *testing.T) {
	for _, app := range []string{SkillCallerAgent, SkillCallerDigitalHuman} {
		for _, entry := range []string{"console", "webapp", "external-api"} {
			t.Run(app+"/"+entry, func(t *testing.T) {
				o := toolgovernance.ExecutionOrigin{OrganizationID: uuid.NewString(), WorkspaceID: uuid.NewString(), ApplicationType: app,
					ApplicationID: uuid.NewString(), Entry: entry, SubjectKind: "account", SubjectID: uuid.NewString(),
					ConversationID: uuid.NewString(), MessageID: uuid.NewString(), PublicationRef: uuid.NewString()}
				if entry == "external-api" {
					o.SubjectKind = "api_end_user"
				}
				params := agentGovernanceRuntimeParameters(nil)
				params["agent_runtime_source"], params["tool_governance_caller_type"] = entry, app
				params[governanceRuntimeParametersKey].(map[string]interface{})["caller_type"] = app
				x := ExecutionContext{OrganizationID: o.OrganizationID, AppID: o.ApplicationID, ConversationID: o.ConversationID,
					MessageID: o.MessageID, PlatformInteractiveApproval: true, PlatformExecutionOrigin: &o, RuntimeParameters: params}
				manifest := toolgovernance.Manifest{ToolID: "calculator.evaluate_expression", SkillID: SkillCalculator, Domain: "calculation",
					Effect: toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow, DefaultApprovalPolicy: toolgovernance.ApprovalPolicyAlwaysAsk,
					AllowedPermissionTiers: []toolgovernance.PermissionTier{toolgovernance.PermissionTierBasic}}
				gateway := NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy())
				decision, err := gateway.DecideSkillTool(t.Context(), ToolGovernanceRequest{Manifest: manifest, SkillID: SkillCalculator, ToolName: "evaluate_expression", ExecutionContext: x})
				if err != nil || decision.Status != toolgovernance.DecisionStatusNeedsApproval {
					t.Fatalf("decision=%+v error=%v", decision, err)
				}
				for _, skill := range []string{SkillAgentDatabase, SkillAgentWorkflow} {
					decision, err := gateway.DecideSkillTool(t.Context(), ToolGovernanceRequest{Manifest: manifest, SkillID: skill, ToolName: "evaluate_expression", ExecutionContext: x})
					if err != nil || decision.Status != toolgovernance.DecisionStatusDenied {
						t.Fatalf("missing resource binding: %+v %v", decision, err)
					}
				}
				x.MessageID = uuid.NewString()
				if platformInteractiveGovernanceAllowed(x, params, params[governanceRuntimeParametersKey].(map[string]interface{})) {
					t.Fatal("wrong message obtained approval capability")
				}
			})
		}
	}
}
