package skills

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
)

func TestSystemAssetToolsDeclareGovernance(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	expected := []struct {
		skillID                 string
		toolName                string
		effect                  toolgovernance.Effect
		assetType               string
		riskLevel               toolgovernance.RiskLevel
		requiresAssetResolution bool
	}{
		{SkillFileReader, "list_visible_files", toolgovernance.EffectRead, "file", toolgovernance.RiskLevelLow, false},
		{SkillFileReader, "read_file", toolgovernance.EffectRead, "file", toolgovernance.RiskLevelLow, true},
		{SkillFileManager, "delete_file", toolgovernance.EffectDelete, "file", toolgovernance.RiskLevelHigh, true},
		{SkillFileGenerator, "generate_file", toolgovernance.EffectCreate, "file", toolgovernance.RiskLevelMedium, false},
		{SkillFileGenerator, "generate_docx", toolgovernance.EffectCreate, "file", toolgovernance.RiskLevelMedium, false},
		{SkillFileGenerator, "generate_pdf", toolgovernance.EffectCreate, "file", toolgovernance.RiskLevelMedium, false},
		{SkillFileGenerator, "generate_pptx", toolgovernance.EffectCreate, "file", toolgovernance.RiskLevelMedium, false},
		{SkillChartGenerator, "generate_chart", toolgovernance.EffectCreate, "file", toolgovernance.RiskLevelMedium, false},
		{SkillWorkReport, "generate_file", toolgovernance.EffectCreate, "file", toolgovernance.RiskLevelMedium, false},
		{SkillInternalKnowledge, "list_accessible_knowledge_bases", toolgovernance.EffectRead, "knowledge_base", toolgovernance.RiskLevelLow, false},
		{SkillInternalKnowledge, "retrieve_knowledge", toolgovernance.EffectRead, "knowledge_base", toolgovernance.RiskLevelLow, true},
		{SkillAgentKnowledge, "retrieve_agent_knowledge", toolgovernance.EffectRead, "knowledge_base", toolgovernance.RiskLevelLow, false},
		{SkillInternalDatabase, "list_accessible_databases", toolgovernance.EffectRead, "database", toolgovernance.RiskLevelLow, false},
		{SkillInternalDatabase, "list_database_tables", toolgovernance.EffectRead, "database", toolgovernance.RiskLevelLow, true},
		{SkillInternalDatabase, "describe_database_table", toolgovernance.EffectRead, "database_table", toolgovernance.RiskLevelLow, true},
		{SkillInternalDatabase, "query_table_records", toolgovernance.EffectRead, "database_table", toolgovernance.RiskLevelLow, true},
		{SkillInternalDatabase, "insert_table_records", toolgovernance.EffectCreate, "database_table", toolgovernance.RiskLevelMedium, true},
		{SkillInternalDatabase, "update_table_records", toolgovernance.EffectUpdate, "database_table", toolgovernance.RiskLevelMedium, true},
		{SkillInternalDatabase, "delete_table_records", toolgovernance.EffectDelete, "database_table", toolgovernance.RiskLevelHigh, true},
		{SkillAgentDatabase, "list_accessible_databases", toolgovernance.EffectRead, "database", toolgovernance.RiskLevelLow, false},
		{SkillAgentDatabase, "list_database_tables", toolgovernance.EffectRead, "database", toolgovernance.RiskLevelLow, true},
		{SkillAgentDatabase, "describe_database_table", toolgovernance.EffectRead, "database_table", toolgovernance.RiskLevelLow, true},
		{SkillAgentDatabase, "query_table_records", toolgovernance.EffectRead, "database_table", toolgovernance.RiskLevelLow, true},
		{SkillAgentDatabase, "insert_table_records", toolgovernance.EffectCreate, "database_table", toolgovernance.RiskLevelMedium, true},
		{SkillAgentDatabase, "update_table_records", toolgovernance.EffectUpdate, "database_table", toolgovernance.RiskLevelMedium, true},
		{SkillAgentDatabase, "delete_table_records", toolgovernance.EffectDelete, "database_table", toolgovernance.RiskLevelHigh, true},
		{SkillAgentWorkflow, "list_agent_workflows", toolgovernance.EffectRead, "workflow", toolgovernance.RiskLevelLow, false},
		{SkillAgentWorkflow, "run_agent_workflow", toolgovernance.EffectInvoke, "workflow", toolgovernance.RiskLevelHigh, true},
		{SkillAgentWorkflow, "get_workflow_run_status", toolgovernance.EffectRead, "workflow_run", toolgovernance.RiskLevelLow, true},
	}

	resolved, err := runtime.ResolveEnabledSkills(context.Background(), uniqueGovernanceSkillIDs(expected))
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	for _, tt := range expected {
		t.Run(tt.skillID+"/"+tt.toolName, func(t *testing.T) {
			doc, ok := resolved.Get(tt.skillID)
			if !ok {
				t.Fatalf("skill %s was not resolved", tt.skillID)
			}
			tool, ok := findSkillTool(*doc, tt.toolName)
			if !ok {
				t.Fatalf("tool %s not found", tt.toolName)
			}
			if tool.Governance == nil {
				t.Fatalf("%s/%s governance manifest missing", tt.skillID, tt.toolName)
			}
			manifest := tool.Governance
			if manifest.SkillID != tt.skillID {
				t.Fatalf("skill_id = %q, want %q", manifest.SkillID, tt.skillID)
			}
			if manifest.Effect != tt.effect || manifest.AssetType != tt.assetType || manifest.RiskLevel != tt.riskLevel {
				t.Fatalf("governance = %#v, want effect=%s asset_type=%s risk=%s", manifest, tt.effect, tt.assetType, tt.riskLevel)
			}
			if manifest.RequiresAssetResolution != tt.requiresAssetResolution {
				t.Fatalf("requires_asset_resolution = %v, want %v", manifest.RequiresAssetResolution, tt.requiresAssetResolution)
			}
			if len(manifest.PermissionScopes) == 0 {
				t.Fatalf("permission_scopes missing: %#v", manifest)
			}
			if !manifest.AuditRequired {
				t.Fatalf("audit_required = false, want true")
			}
			if !sameStrings(permissionTierStrings(manifest.AllowedPermissionTiers), []string{"basic", "advanced", "full"}) {
				t.Fatalf("allowed_permission_tiers = %#v, want basic/advanced/full", manifest.AllowedPermissionTiers)
			}
		})
	}
}

