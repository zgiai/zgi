package skills

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestFileReaderSystemSkillGovernanceManifest(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{"file-reader"})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get("file-reader")
	if !ok {
		t.Fatalf("file-reader skill was not resolved")
	}
	if !sameStrings(doc.Metadata.SupportedCallers, []string{SkillCallerAIChat}) {
		t.Fatalf("supported callers = %#v, want aichat", doc.Metadata.SupportedCallers)
	}
	if got := toolNames(doc.Tools); !sameStrings(got, []string{"read_file", "delete_file"}) {
		t.Fatalf("file-reader tools = %v, want read_file/delete_file", got)
	}
	readTool, ok := findSkillTool(*doc, "read_file")
	if !ok {
		t.Fatalf("read_file tool not found")
	}
	if readTool.ProviderType != tools.ToolProviderTypeBuiltin || readTool.ProviderID != "files" {
		t.Fatalf("read_file provider = %s/%s, want builtin/files", readTool.ProviderType, readTool.ProviderID)
	}
	if readTool.Governance == nil {
		t.Fatalf("read_file governance manifest missing")
	}
	if readTool.Governance.ToolID != "file.read" {
		t.Fatalf("tool_id = %q, want file.read", readTool.Governance.ToolID)
	}
	if readTool.Governance.Effect != toolgovernance.EffectRead {
		t.Fatalf("effect = %q, want read", readTool.Governance.Effect)
	}
	if readTool.Governance.AssetType != "file" {
		t.Fatalf("asset_type = %q, want file", readTool.Governance.AssetType)
	}
	if readTool.Governance.RiskLevel != toolgovernance.RiskLevelLow {
		t.Fatalf("risk_level = %q, want low", readTool.Governance.RiskLevel)
	}
	if !readTool.Governance.RequiresAssetResolution {
		t.Fatalf("requires_asset_resolution = false, want true")
	}
	if got := readTool.Governance.PermissionScopes; len(got) != 1 || got[0] != "file:read" {
		t.Fatalf("permission_scopes = %#v, want file:read", got)
	}
	if readTool.Governance.DefaultApprovalPolicy != toolgovernance.ApprovalPolicyAutoByPermissionTier {
		t.Fatalf("default_approval_policy = %q, want auto_by_permission_tier", readTool.Governance.DefaultApprovalPolicy)
	}
	if got := readTool.Governance.AllowedPermissionTiers; len(got) != 3 || got[0] != toolgovernance.PermissionTierBasic || got[1] != toolgovernance.PermissionTierAdvanced || got[2] != toolgovernance.PermissionTierFull {
		t.Fatalf("allowed_permission_tiers = %#v, want basic/advanced/full", got)
	}
	if !readTool.Governance.AuditRequired {
		t.Fatalf("audit_required = false, want true")
	}
	deleteTool, ok := findSkillTool(*doc, "delete_file")
	if !ok {
		t.Fatalf("delete_file tool not found")
	}
	if deleteTool.ProviderType != tools.ToolProviderTypeBuiltin || deleteTool.ProviderID != "files" {
		t.Fatalf("delete_file provider = %s/%s, want builtin/files", deleteTool.ProviderType, deleteTool.ProviderID)
	}
	if deleteTool.Governance == nil {
		t.Fatalf("delete_file governance manifest missing")
	}
	if deleteTool.Governance.ToolID != "file.delete" {
		t.Fatalf("delete tool_id = %q, want file.delete", deleteTool.Governance.ToolID)
	}
	if deleteTool.Governance.Effect != toolgovernance.EffectDelete {
		t.Fatalf("delete effect = %q, want delete", deleteTool.Governance.Effect)
	}
	if deleteTool.Governance.AssetType != "file" {
		t.Fatalf("delete asset_type = %q, want file", deleteTool.Governance.AssetType)
	}
	if deleteTool.Governance.RiskLevel != toolgovernance.RiskLevelHigh {
		t.Fatalf("delete risk_level = %q, want high", deleteTool.Governance.RiskLevel)
	}
	if !deleteTool.Governance.RequiresAssetResolution {
		t.Fatalf("delete requires_asset_resolution = false, want true")
	}
	if deleteTool.Governance.DefaultApprovalPolicy != toolgovernance.ApprovalPolicyAlwaysAsk {
		t.Fatalf("delete default_approval_policy = %q, want always_ask", deleteTool.Governance.DefaultApprovalPolicy)
	}
	if got := deleteTool.Governance.PermissionScopes; len(got) != 1 || got[0] != "file:manage" {
		t.Fatalf("delete permission_scopes = %#v, want file:manage", got)
	}
	if !deleteTool.Governance.AuditRequired {
		t.Fatalf("delete audit_required = false, want true")
	}
}
