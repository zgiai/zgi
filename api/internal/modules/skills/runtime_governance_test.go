package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestCallSkillToolGovernanceNeedsApprovalDoesNotInvokeEngine(t *testing.T) {
	runtime, resolved := governedRuntimeForTest(t)
	invocation, err := runtime.CallSkillTool(
		context.Background(),
		resolved,
		"governed-files",
		"delete_file",
		map[string]interface{}{"file_id": "file-1"},
		ExecutionContext{
			OrganizationID: "organization-1",
			UserID:         "user-1",
			ConversationID: "conversation-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance": map[string]interface{}{
					"permission_tier": "basic",
					"assets": []map[string]interface{}{
						{"id": "file-1", "type": "file", "name": "report.pdf", "workspace_id": "workspace-1"},
					},
				},
			},
		},
		"call_delete",
	)
	if err != nil {
		t.Fatalf("CallSkillTool() error = %v", err)
	}
	if invocation == nil {
		t.Fatalf("CallSkillTool() invocation = nil")
	}
	if invocation.Trace.Kind != "tool_governance" || invocation.Trace.Status != string(toolgovernance.DecisionStatusNeedsApproval) {
		t.Fatalf("trace = %#v, want governance needs_approval", invocation.Trace)
	}
	if invocation.Trace.Governance == nil || invocation.Trace.Governance.ApprovalEvent == nil {
		t.Fatalf("governance decision missing approval event: %#v", invocation.Trace.Governance)
	}
	if invocation.Trace.Governance.ApprovalEvent.Grant.ConversationID != "conversation-1" {
		t.Fatalf("approval grant = %#v, want conversation-bound grant", invocation.Trace.Governance.ApprovalEvent.Grant)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(fmt.Sprint(invocation.ToolMessage.Content)), &payload); err != nil {
		t.Fatalf("tool message content is not JSON: %v", err)
	}
	governance, ok := payload["governance"].(map[string]interface{})
	if !ok {
		t.Fatalf("tool message payload = %#v, want governance object", payload)
	}
	if governance["status"] != string(toolgovernance.DecisionStatusNeedsApproval) || governance["requires_approval"] != true {
		t.Fatalf("governance feedback = %#v", governance)
	}
	if instruction := strings.TrimSpace(governance["instruction"].(string)); !strings.Contains(instruction, "not executed") {
		t.Fatalf("instruction = %q, want not executed guidance", instruction)
	}
}

func TestCallSkillToolValidatesRequiredArgumentsBeforeGovernance(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog").WithToolGovernanceGateway(NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy()))
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillAgentManagement})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	invocation, err := runtime.CallSkillTool(
		context.Background(),
		resolved,
		SkillAgentManagement,
		"create_agent",
		map[string]interface{}{},
		ExecutionContext{
			OrganizationID: "organization-1",
			UserID:         "user-1",
			ConversationID: "conversation-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance": map[string]interface{}{
					"permission_tier": "basic",
				},
			},
		},
		"call_create",
	)
	if err == nil {
		t.Fatalf("CallSkillTool() error = nil, want missing required argument")
	}
	if invocation == nil {
		t.Fatalf("CallSkillTool() invocation = nil")
	}
	if invocation.Trace.Kind != "tool_call" || invocation.Trace.Status != "error" {
		t.Fatalf("trace = %#v, want tool_call error", invocation.Trace)
	}
	if invocation.Trace.Governance != nil {
		t.Fatalf("governance = %#v, want nil before approval preflight", invocation.Trace.Governance)
	}
	if !strings.Contains(err.Error(), "name") {
		t.Fatalf("error = %q, want missing name", err.Error())
	}
}

func TestCallSkillToolGovernanceNeedsResolutionBeforeEngine(t *testing.T) {
	runtime, resolved := governedRuntimeForTest(t)
	invocation, err := runtime.CallSkillTool(
		context.Background(),
		resolved,
		"governed-files",
		"read_file",
		map[string]interface{}{"ref": "fourth file"},
		ExecutionContext{
			ConversationID: "conversation-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance_permission_tier": "basic",
			},
		},
		"call_read",
	)
	if err != nil {
		t.Fatalf("CallSkillTool() error = %v", err)
	}
	if invocation.Trace.Status != string(toolgovernance.DecisionStatusNeedsResolution) {
		t.Fatalf("trace status = %s, want needs_resolution", invocation.Trace.Status)
	}
	if invocation.Trace.Governance == nil || invocation.Trace.Governance.Status != toolgovernance.DecisionStatusNeedsResolution {
		t.Fatalf("governance = %#v, want needs_resolution", invocation.Trace.Governance)
	}
}

func TestPolicyToolGovernanceUsesToolArgumentsAsAssetRefs(t *testing.T) {
	gateway := NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy())
	decision, err := gateway.DecideSkillTool(context.Background(), ToolGovernanceRequest{
		Manifest: toolgovernance.Manifest{
			ToolID:                  "file.read",
			Domain:                  "files",
			Effect:                  toolgovernance.EffectRead,
			AssetType:               "file",
			RiskLevel:               toolgovernance.RiskLevelLow,
			RequiresAssetResolution: true,
		},
		SkillID:   "file-reader",
		ToolName:  "read_file",
		Arguments: map[string]interface{}{"file_id": "file-1", "file_name": "report.pdf", "workspace_id": "workspace-1"},
		ExecutionContext: ExecutionContext{
			ConversationID: "conversation-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance_permission_tier": "basic",
			},
		},
	})
	if err != nil {
		t.Fatalf("DecideSkillTool() error = %v", err)
	}
	if decision.Status != toolgovernance.DecisionStatusAllowed {
		t.Fatalf("decision status = %s, want allowed: %#v", decision.Status, decision)
	}
	if len(decision.Assets) != 1 || decision.Assets[0].ID != "file-1" || decision.Assets[0].Name != "report.pdf" {
		t.Fatalf("assets = %#v, want file-1/report.pdf from tool arguments", decision.Assets)
	}
}

