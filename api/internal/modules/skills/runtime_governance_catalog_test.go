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
		{SkillFileManager, "save_file_to_management", toolgovernance.EffectCreate, "file", toolgovernance.RiskLevelMedium, false},
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
		{SkillAgentManagement, "list_agents", toolgovernance.EffectRead, "agent", toolgovernance.RiskLevelLow, false},
		{SkillAgentManagement, "get_agent", toolgovernance.EffectRead, "agent", toolgovernance.RiskLevelLow, false},
		{SkillAgentManagement, "get_agent_config", toolgovernance.EffectRead, "agent", toolgovernance.RiskLevelLow, false},
		{SkillAgentManagement, "list_agent_skill_candidates", toolgovernance.EffectRead, "agent_skill", toolgovernance.RiskLevelLow, false},
		{SkillAgentManagement, "list_agent_knowledge_candidates", toolgovernance.EffectRead, "knowledge_base", toolgovernance.RiskLevelLow, false},
		{SkillAgentManagement, "list_agent_database_candidates", toolgovernance.EffectRead, "database_table", toolgovernance.RiskLevelLow, false},
		{SkillAgentManagement, "list_agent_database_tables", toolgovernance.EffectRead, "database_table", toolgovernance.RiskLevelLow, false},
		{SkillAgentManagement, "list_agent_workflow_binding_candidates", toolgovernance.EffectRead, "workflow", toolgovernance.RiskLevelLow, false},
		{SkillAgentManagement, "list_available_models", toolgovernance.EffectRead, "llm_model", toolgovernance.RiskLevelLow, false},
		{SkillAgentManagement, "create_agent", toolgovernance.EffectCreate, "agent", toolgovernance.RiskLevelMedium, false},
		{SkillAgentManagement, "update_agent_identity", toolgovernance.EffectUpdate, "agent", toolgovernance.RiskLevelMedium, true},
		{SkillAgentManagement, "update_agent_config", toolgovernance.EffectUpdate, "agent", toolgovernance.RiskLevelMedium, true},
		{SkillAgentManagement, "replace_agent_memory_slots", toolgovernance.EffectUpdate, "agent", toolgovernance.RiskLevelMedium, true},
		{SkillAgentManagement, "delete_agent", toolgovernance.EffectDelete, "agent", toolgovernance.RiskLevelHigh, true},
		{SkillAgentManagement, "delete_agents", toolgovernance.EffectDelete, "agent", toolgovernance.RiskLevelHigh, true},
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

