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

func TestPolicyToolGovernanceToolArgumentsOverrideRuntimeAssets(t *testing.T) {
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
	if decision.Status != toolgovernance.DecisionStatusNeedsApproval {
		t.Fatalf("decision status = %s, want needs_approval: %#v", decision.Status, decision)
	}
	if len(decision.Assets) != 1 || decision.Assets[0].ID != "file-2" || decision.Assets[0].Name != "target.pdf" {
		t.Fatalf("assets = %#v, want file-2/target.pdf from tool arguments", decision.Assets)
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

func TestCallSkillToolMatchingSessionGrantAllowsEnginePath(t *testing.T) {
	runtime, resolved := governedRuntimeForTest(t)
	_, err := runtime.CallSkillTool(
		context.Background(),
		resolved,
		"governed-files",
		"delete_file",
		map[string]interface{}{"file_id": "file-1"},
		ExecutionContext{
			ConversationID: "conversation-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance": map[string]interface{}{
					"permission_tier": "basic",
					"assets":          []map[string]interface{}{{"id": "file-1", "type": "file"}},
					"session_grants": []map[string]interface{}{
						{
							"conversation_id": "conversation-1",
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
		SkillID:   "file-reader",
		ToolName:  "delete_file",
		Arguments: map[string]interface{}{"file_id": "file-1"},
		ExecutionContext: ExecutionContext{
			ConversationID: "conversation-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance": map[string]interface{}{
					"permission_tier": "basic",
					"assets":          []map[string]interface{}{{"id": "file-1", "type": "file"}},
					"session_grants": []map[string]interface{}{
						{
							"conversation_id":         "conversation-1",
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

func TestPolicyToolGovernanceMatchingSessionGrantAllowsDifferentRuntimeAsset(t *testing.T) {
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
		Arguments: map[string]interface{}{"file_id": "file-2"},
		ExecutionContext: ExecutionContext{
			ConversationID: "conversation-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance": map[string]interface{}{
					"permission_tier": "basic",
					"assets":          []map[string]interface{}{{"id": "file-2", "type": "file", "name": "other.pdf"}},
					"session_grants": []map[string]interface{}{
						{
							"conversation_id":         "conversation-1",
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
    domain: files
    effect: read
    asset_type: file
    risk_level: low
    requires_asset_resolution: true
    audit_required: true
  delete_file:
    tool_id: file.delete
    domain: files
    effect: delete
    asset_type: file
    risk_level: high
    requires_asset_resolution: true
    audit_required: true
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