func TestPolicyToolGovernanceRejectsToolArgumentOutsideResolvedAssets(t *testing.T) {
	gateway := NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy())
	decision, err := gateway.DecideSkillTool(context.Background(), ToolGovernanceRequest{
		Manifest: toolgovernance.Manifest{
			ToolID:                  "file.delete",
			Domain:                  "files",
			Effect:                  toolgovernance.EffectDelete,
			AssetType:               "file",
			RiskLevel:               toolgovernance.RiskLevelHigh,
			RequiresAssetResolution: true,
		},
		SkillID:   "file-reader",
		ToolName:  "delete_file",
		Arguments: map[string]interface{}{"file_id": "file-2", "file_name": "target.pdf"},
		ExecutionContext: ExecutionContext{
			ConversationID: "conversation-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance": map[string]interface{}{
					"permission_tier": "basic",
					"assets": []map[string]interface{}{
						{"id": "file-1", "type": "file", "name": "wrong.pdf", "workspace_id": "workspace-1"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("DecideSkillTool() error = %v", err)
	}
	if decision.Status != toolgovernance.DecisionStatusNeedsResolution {
		t.Fatalf("decision status = %s, want needs_resolution: %#v", decision.Status, decision)
	}
	if len(decision.Assets) != 1 || decision.Assets[0].ID != "file-2" || decision.Assets[0].Name != "target.pdf" {
		t.Fatalf("assets = %#v, want file-2/target.pdf from tool arguments", decision.Assets)
	}
	if len(decision.ExpectedAssets) != 1 || decision.ExpectedAssets[0].ID != "file-1" || decision.ExpectedAssets[0].Name != "wrong.pdf" {
		t.Fatalf("expected assets = %#v, want resolved runtime asset file-1/wrong.pdf", decision.ExpectedAssets)
	}
	if expected := decision.ModelFeedback["expected_assets"]; expected == nil {
		t.Fatalf("model feedback = %#v, want expected_assets", decision.ModelFeedback)
	}
}

func TestPolicyToolGovernanceIgnoresRuntimeAssetsWithDifferentType(t *testing.T) {
	gateway := NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy())
	decision, err := gateway.DecideSkillTool(context.Background(), ToolGovernanceRequest{
		Manifest: toolgovernance.Manifest{
			ToolID:                  "agent.delete",
			Domain:                  "agents",
			Effect:                  toolgovernance.EffectDelete,
			AssetType:               "agent",
			RiskLevel:               toolgovernance.RiskLevelHigh,
			RequiresAssetResolution: true,
			DefaultApprovalPolicy:   toolgovernance.ApprovalPolicyAlwaysAsk,
		},
		SkillID:   "agent-management",
		ToolName:  "delete_agent",
		Arguments: map[string]interface{}{"agent_id": "agent-1", "agent_name": "Novel Agent"},
		ExecutionContext: ExecutionContext{
			ConversationID: "conversation-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance": map[string]interface{}{
					"permission_tier": "basic",
					"assets": []map[string]interface{}{
						{"id": "file-1", "type": "file", "name": "first.txt", "workspace_id": "workspace-1"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("DecideSkillTool() error = %v", err)
	}
	if decision.Status != toolgovernance.DecisionStatusNeedsApproval {
		t.Fatalf("decision status = %s, want needs_approval: %#v", decision.Status, decision)
	}
	if len(decision.ExpectedAssets) != 0 {
		t.Fatalf("expected assets = %#v, want runtime file assets ignored for agent tool", decision.ExpectedAssets)
	}
	if len(decision.Assets) != 1 || decision.Assets[0].ID != "agent-1" || decision.Assets[0].Name != "Novel Agent" {
		t.Fatalf("assets = %#v, want agent-1/Novel Agent from tool arguments", decision.Assets)
	}
}

func TestPolicyToolGovernanceFileGeneratorTemporaryArtifactsDoNotNeedApproval(t *testing.T) {
	gateway := NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy())
	decision, err := gateway.DecideSkillTool(context.Background(), ToolGovernanceRequest{
		Manifest: fileGeneratorGovernanceManifestForTest(),
		SkillID:  "file-generator",
		ToolName: "generate_file",
		Arguments: map[string]interface{}{
			"filename": "draft.txt",
			"target":   "temporary_artifact",
		},
		ExecutionContext: ExecutionContext{
			ConversationID: "conversation-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance_permission_tier": "basic",
			},
		},
	})
	if err != nil {
		t.Fatalf("DecideSkillTool() error = %v", err)
	}
	if decision.Status != toolgovernance.DecisionStatusAllowed || decision.RequiresApproval {
		t.Fatalf("decision = %#v, want allowed without approval for temporary artifact", decision)
	}
	if decision.Manifest.DefaultApprovalPolicy != toolgovernance.ApprovalPolicyNeverAsk {
		t.Fatalf("manifest policy = %q, want temporary artifact override", decision.Manifest.DefaultApprovalPolicy)
	}
}

func TestPolicyToolGovernanceFileGeneratorLegacyManagedTargetStillStaysTemporaryPolicy(t *testing.T) {
	gateway := NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy())
	decision, err := gateway.DecideSkillTool(context.Background(), ToolGovernanceRequest{
		Manifest: fileGeneratorGovernanceManifestForTest(),
		SkillID:  "file-generator",
		ToolName: "generate_pdf",
		Arguments: map[string]interface{}{
			"filename": "report.pdf",
			"target":   "managed_file",
		},
		ExecutionContext: ExecutionContext{
			ConversationID: "conversation-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance_permission_tier": "basic",
			},
		},
	})
	if err != nil {
		t.Fatalf("DecideSkillTool() error = %v", err)
	}
	if decision.Status != toolgovernance.DecisionStatusAllowed || decision.RequiresApproval {
		t.Fatalf("decision = %#v, want allowed without approval because file-generator only creates temporary artifacts", decision)
	}
	if decision.Manifest.DefaultApprovalPolicy != toolgovernance.ApprovalPolicyNeverAsk {
		t.Fatalf("manifest policy = %q, want generator override", decision.Manifest.DefaultApprovalPolicy)
	}
}

func TestPolicyToolGovernanceFileGeneratorConsoleFilesDefaultKeepsTemporaryArtifactPolicy(t *testing.T) {
	gateway := NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy())
	decision, err := gateway.DecideSkillTool(context.Background(), ToolGovernanceRequest{
		Manifest: fileGeneratorGovernanceManifestForTest(),
		SkillID:  "file-generator",
		ToolName: "generate_pdf",
		Arguments: map[string]interface{}{
			"filename": "report.pdf",
		},
		ExecutionContext: ExecutionContext{
			ConversationID: "conversation-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance_permission_tier": "basic",
				"console_files_page":              true,
			},
		},
	})
	if err != nil {
		t.Fatalf("DecideSkillTool() error = %v", err)
	}
	if decision.Status != toolgovernance.DecisionStatusAllowed || decision.RequiresApproval {
		t.Fatalf("decision = %#v, want allowed temporary artifact default on Files page", decision)
	}
	if decision.Manifest.DefaultApprovalPolicy != toolgovernance.ApprovalPolicyNeverAsk {
		t.Fatalf("manifest policy = %q, want temporary artifact override", decision.Manifest.DefaultApprovalPolicy)
	}
}

func TestPolicyToolGovernanceEnrichesFileArgumentNameFromVisibleFiles(t *testing.T) {
	gateway := NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy())
	decision, err := gateway.DecideSkillTool(context.Background(), ToolGovernanceRequest{
		Manifest: toolgovernance.Manifest{
			ToolID:                  "file.read",
			Domain:                  "files",
			Effect:                  toolgovernance.EffectRead,
			AssetType:               "file",
			RiskLevel:               toolgovernance.RiskLevelLow,
			RequiresAssetResolution: false,
		},
		SkillID:   "file-reader",
		ToolName:  "read_file",
		Arguments: map[string]interface{}{"file_id": "file-1"},
		ExecutionContext: ExecutionContext{
			ConversationID: "conversation-1",
			RuntimeParameters: map[string]interface{}{
				"console_files_visible_files": []map[string]interface{}{
					{"file_id": "file-1", "name": "report.pdf", "workspace_id": "workspace-1"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("DecideSkillTool() error = %v", err)
	}
	if decision.Status != toolgovernance.DecisionStatusAllowed {
		t.Fatalf("decision status = %s, want allowed: %#v", decision.Status, decision)
	}
	if len(decision.Assets) != 1 || decision.Assets[0].ID != "file-1" || decision.Assets[0].Name != "report.pdf" {
		t.Fatalf("assets = %#v, want file-1/report.pdf from visible files", decision.Assets)
	}
}

func fileGeneratorGovernanceManifestForTest() toolgovernance.Manifest {
	return toolgovernance.Manifest{
		ToolID:                "file.generate",
		SkillID:               "file-generator",
		Domain:                "files",
		Effect:                toolgovernance.EffectCreate,
		AssetType:             "file",
		RiskLevel:             toolgovernance.RiskLevelMedium,
		DefaultApprovalPolicy: toolgovernance.ApprovalPolicyAutoByPermissionTier,
		AllowedPermissionTiers: []toolgovernance.PermissionTier{
			toolgovernance.PermissionTierBasic,
			toolgovernance.PermissionTierAdvanced,
			toolgovernance.PermissionTierFull,
		},
	}
}

func TestPolicyToolGovernanceRequiresToolArgumentWhenResolvedAssetExists(t *testing.T) {
	gateway := NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy())
	decision, err := gateway.DecideSkillTool(context.Background(), ToolGovernanceRequest{
		Manifest: toolgovernance.Manifest{
			ToolID:                  "file.read",
			Domain:                  "files",
			Effect:                  toolgovernance.EffectRead,
			AssetType:               "file",
			RiskLevel:               toolgovernance.RiskLevelLow,
			RequiresAssetResolution: true,
		},
		SkillID:   "file-reader",
		ToolName:  "read_file",
		Arguments: map[string]interface{}{"ref": "second Excel"},
		ExecutionContext: ExecutionContext{
			ConversationID: "conversation-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance": map[string]interface{}{
					"permission_tier": "advanced",
					"assets": []map[string]interface{}{
						{"id": "file-2", "type": "file", "name": "second.xlsx", "workspace_id": "workspace-1"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("DecideSkillTool() error = %v", err)
	}
	if decision.Status != toolgovernance.DecisionStatusNeedsResolution {
		t.Fatalf("decision status = %s, want needs_resolution: %#v", decision.Status, decision)
	}
	if len(decision.Assets) != 0 {
		t.Fatalf("assets = %#v, want no executable asset from missing file_id", decision.Assets)
	}
	if len(decision.ExpectedAssets) != 1 || decision.ExpectedAssets[0].ID != "file-2" {
		t.Fatalf("expected assets = %#v, want resolved file-2", decision.ExpectedAssets)
	}
}

func TestCallSkillToolGovernanceRewritesReadArgumentsToResolvedAsset(t *testing.T) {
	runtime, resolved, readTool := governedRuntimeWithReadToolForTest(t)
	invocation, err := runtime.CallSkillTool(
		context.Background(),
		resolved,
		"governed-files",
		"read_file",
		map[string]interface{}{"file_id": "file-wrong", "max_chars": 8000},
		ExecutionContext{
			ConversationID: "conversation-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance": map[string]interface{}{
					"permission_tier": "basic",
					"assets": []map[string]interface{}{
						{"id": "file-expected", "type": "file", "name": "second.xlsx", "workspace_id": "workspace-1"},
					},
				},
			},
		},
		"call_read",
	)
	if err != nil {
		t.Fatalf("CallSkillTool() error = %v", err)
	}
	if invocation == nil || invocation.Trace.Status != "success" {
		t.Fatalf("invocation = %#v, want successful tool call", invocation)
	}
	if len(readTool.calls) != 1 || readTool.calls[0] != "file-expected" {
		t.Fatalf("read calls = %#v, want resolved file-expected", readTool.calls)
	}
	if invocation.Trace.Governance == nil || invocation.Trace.Governance.Status != toolgovernance.DecisionStatusAllowed {
		t.Fatalf("governance = %#v, want allowed after rewrite", invocation.Trace.Governance)
	}
	if invocation.Trace.Arguments["file_id"] != "file-expected" {
		t.Fatalf("trace arguments = %#v, want rewritten file_id", invocation.Trace.Arguments)
	}
	rewrite, ok := invocation.Trace.Arguments["governance_argument_rewrite"].(map[string]interface{})
	if !ok || rewrite["from_file_id"] != "file-wrong" || rewrite["to_file_id"] != "file-expected" {
		t.Fatalf("rewrite summary = %#v, want from/to file ids", invocation.Trace.Arguments["governance_argument_rewrite"])
	}
	if len(invocation.Messages) != 2 {
		t.Fatalf("messages len = %d, want business result plus governance observation", len(invocation.Messages))
	}
	if invocation.Messages[1].Type != tools.ToolInvokeMessageTypeJSON {
		t.Fatalf("governance observation message = %#v, want JSON", invocation.Messages[1])
	}
	observation := invocation.Messages[1].Data
	if observation["kind"] != "resolved_target_observation" {
		t.Fatalf("governance observation = %#v, want resolved_target_observation", observation)
	}
	if got := strings.TrimSpace(fmt.Sprint(observation["resolved_asset_guidance"])); !strings.Contains(got, "do not mention internal resolution") {
		t.Fatalf("resolved asset guidance = %q", got)
	}
	content := fmt.Sprint(invocation.ToolMessage.Content)
	for _, want := range []string{"resolved_target_observation", "file-expected", "second.xlsx", "do not mention internal resolution"} {
		if !strings.Contains(content, want) {
			t.Fatalf("tool message content missing %q in %s", want, content)
		}
	}
}

func TestCallSkillToolGovernanceUsesToolArgumentEnrichmentBeforePreflight(t *testing.T) {
	runtime, resolved, readTool := governedRuntimeWithReadToolForTest(t)
	readTool.enrich = func(arguments map[string]interface{}) map[string]interface{} {
		out := copyStringAnyMap(arguments)
		out["file_name"] = "Readable File.md"
		return out
	}
	invocation, err := runtime.CallSkillTool(
		context.Background(),
		resolved,
		"governed-files",
		"read_file",
		map[string]interface{}{"file_id": "file-1"},
		ExecutionContext{
			ConversationID: "conversation-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance": map[string]interface{}{
					"permission_tier": "basic",
					"assets": []map[string]interface{}{
						{"id": "file-1", "type": "file", "workspace_id": "workspace-1"},
					},
				},
			},
		},
		"call_read",
	)
	if err != nil {
		t.Fatalf("CallSkillTool() error = %v", err)
	}
	if invocation == nil || invocation.Trace.Governance == nil {
		t.Fatalf("invocation = %#v, want governance trace", invocation)
	}
	if got := fmt.Sprint(invocation.Trace.Arguments["file_name"]); got != "Readable File.md" {
		t.Fatalf("trace file_name = %q, want Readable File.md", got)
	}
	if assets := invocation.Trace.Governance.Assets; len(assets) != 1 || assets[0].Name != "Readable File.md" {
		t.Fatalf("governance assets = %#v, want enriched file name", assets)
	}
}

func TestPolicyToolGovernanceEnrichesArgumentAssetFromRuntimeAsset(t *testing.T) {
	gateway := NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy())
	decision, err := gateway.DecideSkillTool(context.Background(), ToolGovernanceRequest{
		Manifest: toolgovernance.Manifest{
			ToolID:                  "file.delete",
			Domain:                  "files",
			Effect:                  toolgovernance.EffectDelete,
			AssetType:               "file",
			RiskLevel:               toolgovernance.RiskLevelHigh,
			RequiresAssetResolution: true,
		},
		SkillID:   "file-reader",
		ToolName:  "delete_file",
		Arguments: map[string]interface{}{"file_id": "file-1"},
		ExecutionContext: ExecutionContext{
			ConversationID: "conversation-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance": map[string]interface{}{
					"permission_tier": "basic",
					"assets": []map[string]interface{}{
						{"id": "file-1", "type": "file", "name": "report.pdf", "workspace_id": "workspace-1"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("DecideSkillTool() error = %v", err)
	}
	if len(decision.Assets) != 1 || decision.Assets[0].ID != "file-1" || decision.Assets[0].Name != "report.pdf" || decision.Assets[0].WorkspaceID != "workspace-1" {
		t.Fatalf("assets = %#v, want enriched file-1/report.pdf/workspace-1", decision.Assets)
	}
}

func TestPolicyToolGovernancePrefersRuntimeAssetNameWhenIDMatches(t *testing.T) {
	gateway := NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy())
	decision, err := gateway.DecideSkillTool(context.Background(), ToolGovernanceRequest{
		Manifest: toolgovernance.Manifest{
			ToolID:                  "file.delete",
			Domain:                  "files",
			Effect:                  toolgovernance.EffectDelete,
			AssetType:               "file",
			RiskLevel:               toolgovernance.RiskLevelHigh,
			RequiresAssetResolution: true,
		},
		SkillID:   "file-reader",
		ToolName:  "delete_file",
		Arguments: map[string]interface{}{"file_id": "file-1", "file_name": "Read file"},
		ExecutionContext: ExecutionContext{
			ConversationID: "conversation-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance": map[string]interface{}{
					"permission_tier": "basic",
					"assets": []map[string]interface{}{
						{"id": "file-1", "type": "file", "name": "codex-smoke.txt", "workspace_id": "workspace-1"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("DecideSkillTool() error = %v", err)
	}
	if len(decision.Assets) != 1 || decision.Assets[0].ID != "file-1" || decision.Assets[0].Name != "codex-smoke.txt" {
		t.Fatalf("assets = %#v, want trusted runtime asset name", decision.Assets)
	}
}

func TestPolicyToolGovernancePrefersRecentAgentUpdateOverVisibleSnapshot(t *testing.T) {
	gateway := NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy())
	decision, err := gateway.DecideSkillTool(context.Background(), ToolGovernanceRequest{
		Manifest: toolgovernance.Manifest{
			ToolID:                  "agent.update_config",
			Domain:                  "agents",
			Effect:                  toolgovernance.EffectUpdate,
			AssetType:               "agent",
			RiskLevel:               toolgovernance.RiskLevelMedium,
			RequiresAssetResolution: true,
		},
		SkillID:  "agent-management",
		ToolName: "update_agent_config",
		Arguments: map[string]interface{}{
			"agent_id":   "agent-1",
			"agent_name": "Fresh Agent",
			"agents": []map[string]interface{}{{
				"agent_id":   "agent-1",
				"name":       "Fresh Agent",
				"agent_name": "Fresh Agent",
			}},
			"home_title": "Updated Home",
		},
		ExecutionContext: ExecutionContext{
			ConversationID: "conversation-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance_permission_tier": "basic",
				"console_agents_recent_agent_updates": []map[string]interface{}{{
					"id":           "agent-1",
					"name":         "Fresh Agent",
					"agent_name":   "Fresh Agent",
					"workspace_id": "workspace-1",
				}},
				"console_agents_visible_agents": []map[string]interface{}{{
					"id":           "agent-1",
					"name":         "Stale Visible Agent",
					"agent_name":   "Stale Visible Agent",
					"workspace_id": "workspace-1",
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("DecideSkillTool() error = %v", err)
	}
	if len(decision.Assets) != 1 || decision.Assets[0].ID != "agent-1" || decision.Assets[0].Name != "Fresh Agent" {
		t.Fatalf("assets = %#v, want recent Agent update name", decision.Assets)
	}
}

func TestCallSkillToolMatchingSessionGrantAllowsEnginePath(t *testing.T) {
	runtime, resolved := governedRuntimeForTest(t)
	_, err := runtime.CallSkillTool(
		context.Background(),
		resolved,
		"governed-files",
		"delete_file",
		map[string]interface{}{"file_id": "file-1"},
		ExecutionContext{
			OrganizationID: "organization-1",
			UserID:         "user-1",
			ConversationID: "conversation-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance": map[string]interface{}{
					"permission_tier": "basic",
					"assets":          []map[string]interface{}{{"id": "file-1", "type": "file"}},
					"session_grants": []map[string]interface{}{
						{
							"conversation_id": "conversation-1",
							"organization_id": "organization-1",
							"user_id":         "user-1",
							"skill_id":        "governed-files",
							"provider_type":   "builtin",
							"provider_id":     "files",
							"tool_id":         "file.delete",
							"effect":          "delete",
							"asset_type":      "file",
							"assets":          []map[string]interface{}{{"id": "file-1", "type": "file"}},
							"risk_level":      "high",
							"expires_at":      time.Now().Add(time.Hour).Format(time.RFC3339),
						},
					},
				},
			},
		},
		"call_delete",
	)
	if err == nil || !strings.Contains(err.Error(), "tool engine is not configured") {
		t.Fatalf("CallSkillTool() error = %v, want fallthrough to engine path", err)
	}
}

func TestPolicyToolGovernanceMatchingSessionGrantCarriesApprovalCorrelation(t *testing.T) {
	gateway := NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy())
	decision, err := gateway.DecideSkillTool(context.Background(), ToolGovernanceRequest{
		Manifest: toolgovernance.Manifest{
			ToolID:                  "file.delete",
			Domain:                  "files",
			Effect:                  toolgovernance.EffectDelete,
			AssetType:               "file",
			RiskLevel:               toolgovernance.RiskLevelHigh,
			RequiresAssetResolution: true,
		},
		SkillID:      "file-reader",
		ToolName:     "delete_file",
		ProviderType: tools.ToolProviderTypeBuiltin,
		ProviderID:   "files",
		Arguments:    map[string]interface{}{"file_id": "file-1"},
		ExecutionContext: ExecutionContext{
			OrganizationID: "organization-1",
			UserID:         "user-1",
			ConversationID: "conversation-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance": map[string]interface{}{
					"permission_tier": "basic",
					"assets":          []map[string]interface{}{{"id": "file-1", "type": "file"}},
					"session_grants": []map[string]interface{}{
						{
							"conversation_id":         "conversation-1",
							"organization_id":         "organization-1",
							"user_id":                 "user-1",
							"skill_id":                "file-reader",
							"provider_type":           "builtin",
							"provider_id":             "files",
							"tool_id":                 "file.delete",
							"effect":                  "delete",
							"asset_type":              "file",
							"assets":                  []map[string]interface{}{{"id": "file-1", "type": "file", "name": "report.pdf"}},
							"risk_level":              "high",
							"approval_correlation_id": "approval-corr-1",
							"expires_at":              time.Now().Add(time.Hour).Format(time.RFC3339),
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("DecideSkillTool() error = %v", err)
	}
	if decision.Status != toolgovernance.DecisionStatusAllowed {
		t.Fatalf("decision status = %s, want allowed: %#v", decision.Status, decision)
	}
	if decision.ApprovedByCorrelationID != "approval-corr-1" {
		t.Fatalf("approved_by_correlation_id = %q, want approval-corr-1", decision.ApprovedByCorrelationID)
	}
	if decision.MatchedGrant == nil || decision.MatchedGrant.ApprovalCorrelationID != "approval-corr-1" {
		t.Fatalf("matched grant = %#v, want approval correlation", decision.MatchedGrant)
	}
	if len(decision.MatchedGrant.Assets) != 1 || decision.MatchedGrant.Assets[0].ID != "file-1" || decision.MatchedGrant.Assets[0].Name != "report.pdf" {
		t.Fatalf("matched grant assets = %#v, want approved asset", decision.MatchedGrant.Assets)
	}
}

func TestPolicyToolGovernanceMatchingSessionGrantRequiresApprovalForDifferentRuntimeAsset(t *testing.T) {
	gateway := NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy())
	decision, err := gateway.DecideSkillTool(context.Background(), ToolGovernanceRequest{
		Manifest: toolgovernance.Manifest{
			ToolID:                  "file.delete",
			Domain:                  "files",
			Effect:                  toolgovernance.EffectDelete,
			AssetType:               "file",
			RiskLevel:               toolgovernance.RiskLevelHigh,
			RequiresAssetResolution: true,
		},
		SkillID:      "file-reader",
		ToolName:     "delete_file",
		ProviderType: tools.ToolProviderTypeBuiltin,
		ProviderID:   "files",
		Arguments:    map[string]interface{}{"file_id": "file-2"},
		ExecutionContext: ExecutionContext{
			OrganizationID: "organization-1",
			UserID:         "user-1",
			ConversationID: "conversation-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance": map[string]interface{}{
					"permission_tier": "basic",
					"assets":          []map[string]interface{}{{"id": "file-2", "type": "file", "name": "other.pdf"}},
					"session_grants": []map[string]interface{}{
						{
							"conversation_id":         "conversation-1",
							"organization_id":         "organization-1",
							"user_id":                 "user-1",
							"skill_id":                "file-reader",
							"provider_type":           "builtin",
							"provider_id":             "files",
							"tool_id":                 "file.delete",
							"effect":                  "delete",
							"asset_type":              "file",
							"assets":                  []map[string]interface{}{{"id": "file-1", "type": "file", "name": "report.pdf"}},
							"risk_level":              "high",
							"approval_correlation_id": "approval-corr-1",
							"expires_at":              time.Now().Add(time.Hour).Format(time.RFC3339),
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("DecideSkillTool() error = %v", err)
	}
	if decision.Status != toolgovernance.DecisionStatusNeedsApproval {
		t.Fatalf("decision status = %s, want needs_approval: %#v", decision.Status, decision)
	}
	if decision.MatchedGrant != nil || decision.ApprovedByCorrelationID != "" {
		t.Fatalf("decision matched mismatched session grant: %#v", decision)
	}
}

func TestCallSkillToolGovernancePreflightsRunScriptBeforeRunner(t *testing.T) {
	scriptRunner := &governedScriptRunnerForTest{}
	runtime := NewRuntimeWithCatalog(nil, nil, "").WithScriptRunner(scriptRunner).WithToolGovernanceGateway(NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy()))
	resolved := &ResolvedSkills{Skills: []SkillDocument{governedScriptSkillForTest()}}

	invocation, err := runtime.CallSkillTool(
		context.Background(),
		resolved,
		"governed-script",
		SkillScriptToolRun,
		map[string]interface{}{"input": "hello"},
		ExecutionContext{ConversationID: "conversation-1"},
		"call_script",
	)
	if err != nil {
		t.Fatalf("CallSkillTool() error = %v", err)
	}
	if invocation == nil || invocation.Trace.Kind != "tool_governance" {
		t.Fatalf("invocation = %#v, want governance preflight result", invocation)
	}
	if invocation.Trace.Status != string(toolgovernance.DecisionStatusNeedsApproval) {
		t.Fatalf("trace status = %s, want needs_approval", invocation.Trace.Status)
	}
	if scriptRunner.calls != 0 {
		t.Fatalf("script runner calls = %d, want 0 before approval", scriptRunner.calls)
	}
}

func TestCallSkillToolGovernanceAllowsRunScriptWithMatchingGrant(t *testing.T) {
	scriptRunner := &governedScriptRunnerForTest{}
	runtime := NewRuntimeWithCatalog(nil, nil, "").WithScriptRunner(scriptRunner).WithToolGovernanceGateway(NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy()))
	resolved := &ResolvedSkills{Skills: []SkillDocument{governedScriptSkillForTest()}}

	invocation, err := runtime.CallSkillTool(
		context.Background(),
		resolved,
		"governed-script",
		SkillScriptToolRun,
		map[string]interface{}{"input": "hello"},
		ExecutionContext{
			OrganizationID: "organization-1",
			UserID:         "user-1",
			ConversationID: "conversation-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance": map[string]interface{}{
					"session_grants": []map[string]interface{}{
						{
							"conversation_id":         "conversation-1",
							"organization_id":         "organization-1",
							"user_id":                 "user-1",
							"skill_id":                "governed-script",
							"provider_type":           "builtin",
							"provider_id":             "skill-script",
							"tool_id":                 "skill.run_script",
							"effect":                  "invoke",
							"asset_type":              "script",
							"risk_level":              "high",
							"approval_correlation_id": "approval-corr-1",
							"expires_at":              time.Now().Add(time.Hour).Format(time.RFC3339),
						},
					},
				},
			},
		},
		"call_script",
	)
	if err != nil {
		t.Fatalf("CallSkillTool() error = %v", err)
	}
	if invocation == nil || invocation.Trace.Kind != "tool_call" || invocation.Trace.Status != "success" {
		t.Fatalf("invocation = %#v, want successful script tool call", invocation)
	}
	if scriptRunner.calls != 1 {
		t.Fatalf("script runner calls = %d, want 1 after matching grant", scriptRunner.calls)
	}
}

func governedRuntimeForTest(t *testing.T) (*Runtime, *ResolvedSkills) {
	t.Helper()
	catalogDir := t.TempDir()
	root := filepath.Join(catalogDir, "governed-files")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	skill := `---
name: governed-files
description: Governed files test skill
when_to_use: Use when testing governance preflight.
provider_type: builtin
provider_id: files
tools:
  - read_file
  - delete_file
runtime_type: tool
tool_governance:
  read_file:
    tool_id: file.read
    skill_id: governed-files
    domain: files
    effect: read
    asset_type: file
    risk_level: low
    requires_asset_resolution: true
    permission_scopes:
      - file:read
    default_approval_policy: auto_by_permission_tier
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  delete_file:
    tool_id: file.delete
    skill_id: governed-files
    domain: files
    effect: delete
    asset_type: file
    risk_level: high
    requires_asset_resolution: true
    permission_scopes:
      - file:manage
    default_approval_policy: always_ask
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
---
Use file tools.
`
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(skill), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	runtime := NewRuntimeWithCatalog(nil, nil, catalogDir).WithToolGovernanceGateway(NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy()))
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{"governed-files"})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	return runtime, resolved
}

func governedRuntimeWithReadToolForTest(t *testing.T) (*Runtime, *ResolvedSkills, *governedReadToolForTest) {
	t.Helper()
	catalogDir := t.TempDir()
	root := filepath.Join(catalogDir, "governed-files")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	skill := `---
name: governed-files
description: Governed files test skill
when_to_use: Use when testing governance preflight.
provider_type: builtin
provider_id: governed-files-test
tools:
  - read_file
runtime_type: tool
tool_governance:
  read_file:
    tool_id: file.read
    skill_id: governed-files
    domain: files
    effect: read
    asset_type: file
    risk_level: low
    requires_asset_resolution: true
    permission_scopes:
      - file:read
    default_approval_policy: auto_by_permission_tier
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
---
Use file tools.
`
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(skill), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	readTool := &governedReadToolForTest{}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(&governedReadProviderForTest{tool: readTool}); err != nil {
		t.Fatalf("RegisterProvider() error = %v", err)
	}
	runtime := NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir).WithToolGovernanceGateway(NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy()))
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{"governed-files"})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	return runtime, resolved, readTool
}

type governedReadProviderForTest struct {
	tool *governedReadToolForTest
}

func (p *governedReadProviderForTest) GetEntity() tools.ToolProviderEntity {
	return tools.ToolProviderEntity{
		Identity: tools.ToolProviderIdentity{
			Name:        "governed-files-test",
			Label:       tools.I18nText{"en_US": "Governed Files Test"},
			Description: tools.I18nText{"en_US": "Governed files test provider"},
		},
		ProviderType: tools.ToolProviderTypeBuiltin,
	}
}

func (p *governedReadProviderForTest) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeBuiltin
}

func (p *governedReadProviderForTest) GetTool(name string) (tools.Tool, error) {
	if name != "read_file" {
		return nil, tools.ErrToolNotFound
	}
	return p.tool, nil
}

func (p *governedReadProviderForTest) GetTools() []tools.Tool {
	return []tools.Tool{p.tool}
}

func (p *governedReadProviderForTest) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

type governedReadToolForTest struct {
	calls  []string
	enrich func(map[string]interface{}) map[string]interface{}
}

func (t *governedReadToolForTest) GetEntity() tools.ToolEntity {
	return tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "read_file",
			Provider: "governed-files-test",
			Label:    tools.I18nText{"en_US": "Read File"},
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{"en_US": "Read a file"},
			LLM:   "Read the file identified by file_id.",
		},
		Parameters: []tools.ToolParameter{
			{
				Name:     "file_id",
				Label:    tools.I18nText{"en_US": "File ID"},
				Type:     tools.ToolParameterTypeString,
				Form:     tools.ToolParameterFormLLM,
				Required: true,
			},
		},
	}
}

func (t *governedReadToolForTest) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeBuiltin
}

func (t *governedReadToolForTest) GetTenantID() string {
	return ""
}

func (t *governedReadToolForTest) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolInvokeMessage, error) {
	_ = ctx
	_ = userID
	_ = conversationID
	_ = appID
	_ = messageID
	fileID, _ := toolParameters["file_id"].(string)
	t.calls = append(t.calls, strings.TrimSpace(fileID))
	return []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"file_id":        fileID,
			"content_status": "extracted",
			"content":        "test file content",
		},
	}}, nil
}

func (t *governedReadToolForTest) EnrichGovernanceArguments(ctx context.Context, userID string, toolParameters map[string]interface{}) map[string]interface{} {
	_ = ctx
	_ = userID
	if t.enrich == nil {
		return toolParameters
	}
	return t.enrich(toolParameters)
}

func (t *governedReadToolForTest) GetRuntimeParameters(
	context.Context,
	*string,
	*string,
	*string,
) ([]tools.ToolParameter, error) {
	return nil, nil
}

func (t *governedReadToolForTest) ForkToolRuntime(*tools.ToolRuntime) tools.Tool {
	return t
}

func (t *governedReadToolForTest) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

func governedScriptSkillForTest() SkillDocument {
	return SkillDocument{
		Metadata: SkillMetadata{
			ID:               "governed-script",
			Name:             "governed-script",
			Description:      "Governed script test skill",
			WhenToUse:        "Use when testing governed scripts.",
			RuntimeType:      SkillRuntimeTypeTool,
			HasScripts:       true,
			ScriptsSupported: true,
		},
		Instructions: "Run governed script.",
		Tools: []SkillToolDefinition{{
			Name:         SkillScriptToolRun,
			ProviderType: tools.ToolProviderTypeBuiltin,
			ProviderID:   "skill-script",
			Governance: &toolgovernance.Manifest{
				ToolID:                 "skill.run_script",
				SkillID:                "governed-script",
				Domain:                 "skills",
				Effect:                 toolgovernance.EffectInvoke,
				AssetType:              "script",
				RiskLevel:              toolgovernance.RiskLevelHigh,
				PermissionScopes:       []string{"skill:run"},
				DefaultApprovalPolicy:  toolgovernance.ApprovalPolicyAlwaysAsk,
				AllowedPermissionTiers: []toolgovernance.PermissionTier{toolgovernance.PermissionTierBasic, toolgovernance.PermissionTierAdvanced, toolgovernance.PermissionTierFull},
				AuditRequired:          true,
			},
		}},
	}
}

type governedScriptRunnerForTest struct {
	calls int
}

func (r *governedScriptRunnerForTest) RunSkillScript(ctx context.Context, doc SkillDocument, arguments map[string]interface{}, execCtx ExecutionContext, callID string) (*ToolInvocationResult, error) {
	_ = ctx
	_ = doc
	_ = arguments
	_ = execCtx
	_ = callID
	r.calls++
	return &ToolInvocationResult{
		Messages: []tools.ToolInvokeMessage{{
			Type: tools.ToolInvokeMessageTypeJSON,
			Data: map[string]interface{}{"status": "ok"},
		}},
		Trace: SkillTrace{
			Kind:     "tool_call",
			SkillID:  "governed-script",
			ToolName: SkillScriptToolRun,
			Status:   "success",
		},
	}, nil
}

func (r *governedScriptRunnerForTest) Configured() bool {
	return true
}