func TestSystemSkillToolGovernanceManifestReadsCatalog(t *testing.T) {
	manifest, ok := SystemSkillToolGovernanceManifest(SkillAgentManagement, "list_available_models")
	if !ok {
		t.Fatal("SystemSkillToolGovernanceManifest(agent-management/list_available_models) = false, want true")
	}
	if manifest.Effect != toolgovernance.EffectRead || manifest.AssetType != "llm_model" {
		t.Fatalf("manifest = %#v, want read llm_model", manifest)
	}

	manifest, ok = SystemSkillToolGovernanceManifest(SkillAgentManagement, "get_agent_config")
	if !ok {
		t.Fatal("SystemSkillToolGovernanceManifest(agent-management/get_agent_config) = false, want true")
	}
	if manifest.Effect != toolgovernance.EffectRead || manifest.AssetType != "agent" {
		t.Fatalf("manifest = %#v, want read agent", manifest)
	}

	manifest, ok = SystemSkillToolGovernanceManifest(SkillAgentManagement, "delete_agents")
	if !ok {
		t.Fatal("SystemSkillToolGovernanceManifest(agent-management/delete_agents) = false, want true")
	}
	if manifest.Effect != toolgovernance.EffectDelete || manifest.AssetType != "agent" || !manifest.RequiresAssetResolution {
		t.Fatalf("manifest = %#v, want delete agent with asset resolution", manifest)
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
			name:     "agent batch targets",
			manifest: toolgovernance.Manifest{AssetType: "agent"},
			arguments: map[string]interface{}{
				"agents": []interface{}{
					map[string]interface{}{"agent_id": "agent-1", "name": "Agent One", "workspace_id": "workspace-1"},
					map[string]interface{}{"agent_id": "agent-2", "name": "Agent Two", "workspace_id": "workspace-1"},
				},
			},
			want: []toolgovernance.AssetRef{
				{ID: "agent-1", Type: "agent", Name: "Agent One", WorkspaceID: "workspace-1", Source: "tool_arguments"},
				{ID: "agent-2", Type: "agent", Name: "Agent Two", WorkspaceID: "workspace-1", Source: "tool_arguments"},
			},
		},
		{
			name:     "agent knowledge binding replacement",
			manifest: toolgovernance.Manifest{AssetType: "knowledge_base"},
			arguments: map[string]interface{}{
				"agent_id":   "agent-1",
				"agent_name": "Support Agent",
				"dataset_ids": []interface{}{
					"kb-1",
					"kb-2",
				},
			},
			want: []toolgovernance.AssetRef{
				{ID: "agent-1", Type: "agent", Name: "Support Agent", Source: "tool_arguments", Metadata: map[string]interface{}{"binding_owner": true}},
				{ID: "kb-1", Type: "knowledge_base", Source: "tool_arguments", Metadata: map[string]interface{}{"agent_id": "agent-1", "agent_name": "Support Agent"}},
				{ID: "kb-2", Type: "knowledge_base", Source: "tool_arguments", Metadata: map[string]interface{}{"agent_id": "agent-1", "agent_name": "Support Agent"}},
			},
		},
		{
			name:     "agent skill binding replacement",
			manifest: toolgovernance.Manifest{AssetType: "agent_skill"},
			arguments: map[string]interface{}{
				"agent_id":   "agent-1",
				"agent_name": "Support Agent",
				"skill_ids": []interface{}{
					"chart-generator",
					"toolkit",
				},
			},
			want: []toolgovernance.AssetRef{
				{ID: "agent-1", Type: "agent", Name: "Support Agent", Source: "tool_arguments", Metadata: map[string]interface{}{"binding_owner": true}},
				{ID: "chart-generator", Type: "agent_skill", Source: "tool_arguments", Metadata: map[string]interface{}{"agent_id": "agent-1", "agent_name": "Support Agent"}},
				{ID: "toolkit", Type: "agent_skill", Source: "tool_arguments", Metadata: map[string]interface{}{"agent_id": "agent-1", "agent_name": "Support Agent"}},
			},
		},
		{
			name:     "agent database binding replacement",
			manifest: toolgovernance.Manifest{AssetType: "database_table"},
			arguments: map[string]interface{}{
				"agent_id":   "agent-1",
				"agent_name": "Support Agent",
				"bindings": []interface{}{map[string]interface{}{
					"data_source_id":   "db-1",
					"data_source_name": "CRM",
					"table_ids":        []interface{}{"table-1", "table-2"},
					"writable_table_ids": []interface{}{
						"table-2",
					},
					"tables": []interface{}{
						map[string]interface{}{"table_id": "table-1", "table_name": "customers"},
						map[string]interface{}{"table_id": "table-2", "table_name": "orders"},
					},
				}},
			},
			want: []toolgovernance.AssetRef{
				{ID: "agent-1", Type: "agent", Name: "Support Agent", Source: "tool_arguments", Metadata: map[string]interface{}{"binding_owner": true}},
				{ID: "table-1", Type: "database_table", Name: "customers", Source: "tool_arguments", Metadata: map[string]interface{}{"agent_id": "agent-1", "data_source_id": "db-1", "database_name": "CRM"}},
				{ID: "table-2", Type: "database_table", Name: "orders", Source: "tool_arguments", Metadata: map[string]interface{}{"agent_id": "agent-1", "data_source_id": "db-1", "database_name": "CRM", "writable": true}},
			},
		},
		{
			name:     "agent workflow binding replacement",
			manifest: toolgovernance.Manifest{AssetType: "workflow"},
			arguments: map[string]interface{}{
				"agent_id":   "agent-1",
				"agent_name": "Support Agent",
				"bindings": []interface{}{map[string]interface{}{
					"binding_id":       "binding-1",
					"label":            "Approval Flow",
					"workflow_id":      "workflow-1",
					"version_strategy": "latest",
				}},
			},
			want: []toolgovernance.AssetRef{
				{ID: "agent-1", Type: "agent", Name: "Support Agent", Source: "tool_arguments", Metadata: map[string]interface{}{"binding_owner": true}},
				{ID: "binding-1", Type: "workflow", Name: "Approval Flow", Source: "tool_arguments", Metadata: map[string]interface{}{"agent_id": "agent-1", "workflow_id": "workflow-1", "version_strategy": "latest"}},
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
		{
			name:     "generated file name uses format extension",
			manifest: toolgovernance.Manifest{AssetType: "file"},
			arguments: map[string]interface{}{
				"filename": "monthly-summary",
				"format":   "svg",
			},
			want: []toolgovernance.AssetRef{
				{Name: "monthly-summary.svg", Type: "file", Source: "tool_arguments", Metadata: map[string]interface{}{"format": "svg"}},
			},
		},
		{
			name:     "generated file name corrects mismatched extension",
			manifest: toolgovernance.Manifest{AssetType: "file"},
			arguments: map[string]interface{}{
				"filename": "monthly-summary.txt",
				"format":   "svg",
			},
			want: []toolgovernance.AssetRef{
				{Name: "monthly-summary.svg", Type: "file", Source: "tool_arguments", Metadata: map[string]interface{}{"format": "svg"}},
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

func TestPolicyToolGovernanceEnrichesAgentBindingResourceNamesFromCandidates(t *testing.T) {
	gateway := NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy())
	decision, err := gateway.DecideSkillTool(context.Background(), ToolGovernanceRequest{
		Manifest: toolgovernance.Manifest{
			ToolID:                  "agent.replace_knowledge_bindings",
			SkillID:                 SkillAgentManagement,
			Domain:                  "agents",
			Effect:                  toolgovernance.EffectUpdate,
			AssetType:               "knowledge_base",
			RiskLevel:               toolgovernance.RiskLevelMedium,
			RequiresAssetResolution: true,
			PermissionScopes:        []string{"agent:manage", "knowledge:read"},
			DefaultApprovalPolicy:   toolgovernance.ApprovalPolicyAutoByPermissionTier,
			AllowedPermissionTiers:  []toolgovernance.PermissionTier{toolgovernance.PermissionTierAdvanced},
			AuditRequired:           true,
		},
		SkillID:  SkillAgentManagement,
		ToolName: "replace_agent_knowledge_bindings",
		Arguments: map[string]interface{}{
			"agent_id":   "agent-1",
			"agent_name": "Support Agent",
			"dataset_ids": []interface{}{
				"kb-1",
			},
		},
		ExecutionContext: ExecutionContext{
			ConversationID: "conversation-1",
			RuntimeParameters: map[string]interface{}{
				"tool_governance_permission_tier": "advanced",
				"knowledge_binding_candidates": []map[string]interface{}{
					{"dataset_id": "kb-1", "name": "Policies"},
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
	if len(decision.Assets) != 2 {
		t.Fatalf("assets = %#v, want owner Agent and knowledge base", decision.Assets)
	}
	if decision.Assets[1].ID != "kb-1" || decision.Assets[1].Name != "Policies" || decision.Assets[1].Type != "knowledge_base" {
		t.Fatalf("knowledge asset = %#v, want named kb-1 Policies", decision.Assets[1])
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
