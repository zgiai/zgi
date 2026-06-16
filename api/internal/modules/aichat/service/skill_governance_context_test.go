package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	aichatmodel "github.com/zgiai/zgi/api/internal/modules/aichat/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestSkillExecutionContextAddsSelectedFileGovernanceAsset(t *testing.T) {
	prepared := preparedGovernanceTestChat("read the selected file", []governanceTestFile{
		{ID: "file-1", Name: "one.pdf", Extension: "pdf"},
		{ID: "file-2", Name: "selected.xlsx", Extension: "xlsx", Selected: true},
	})

	execCtx := (&service{}).skillExecutionContext(prepared)
	governance := governanceRuntimeParamsFromTest(t, execCtx.RuntimeParameters)
	if governance["permission_tier"] != "basic" {
		t.Fatalf("permission_tier = %#v, want basic", governance["permission_tier"])
	}
	assets := governanceAssetsFromTest(t, governance)
	if len(assets) != 1 || assets[0]["id"] != "file-2" || assets[0]["name"] != "selected.xlsx" {
		t.Fatalf("governance assets = %#v, want selected file-2", assets)
	}
}

func TestSkillExecutionContextAddsOrdinalFileGovernanceAsset(t *testing.T) {
	prepared := preparedGovernanceTestChat("translate the fourth file", []governanceTestFile{
		{ID: "file-1", Name: "one.xlsx", Extension: "xlsx"},
		{ID: "file-2", Name: "two.xlsx", Extension: "xlsx"},
		{ID: "file-3", Name: "three.pdf", Extension: "pdf"},
		{ID: "file-4", Name: "four.pdf", Extension: "pdf"},
	})

	governance := governanceRuntimeParamsFromTest(t, (&service{}).skillExecutionContext(prepared).RuntimeParameters)
	assets := governanceAssetsFromTest(t, governance)
	if len(assets) != 1 || assets[0]["id"] != "file-4" || assets[0]["name"] != "four.pdf" {
		t.Fatalf("governance assets = %#v, want fourth file", assets)
	}
}

func TestSkillExecutionContextDoesNotAddAmbiguousFileGovernanceAssets(t *testing.T) {
	prepared := preparedGovernanceTestChat("review these files", []governanceTestFile{
		{ID: "file-1", Name: "one.pdf", Extension: "pdf"},
		{ID: "file-2", Name: "two.xlsx", Extension: "xlsx"},
	})

	governance := governanceRuntimeParamsFromTest(t, (&service{}).skillExecutionContext(prepared).RuntimeParameters)
	if _, exists := governance["assets"]; exists {
		t.Fatalf("governance assets = %#v, want omitted for ambiguous target", governance["assets"])
	}
	if governance["permission_tier"] != "basic" {
		t.Fatalf("permission_tier = %#v, want basic", governance["permission_tier"])
	}
}

func TestSkillExecutionContextUsesConversationWorkspaceWhenScopeMissing(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	workspaceID := uuid.New()
	prepared := &PreparedChat{
		Conversation: &aichatmodel.Conversation{ID: uuid.New(), WorkspaceID: &workspaceID},
		Message:      &aichatmodel.Message{ID: uuid.New()},
		Scope: Scope{
			OrganizationID: organizationID,
			AccountID:      accountID,
		},
	}

	params := (&service{}).skillExecutionContext(prepared).RuntimeParameters
	if params["workspace_id"] != workspaceID.String() {
		t.Fatalf("workspace_id = %#v, want conversation workspace %s", params["workspace_id"], workspaceID)
	}
}

func TestToolGovernanceDecisionPayloadIncludesDecision(t *testing.T) {
	prepared := preparedGovernanceTestChat("delete selected file", []governanceTestFile{
		{ID: "file-1", Name: "one.pdf", Extension: "pdf", Selected: true},
	})
	trace := governanceDecisionTraceForTest()

	payload := toolGovernanceDecisionPayload(prepared, trace)
	if payload["decision"] != trace.Governance.Status || payload["correlation_id"] != trace.Governance.CorrelationID {
		t.Fatalf("payload = %#v, want governance decision and correlation id", payload)
	}
	if payload["approval_event"] == nil {
		t.Fatalf("payload = %#v, want approval_event", payload)
	}
}

type governanceTestFile struct {
	ID        string
	Name      string
	Extension string
	Selected  bool
}

func preparedGovernanceTestChat(query string, files []governanceTestFile) *PreparedChat {
	organizationID := uuid.New()
	accountID := uuid.New()
	workspaceID := uuid.New()
	operationContext := governanceTestOperationContext(files, workspaceID.String())
	return &PreparedChat{
		Conversation: &aichatmodel.Conversation{ID: uuid.New()},
		Message:      &aichatmodel.Message{ID: uuid.New()},
		Scope: Scope{
			OrganizationID: organizationID,
			AccountID:      accountID,
			WorkspaceID:    &workspaceID,
		},
		parts: &chatRequestParts{
			Query:               query,
			RawOperationContext: operationContext,
			OperationContext:    operationContext,
		},
	}
}

func governanceTestOperationContext(files []governanceTestFile, workspaceID string) map[string]interface{} {
	resources := make([]interface{}, 0, len(files))
	for _, file := range files {
		resources = append(resources, map[string]interface{}{
			"resource_type": "file",
			"resource_id":   file.ID,
			"title":         file.Name,
			"source":        "Files page",
			"metadata": map[string]interface{}{
				"file_id":      file.ID,
				"name":         file.Name,
				"extension":    file.Extension,
				"selected":     file.Selected,
				"workspace_id": workspaceID,
			},
		})
	}
	return map[string]interface{}{
		"schema":    "zgi.aichat.operation_context.v1",
		"version":   1,
		"resources": resources,
	}
}

func governanceDecisionTraceForTest() skills.SkillTrace {
	decision := toolgovernance.Decision{
		Status:           toolgovernance.DecisionStatusNeedsApproval,
		RequiresApproval: true,
		Reason:           "delete effect requires user approval",
		CorrelationID:    "corr-1",
		Manifest: toolgovernance.Manifest{
			ToolID:    "file.delete",
			SkillID:   "console-files",
			Domain:    "files",
			Effect:    toolgovernance.EffectDelete,
			AssetType: "file",
			RiskLevel: toolgovernance.RiskLevelHigh,
		},
		ApprovalEvent: &toolgovernance.ApprovalEvent{
			Type:          toolgovernance.ApprovalEventTypeAssetToolApproval,
			CorrelationID: "corr-1",
			ToolID:        "file.delete",
			SkillID:       "console-files",
			Effect:        toolgovernance.EffectDelete,
			AssetType:     "file",
			RiskLevel:     toolgovernance.RiskLevelHigh,
		},
	}
	return skills.SkillTrace{
		Kind:       "tool_governance",
		SkillID:    "console-files",
		ToolName:   "delete_file",
		Status:     string(toolgovernance.DecisionStatusNeedsApproval),
		Governance: &decision,
	}
}

func governanceRuntimeParamsFromTest(t *testing.T, params map[string]interface{}) map[string]interface{} {
	t.Helper()
	governance, ok := params["tool_governance"].(map[string]interface{})
	if !ok {
		t.Fatalf("tool_governance = %#v, want map", params["tool_governance"])
	}
	return governance
}

func governanceAssetsFromTest(t *testing.T, governance map[string]interface{}) []map[string]interface{} {
	t.Helper()
	assets, ok := governance["assets"].([]map[string]interface{})
	if !ok {
		t.Fatalf("assets = %#v, want []map[string]interface{}", governance["assets"])
	}
	return assets
}