func TestGovernanceAssetRefsFromToolArguments(t *testing.T) {
	tests := []struct {
		name      string
		manifest  toolgovernance.Manifest
		arguments map[string]interface{}
		want      []toolgovernance.AssetRef
	}{
		{
			name:     "knowledge dataset ids",
			manifest: toolgovernance.Manifest{AssetType: "knowledge_base"},
			arguments: map[string]interface{}{
				"dataset_ids": []interface{}{"kb-1", "kb-2"},
			},
			want: []toolgovernance.AssetRef{
				{ID: "kb-1", Type: "knowledge_base", Source: "tool_arguments"},
				{ID: "kb-2", Type: "knowledge_base", Source: "tool_arguments"},
			},
		},
		{
			name:     "database table with datasource and records",
			manifest: toolgovernance.Manifest{AssetType: "database_table"},
			arguments: map[string]interface{}{
				"data_source_id": "db-1",
				"table_id":       "table-1",
				"records":        []interface{}{map[string]interface{}{"id": "row-1"}, map[string]interface{}{"id": "row-2"}},
			},
			want: []toolgovernance.AssetRef{
				{ID: "table-1", Type: "database_table", Source: "tool_arguments", Metadata: map[string]interface{}{"data_source_id": "db-1", "record_count": 2}},
			},
		},
		{
			name:     "workflow binding",
			manifest: toolgovernance.Manifest{AssetType: "workflow"},
			arguments: map[string]interface{}{
				"binding_id":   "binding-1",
				"binding_name": "Approval Flow",
			},
			want: []toolgovernance.AssetRef{
				{ID: "binding-1", Type: "workflow", Name: "Approval Flow", Source: "tool_arguments"},
			},
		},
		{
			name:     "generated chart file name",
			manifest: toolgovernance.Manifest{AssetType: "file"},
			arguments: map[string]interface{}{
				"output_filename": "score-chart",
				"chart_type":      "bar",
			},
			want: []toolgovernance.AssetRef{
				{Name: "score-chart", Type: "file", Source: "tool_arguments", Metadata: map[string]interface{}{"format": "bar"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := assetRefsFromToolArguments(tt.manifest, tt.arguments)
			if len(got) != len(tt.want) {
				t.Fatalf("assetRefsFromToolArguments() = %#v, want %#v", got, tt.want)
			}
			for idx := range tt.want {
				if got[idx].ID != tt.want[idx].ID || got[idx].Type != tt.want[idx].Type || got[idx].Name != tt.want[idx].Name || got[idx].Source != tt.want[idx].Source {
					t.Fatalf("asset[%d] = %#v, want %#v", idx, got[idx], tt.want[idx])
				}
				for key, wantValue := range tt.want[idx].Metadata {
					if got[idx].Metadata[key] != wantValue {
						t.Fatalf("asset[%d].metadata[%s] = %#v, want %#v; asset=%#v", idx, key, got[idx].Metadata[key], wantValue, got[idx])
					}
				}
			}
		})
	}
}

func uniqueGovernanceSkillIDs(items []struct {
	skillID                 string
	toolName                string
	effect                  toolgovernance.Effect
	assetType               string
	riskLevel               toolgovernance.RiskLevel
	requiresAssetResolution bool
}) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, item := range items {
		if _, ok := seen[item.skillID]; ok {
			continue
		}
		seen[item.skillID] = struct{}{}
		out = append(out, item.skillID)
	}
	return out
}

func permissionTierStrings(tiers []toolgovernance.PermissionTier) []string {
	out := make([]string, 0, len(tiers))
	for _, tier := range tiers {
		out = append(out, string(tier))
	}
	return out
}
