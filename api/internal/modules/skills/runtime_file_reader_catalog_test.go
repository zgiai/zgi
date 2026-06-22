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
	if got := docTimeoutSeconds(*doc); got != 120 {
		t.Fatalf("timeout_seconds = %d, want 120", got)
	}
	if got := toolNames(doc.Tools); !sameStrings(got, []string{"list_visible_files", "read_file"}) {
		t.Fatalf("file-reader tools = %v, want list_visible_files/read_file", got)
	}
	listTool, ok := findSkillTool(*doc, "list_visible_files")
	if !ok {
		t.Fatalf("list_visible_files tool not found")
	}
	if listTool.ProviderType != tools.ToolProviderTypeBuiltin || listTool.ProviderID != "files" {
		t.Fatalf("list_visible_files provider = %s/%s, want builtin/files", listTool.ProviderType, listTool.ProviderID)
	}
	if listTool.Governance == nil {
		t.Fatalf("list_visible_files governance manifest missing")
	}
	if listTool.Governance.ToolID != "file.list_visible" {
		t.Fatalf("list tool_id = %q, want file.list_visible", listTool.Governance.ToolID)
	}
	if listTool.Governance.Effect != toolgovernance.EffectRead {
		t.Fatalf("list effect = %q, want read", listTool.Governance.Effect)
	}
	if listTool.Governance.AssetType != "file" || listTool.Governance.RiskLevel != toolgovernance.RiskLevelLow {
		t.Fatalf("list governance = %#v, want low-risk file read", listTool.Governance)
	}
	if listTool.Governance.RequiresAssetResolution {
		t.Fatalf("list requires_asset_resolution = true, want false")
	}
	if got := listTool.Governance.PermissionScopes; len(got) != 1 || got[0] != "file:read" {
		t.Fatalf("list permission_scopes = %#v, want file:read", got)
	}
	if listTool.Governance.DefaultApprovalPolicy != toolgovernance.ApprovalPolicyAutoByPermissionTier {
		t.Fatalf("list default_approval_policy = %q, want auto_by_permission_tier", listTool.Governance.DefaultApprovalPolicy)
	}
	if !listTool.Governance.AuditRequired {
		t.Fatalf("list audit_required = false, want true")
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
}

func TestFileManagerSystemSkillGovernanceManifest(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillFileManager})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get(SkillFileManager)
	if !ok {
		t.Fatalf("file-manager skill was not resolved")
	}
	if !sameStrings(doc.Metadata.SupportedCallers, []string{SkillCallerAIChat}) {
		t.Fatalf("supported callers = %#v, want aichat", doc.Metadata.SupportedCallers)
	}
	if !IsHiddenSystemSkill(SkillFileManager) {
		t.Fatal("file-manager should be hidden from manual skill management")
	}
	if got := docTimeoutSeconds(*doc); got != 120 {
		t.Fatalf("timeout_seconds = %d, want 120", got)
	}
	if got := toolNames(doc.Tools); !sameStrings(got, []string{"delete_file", "save_file_to_management"}) {
		t.Fatalf("file-manager tools = %v, want delete_file and save_file_to_management", got)
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
	saveTool, ok := findSkillTool(*doc, "save_file_to_management")
	if !ok {
		t.Fatalf("save_file_to_management tool not found")
	}
	if saveTool.ProviderType != tools.ToolProviderTypeBuiltin || saveTool.ProviderID != "files" {
		t.Fatalf("save_file_to_management provider = %s/%s, want builtin/files", saveTool.ProviderType, saveTool.ProviderID)
	}
	if saveTool.Governance == nil {
		t.Fatalf("save_file_to_management governance manifest missing")
	}
	if saveTool.Governance.ToolID != "file.save_to_management" {
		t.Fatalf("save tool_id = %q, want file.save_to_management", saveTool.Governance.ToolID)
	}
	if saveTool.Governance.Effect != toolgovernance.EffectCreate {
		t.Fatalf("save effect = %q, want create", saveTool.Governance.Effect)
	}
	if saveTool.Governance.AssetType != "file" {
		t.Fatalf("save asset_type = %q, want file", saveTool.Governance.AssetType)
	}
	if saveTool.Governance.RiskLevel != toolgovernance.RiskLevelMedium {
		t.Fatalf("save risk_level = %q, want medium", saveTool.Governance.RiskLevel)
	}
	if saveTool.Governance.RequiresAssetResolution {
		t.Fatalf("save requires_asset_resolution = true, want false")
	}
	if saveTool.Governance.DefaultApprovalPolicy != toolgovernance.ApprovalPolicyAutoByPermissionTier {
		t.Fatalf("save default_approval_policy = %q, want auto_by_permission_tier", saveTool.Governance.DefaultApprovalPolicy)
	}
	if got := saveTool.Governance.PermissionScopes; len(got) != 1 || got[0] != "file:create" {
		t.Fatalf("save permission_scopes = %#v, want file:create", got)
	}
	if !saveTool.Governance.AuditRequired {
		t.Fatalf("save audit_required = false, want true")
	}
}

func TestFileReaderReadGovernanceAutoAllowsReadTools(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{"file-reader"})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get("file-reader")
	if !ok {
		t.Fatalf("file-reader skill was not resolved")
	}
	gateway := NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy())
	execCtx := ExecutionContext{
		ConversationID: "conversation-1",
		RuntimeParameters: map[string]interface{}{
			"tool_governance_permission_tier": string(toolgovernance.PermissionTierBasic),
		},
	}

	for _, tt := range []struct {
		name      string
		toolName  string
		arguments map[string]interface{}
	}{
		{
			name:     "list visible files",
			toolName: "list_visible_files",
		},
		{
			name:     "read file",
			toolName: "read_file",
			arguments: map[string]interface{}{
				"file_id":   "file-1",
				"file_name": "report.txt",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tool, ok := findSkillTool(*doc, tt.toolName)
			if !ok {
				t.Fatalf("%s tool not found", tt.toolName)
			}
			if tool.Governance == nil {
				t.Fatalf("%s governance manifest missing", tt.toolName)
			}
			decision, err := gateway.DecideSkillTool(context.Background(), ToolGovernanceRequest{
				Manifest:         *tool.Governance,
				SkillID:          SkillFileReader,
				ToolName:         tt.toolName,
				Arguments:        tt.arguments,
				ExecutionContext: execCtx,
			})
			if err != nil {
				t.Fatalf("DecideSkillTool() error = %v", err)
			}
			if decision.Status != toolgovernance.DecisionStatusAllowed {
				t.Fatalf("decision status = %s, want allowed: %#v", decision.Status, decision)
			}
			if decision.RequiresApproval {
				t.Fatalf("RequiresApproval = true, want false: %#v", decision)
			}
			if decision.ApprovalEvent != nil {
				t.Fatalf("ApprovalEvent = %#v, want nil", decision.ApprovalEvent)
			}
		})
	}

}

func TestFileManagerDeleteGovernanceNeedsApproval(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillFileManager})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get(SkillFileManager)
	if !ok {
		t.Fatalf("file-manager skill was not resolved")
	}
	gateway := NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy())
	execCtx := ExecutionContext{
		ConversationID: "conversation-1",
		RuntimeParameters: map[string]interface{}{
			"tool_governance_permission_tier": string(toolgovernance.PermissionTierBasic),
		},
	}
	deleteTool, ok := findSkillTool(*doc, "delete_file")
	if !ok {
		t.Fatalf("delete_file tool not found")
	}
	decision, err := gateway.DecideSkillTool(context.Background(), ToolGovernanceRequest{
		Manifest: *deleteTool.Governance,
		SkillID:  SkillFileManager,
		ToolName: "delete_file",
		Arguments: map[string]interface{}{
			"file_id":   "file-1",
			"file_name": "report.txt",
		},
		ExecutionContext: execCtx,
	})
	if err != nil {
		t.Fatalf("DecideSkillTool(delete_file) error = %v", err)
	}
	if decision.Status != toolgovernance.DecisionStatusNeedsApproval || !decision.RequiresApproval {
		t.Fatalf("delete decision = %#v, want needs_approval", decision)
	}
}

func TestFileManagerSaveGovernanceRequiresApprovalOnBasic(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillFileManager})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get(SkillFileManager)
	if !ok {
		t.Fatalf("file-manager skill was not resolved")
	}
	saveTool, ok := findSkillTool(*doc, "save_file_to_management")
	if !ok {
		t.Fatalf("save_file_to_management tool not found")
	}
	decision, err := NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy()).DecideSkillTool(context.Background(), ToolGovernanceRequest{
		Manifest: *saveTool.Governance,
		SkillID:  SkillFileManager,
		ToolName: "save_file_to_management",
		Arguments: map[string]interface{}{
			"source_type":  "tool_file",
			"tool_file_id": "tool-1",
			"filename":     "report.pdf",
		},
		ExecutionContext: ExecutionContext{
			ConversationID: "conversation-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance_permission_tier": string(toolgovernance.PermissionTierBasic),
			},
		},
	})
	if err != nil {
		t.Fatalf("DecideSkillTool(save_file_to_management) error = %v", err)
	}
	if decision.Status != toolgovernance.DecisionStatusNeedsApproval || !decision.RequiresApproval {
		t.Fatalf("save decision = %#v, want needs_approval", decision)
	}
}
