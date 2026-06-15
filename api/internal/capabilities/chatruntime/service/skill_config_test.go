package service

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestEffectiveAgentSkillIDsAutoAddsHiddenKnowledge(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillCalculator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}},
		{ID: skills.SkillInternalKnowledge, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillAgentKnowledge, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}, RequiredConfig: []string{skills.SkillRequiredConfigAgentKnowledge}},
		{ID: skills.SkillUserMemory, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}},
	}

	got := effectiveAgentSkillIDs(
		[]string{skills.SkillCalculator, skills.SkillAgentKnowledge, skills.SkillUserMemory, skills.SkillInternalKnowledge},
		catalog,
		&RunConfig{KnowledgeDatasetIDs: []string{"dataset-1"}},
	)
	want := []string{skills.SkillAgentKnowledge, skills.SkillCalculator}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("effectiveAgentSkillIDs() = %#v, want %#v", got, want)
	}
}

func TestEffectiveAgentSkillIDsSkipsKnowledgeWithoutDatasets(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillCalculator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}},
		{ID: skills.SkillAgentKnowledge, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}, RequiredConfig: []string{skills.SkillRequiredConfigAgentKnowledge}},
	}

	got := effectiveAgentSkillIDs(
		[]string{skills.SkillCalculator, skills.SkillAgentKnowledge},
		catalog,
		&RunConfig{},
	)
	want := []string{skills.SkillCalculator}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("effectiveAgentSkillIDs() = %#v, want %#v", got, want)
	}
}

func TestEffectiveAgentSkillIDsAutoAddsHiddenDatabase(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillCalculator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}},
		{ID: skills.SkillAgentDatabase, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}, RequiredConfig: []string{skills.SkillRequiredConfigAgentDatabase}},
	}

	got := effectiveAgentSkillIDs(
		[]string{skills.SkillCalculator},
		catalog,
		&RunConfig{DatabaseBindings: []AgentDatabaseBinding{{DataSourceID: "db-1", TableIDs: []string{"table-1"}}}},
	)
	want := []string{skills.SkillAgentDatabase, skills.SkillCalculator}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("effectiveAgentSkillIDs() = %#v, want %#v", got, want)
	}
}

func TestEffectiveAgentSkillIDsSkipsDatabaseWithoutBindings(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillCalculator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}},
		{ID: skills.SkillAgentDatabase, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}, RequiredConfig: []string{skills.SkillRequiredConfigAgentDatabase}},
	}

	got := effectiveAgentSkillIDs(
		[]string{skills.SkillCalculator, skills.SkillAgentDatabase},
		catalog,
		&RunConfig{},
	)
	want := []string{skills.SkillCalculator}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("effectiveAgentSkillIDs() = %#v, want %#v", got, want)
	}
}

func TestEffectiveAgentSkillIDsAutoAddsHiddenWorkflow(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillCalculator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}},
		{ID: skills.SkillAgentWorkflow, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}, RequiredConfig: []string{skills.SkillRequiredConfigAgentWorkflow}},
	}

	got := effectiveAgentSkillIDs(
		[]string{skills.SkillCalculator},
		catalog,
		&RunConfig{WorkflowBindings: []AgentWorkflowBinding{{BindingID: "approval-flow", AgentID: "agent-1", WorkflowID: "workflow-1"}}},
	)
	want := []string{skills.SkillAgentWorkflow, skills.SkillCalculator}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("effectiveAgentSkillIDs() = %#v, want %#v", got, want)
	}
}

func TestEffectiveAgentSkillIDsSkipsWorkflowWithoutBindings(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillCalculator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}},
		{ID: skills.SkillAgentWorkflow, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}, RequiredConfig: []string{skills.SkillRequiredConfigAgentWorkflow}},
	}

	got := effectiveAgentSkillIDs(
		[]string{skills.SkillCalculator, skills.SkillAgentWorkflow},
		catalog,
		&RunConfig{},
	)
	want := []string{skills.SkillCalculator}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("effectiveAgentSkillIDs() = %#v, want %#v", got, want)
	}
}

func TestEffectiveAgentSkillIDsDoesNotAutoAddHiddenAgentMemory(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillAgentMemory, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}},
		{ID: skills.SkillUserMemory, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
	}

	got := effectiveAgentSkillIDs(
		[]string{skills.SkillUserMemory},
		catalog,
		&RunConfig{
			AgentMemoryEnabled: true,
			AgentMemorySlots: []AgentMemorySlotConfig{{
				Key:      "profile",
				MaxChars: 1000,
				Enabled:  true,
			}},
		},
	)
	want := []string{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("effectiveAgentSkillIDs() = %#v, want %#v", got, want)
	}
}

func TestRunConfigAllowsUserMemoryRejectsAgent(t *testing.T) {
	if runConfigAllowsUserMemory(RunConfig{UseMemory: true, BillingAppType: runtimemodel.ConversationCallerAgent}) {
		t.Fatal("runConfigAllowsUserMemory() = true for agent, want false")
	}
	if !runConfigAllowsUserMemory(RunConfig{UseMemory: true, BillingAppType: runtimemodel.ConversationCallerAIChat}) {
		t.Fatal("runConfigAllowsUserMemory() = false for aichat, want true")
	}
}

func TestAddContextualAIChatSkillIDsAddsFileReaderForConsoleFileCapability(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillCalculator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillFileReader, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
	}
	parts := &chatRequestParts{
		RuntimeContext: "route=/console/files capabilities=file.delete",
		RawOperationContext: map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{
					"resource_type": "file",
					"resource_id":   "file-1",
					"title":         "old.pdf",
					"capabilities": []interface{}{
						map[string]interface{}{"id": "file.delete"},
					},
				},
			},
		},
	}

	got := addContextualAIChatSkillIDs(
		[]string{skills.SkillCalculator},
		[]string{skills.SkillCalculator, skills.SkillFileReader},
		catalog,
		parts,
	)
	want := []string{skills.SkillCalculator, skills.SkillFileReader}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("contextual skills = %#v, want %#v", got, want)
	}
}

func TestAddContextualAIChatSkillIDsRespectsOrganizationDisabledSkill(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillCalculator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillFileReader, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
	}
	parts := &chatRequestParts{
		RuntimeContext: "route=/console/files capabilities=file.delete",
		RawOperationContext: map[string]interface{}{
			"capabilities": []interface{}{
				map[string]interface{}{"id": "file.delete"},
			},
		},
	}

	got := addContextualAIChatSkillIDs(
		[]string{skills.SkillCalculator},
		[]string{skills.SkillCalculator},
		catalog,
		parts,
	)
	want := []string{skills.SkillCalculator}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("contextual skills = %#v, want %#v", got, want)
	}
}

func TestAddUnconfiguredDefaultSkillIDsAddsMissingDefaultSystemSkill(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillCalculator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillFileReader, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
	}

	got := addUnconfiguredDefaultSkillIDs(
		[]string{skills.SkillCalculator},
		map[string]struct{}{skills.SkillCalculator: {}},
		catalog,
	)
	want := []string{skills.SkillCalculator, skills.SkillFileReader}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("default skills = %#v, want %#v", got, want)
	}
}

func TestAddUnconfiguredDefaultSkillIDsPreservesExplicitDisable(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillCalculator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillFileReader, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
	}

	got := addUnconfiguredDefaultSkillIDs(
		[]string{skills.SkillCalculator},
		map[string]struct{}{
			skills.SkillCalculator: {},
			skills.SkillFileReader: {},
		},
		catalog,
	)
	want := []string{skills.SkillCalculator}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("default skills = %#v, want %#v", got, want)
	}
}

func TestApplyRunConfigToPartsPreservesAIChatUseMemoryRequest(t *testing.T) {
	parts := &chatRequestParts{UseMemory: true}

	applyRunConfigToParts(RunConfig{BillingAppType: runtimemodel.MessageBillingReasonSourceAIChat}, parts)

	if !parts.UseMemory {
		t.Fatal("applyRunConfigToParts() cleared AIChat request use memory")
	}
}

func TestApplyRunConfigToPartsDisablesAccountMemoryForAgentCaller(t *testing.T) {
	parts := &chatRequestParts{UseMemory: true}

	applyRunConfigToParts(RunConfig{BillingAppType: runtimemodel.ConversationCallerAgent}, parts)

	if parts.UseMemory {
		t.Fatal("applyRunConfigToParts() left account memory enabled for agent caller")
	}
}

func TestSkillRuntimeParametersUseCapabilityConfig(t *testing.T) {
	organizationID := uuid.New()
	workspaceID := uuid.New()
	params := skillRuntimeParameters(Scope{OrganizationID: organizationID, WorkspaceID: &workspaceID}, RunConfig{
		BillingAppType:            runtimemodel.ConversationCallerAgent,
		BillingAppID:              "agent-1",
		KnowledgeDatasetIDs:       []string{"dataset-1"},
		KnowledgeBoundByAccountID: "knowledge-binder",
		KnowledgeBoundAtUnix:      123,
		DatabaseBindings:          []AgentDatabaseBinding{{DataSourceID: "db-1", TableIDs: []string{"table-1"}}},
		DatabaseBoundByAccountID:  "database-binder",
		DatabaseBoundAtUnix:       456,
		WorkflowBindings:          []AgentWorkflowBinding{{BindingID: "approval-flow", AgentID: "agent-1", WorkflowID: "workflow-1"}},
		WorkflowBoundByAccountID:  "workflow-binder",
		WorkflowBoundAtUnix:       789,
	})

	if params["organization_id"] != organizationID.String() || params["workspace_id"] != workspaceID.String() {
		t.Fatalf("scope params = %#v, want organization and workspace ids", params)
	}
	if params["agent_id"] != "agent-1" {
		t.Fatalf("agent_id = %#v, want agent-1", params["agent_id"])
	}
	if params["knowledge_binding_grant"] != true || params["knowledge_bound_by_account_id"] != "knowledge-binder" || params["knowledge_bound_at_unix"] != int64(123) {
		t.Fatalf("knowledge grant params = %#v", params)
	}
	if params["database_binding_grant"] != true || params["database_bound_by_account_id"] != "database-binder" || params["database_bound_at_unix"] != int64(456) {
		t.Fatalf("database grant params = %#v", params)
	}
	if params["workflow_binding_grant"] != true || params["workflow_bound_by_account_id"] != "workflow-binder" || params["workflow_bound_at_unix"] != int64(789) {
		t.Fatalf("workflow grant params = %#v", params)
	}
	if bindings, ok := params["workflow_bindings"].([]AgentWorkflowBinding); !ok || len(bindings) != 1 || bindings[0].BindingID != "approval-flow" {
		t.Fatalf("workflow bindings param = %#v", params["workflow_bindings"])
	}
}

func TestSkillRuntimeParametersForPreparedAddsSelectedFileGovernanceAsset(t *testing.T) {
	prepared := &PreparedChat{
		Scope:     Scope{OrganizationID: uuid.New()},
		RunConfig: RunConfig{},
		parts: consoleFilesSemanticTestParts("read the selected file", []consoleFilesTestFile{
			{ID: "file-1", Name: "one.pdf", Extension: "pdf"},
			{ID: "file-2", Name: "selected.xlsx", Extension: "xlsx", Selected: true},
		}),
	}

	params := skillRuntimeParametersForPrepared(prepared)
	governance := governanceRuntimeParamsFromTest(t, params)
	if governance["permission_tier"] != "basic" {
		t.Fatalf("permission_tier = %#v, want basic", governance["permission_tier"])
	}
	assets := governanceAssetsFromTest(t, governance)
	if len(assets) != 1 || assets[0]["id"] != "file-2" || assets[0]["name"] != "selected.xlsx" {
		t.Fatalf("governance assets = %#v, want selected file-2", assets)
	}
}

func TestSkillRuntimeParametersForPreparedAddsConsoleFilesVisibleFiles(t *testing.T) {
	prepared := &PreparedChat{
		Scope:     Scope{OrganizationID: uuid.New()},
		RunConfig: RunConfig{},
		parts: consoleFilesSemanticTestParts("what files are visible", []consoleFilesTestFile{
			{ID: "file-1", Name: "one.pdf", Extension: "pdf"},
			{ID: "file-2", Name: "selected.xlsx", Extension: "xlsx", Selected: true},
		}),
	}

	params := skillRuntimeParametersForPrepared(prepared)
	files, ok := params["console_files_visible_files"].([]map[string]interface{})
	if !ok || len(files) != 2 {
		t.Fatalf("console_files_visible_files = %#v, want 2 visible files", params["console_files_visible_files"])
	}
	if files[0]["file_id"] != "file-1" || files[0]["visible_index"] != 1 {
		t.Fatalf("first visible file = %#v, want file-1 at index 1", files[0])
	}
	if files[1]["file_id"] != "file-2" || files[1]["selected"] != true {
		t.Fatalf("second visible file = %#v, want selected file-2", files[1])
	}
}

func TestSkillRuntimeParametersForPreparedAddsOrdinalFileGovernanceAsset(t *testing.T) {
	prepared := &PreparedChat{
		Scope:     Scope{OrganizationID: uuid.New()},
		RunConfig: RunConfig{},
		parts: consoleFilesSemanticTestParts("translate the fourth file", []consoleFilesTestFile{
			{ID: "file-1", Name: "one.xlsx", Extension: "xlsx"},
			{ID: "file-2", Name: "two.xlsx", Extension: "xlsx"},
			{ID: "file-3", Name: "three.pdf", Extension: "pdf"},
			{ID: "file-4", Name: "four.pdf", Extension: "pdf"},
		}),
	}

	governance := governanceRuntimeParamsFromTest(t, skillRuntimeParametersForPrepared(prepared))
	assets := governanceAssetsFromTest(t, governance)
	if len(assets) != 1 || assets[0]["id"] != "file-4" || assets[0]["name"] != "four.pdf" {
		t.Fatalf("governance assets = %#v, want fourth file", assets)
	}
}

func TestSkillRuntimeParametersForPreparedAddsChineseSpreadsheetGovernanceAsset(t *testing.T) {
	prepared := &PreparedChat{
		Scope:     Scope{OrganizationID: uuid.New()},
		RunConfig: RunConfig{},
		parts: consoleFilesSemanticTestParts("\u6458\u8981\u7b2c\u4e8c\u4e2a\u8868\u683c", []consoleFilesTestFile{
			{ID: "file-1", Name: "notes.txt", Extension: "txt"},
			{ID: "file-2", Name: "budget-q1.xlsx", Extension: "xlsx"},
			{ID: "file-3", Name: "proposal.pdf", Extension: "pdf"},
			{ID: "file-4", Name: "budget-q2.xlsx", Extension: "xlsx"},
		}),
	}

	governance := governanceRuntimeParamsFromTest(t, skillRuntimeParametersForPrepared(prepared))
	assets := governanceAssetsFromTest(t, governance)
	if len(assets) != 1 || assets[0]["id"] != "file-4" || assets[0]["name"] != "budget-q2.xlsx" {
		t.Fatalf("governance assets = %#v, want second spreadsheet file-4", assets)
	}
}

func TestSkillRuntimeParametersForPreparedAddsNamedFileGovernanceAsset(t *testing.T) {
	prepared := &PreparedChat{
		Scope:     Scope{OrganizationID: uuid.New()},
		RunConfig: RunConfig{},
		parts: consoleFilesSemanticTestParts("delete file codex-smoke-delete-visible-20260615-1538.txt", []consoleFilesTestFile{
			{ID: "file-1", Name: "codex-smoke-delete-visible-20260615-1538.txt", Extension: "txt"},
			{ID: "file-2", Name: "other.txt", Extension: "txt"},
		}),
	}

	governance := governanceRuntimeParamsFromTest(t, skillRuntimeParametersForPrepared(prepared))
	assets := governanceAssetsFromTest(t, governance)
	if len(assets) != 1 || assets[0]["id"] != "file-1" || assets[0]["name"] != "codex-smoke-delete-visible-20260615-1538.txt" {
		t.Fatalf("governance assets = %#v, want named smoke file", assets)
	}
}

func TestSkillRuntimeParametersForPreparedDoesNotAddAmbiguousFileGovernanceAssets(t *testing.T) {
	prepared := &PreparedChat{
		Scope:     Scope{OrganizationID: uuid.New()},
		RunConfig: RunConfig{},
		parts: consoleFilesSemanticTestParts("review these files", []consoleFilesTestFile{
			{ID: "file-1", Name: "one.pdf", Extension: "pdf"},
			{ID: "file-2", Name: "two.xlsx", Extension: "xlsx"},
		}),
	}

	governance := governanceRuntimeParamsFromTest(t, skillRuntimeParametersForPrepared(prepared))
	if _, exists := governance["assets"]; exists {
		t.Fatalf("governance assets = %#v, want omitted for ambiguous target", governance["assets"])
	}
	if governance["permission_tier"] != "basic" {
		t.Fatalf("permission_tier = %#v, want basic", governance["permission_tier"])
	}
}

func TestApplySkillToolGovernanceRuntimeParametersPreservesFlatPermissionTier(t *testing.T) {
	params := applySkillToolGovernanceRuntimeParameters(map[string]interface{}{
		"tool_governance_permission_tier": "advanced",
	}, nil)

	governance := governanceRuntimeParamsFromTest(t, params)
	if governance["permission_tier"] != "advanced" {
		t.Fatalf("permission_tier = %#v, want advanced", governance["permission_tier"])
	}
}

func TestSkillRuntimeParametersForPreparedUsesOperationContextPermissionTier(t *testing.T) {
	prepared := &PreparedChat{
		Scope:     Scope{OrganizationID: uuid.New()},
		RunConfig: RunConfig{},
		parts: &chatRequestParts{
			Query: "summarize visible files",
			RawOperationContext: map[string]interface{}{
				"tool_governance": map[string]interface{}{
					"permission_tier": "advanced",
				},
			},
		},
	}

	governance := governanceRuntimeParamsFromTest(t, skillRuntimeParametersForPrepared(prepared))
	if governance["permission_tier"] != "advanced" {
		t.Fatalf("permission_tier = %#v, want advanced", governance["permission_tier"])
	}
}

func TestSkillRuntimeParametersForPreparedUsesNormalizedOperationContextPermissionTier(t *testing.T) {
	prepared := &PreparedChat{
		Scope:     Scope{OrganizationID: uuid.New()},
		RunConfig: RunConfig{},
		parts: &chatRequestParts{
			Query: "summarize visible files",
			OperationContext: map[string]interface{}{
				"toolGovernance": map[string]interface{}{
					"permissionTier": "full",
				},
			},
		},
	}

	governance := governanceRuntimeParamsFromTest(t, skillRuntimeParametersForPrepared(prepared))
	if governance["permission_tier"] != "full" {
		t.Fatalf("permission_tier = %#v, want full", governance["permission_tier"])
	}
}

func TestSkillRuntimeParametersForPreparedIgnoresInvalidOperationContextPermissionTier(t *testing.T) {
	prepared := &PreparedChat{
		Scope:     Scope{OrganizationID: uuid.New()},
		RunConfig: RunConfig{},
		parts: &chatRequestParts{
			Query: "summarize visible files",
			RawOperationContext: map[string]interface{}{
				"tool_governance": map[string]interface{}{
					"permission_tier": "owner",
				},
			},
		},
	}

	governance := governanceRuntimeParamsFromTest(t, skillRuntimeParametersForPrepared(prepared))
	if governance["permission_tier"] != "basic" {
		t.Fatalf("permission_tier = %#v, want basic", governance["permission_tier"])
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

func TestAgentWorkflowAvailableBindingsMessageInjectsSafeContext(t *testing.T) {
	message, ok := agentWorkflowAvailableBindingsMessage([]AgentWorkflowBinding{{
		BindingID:       "task-flow",
		Label:           "Task flow",
		Description:     "Runs a task workflow",
		AgentID:         "agent-1",
		WorkflowID:      "workflow-1",
		AgentType:       "WORKFLOW",
		VersionStrategy: "latest_published",
		TimeoutSeconds:  10,
		StartInputs: []AgentWorkflowStartInput{{
			Variable: "task",
			Label:    "Task",
			Type:     "string",
			Required: true,
		}},
	}})

	if !ok {
		t.Fatal("agentWorkflowAvailableBindingsMessage() returned no message")
	}
	content, ok := message.Content.(string)
	if !ok {
		t.Fatalf("message content type = %T, want string", message.Content)
	}
	for _, want := range []string{
		"available_workflows",
		`"binding_id":"task-flow"`,
		`"label":"Task flow"`,
		`"agent_type":"WORKFLOW"`,
		`"default_input_key":"task"`,
		`"required_inputs":["task"]`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("message content missing %q: %s", want, content)
		}
	}
	for _, forbidden := range []string{"workflow-1", "agent-1"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("message content exposed %q: %s", forbidden, content)
		}
	}
}

func TestVisibleSkillMetadataHidesRuntimeManagedSkills(t *testing.T) {
	metadata := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillInternalKnowledge},
		{ID: skills.SkillAgentKnowledge},
		{ID: skills.SkillAgentDatabase},
		{ID: skills.SkillAgentWorkflow},
		{ID: skills.SkillAgentMemory},
		{ID: skills.SkillUserMemory},
		{ID: skills.SkillCalculator},
	}

	got := visibleSkillMetadata(metadata)
	gotIDs := make([]string, 0, len(got))
	for _, item := range got {
		gotIDs = append(gotIDs, item.ID)
	}
	want := []string{skills.SkillInternalKnowledge, skills.SkillCalculator}
	if !reflect.DeepEqual(gotIDs, want) {
		t.Fatalf("visibleSkillMetadata ids = %#v, want %#v", gotIDs, want)
	}
}

func TestMergeSkillTraceMetadataAppendsExistingInvocations(t *testing.T) {
	source := map[string]interface{}{
		"skill_invocations": []interface{}{
			map[string]interface{}{
				"kind":       "memory_planner",
				"skill_id":   skills.SkillAgentMemory,
				"tool_name":  "plan_agent_memory",
				"status":     "success_update",
				"runtime_id": "memory-planner-1",
			},
		},
	}

	metadata := mergeSkillTraceMetadata(source, []skills.SkillTrace{{
		Kind:     "tool_call",
		SkillID:  skills.SkillCalculator,
		ToolName: "calculate",
		Status:   "success",
	}})

	invocations, ok := metadata["skill_invocations"].([]interface{})
	if !ok || len(invocations) != 2 {
		t.Fatalf("skill_invocations = %#v, want two invocations", metadata["skill_invocations"])
	}
	first, _ := invocations[0].(map[string]interface{})
	second, _ := invocations[1].(map[string]interface{})
	if first["kind"] != "memory_planner" || second["tool_name"] != "calculate" {
		t.Fatalf("skill_invocations = %#v, want preserved planner then calculator tool", invocations)
	}
	if metadata["skill_step_count"] != 1 || metadata["tool_call_count"] != 1 {
		t.Fatalf("summary = %#v, want only visible calculator tool counted", metadata)
	}
}

func TestMergeSkillInvocationMetadataMergesStartAndEndByRuntimeID(t *testing.T) {
	runtimeID := "agent-memory:update:profile"
	metadata := mergeSkillInvocationMetadata(nil, []map[string]interface{}{
		newSkillInvocation("tool_call", skills.SkillAgentMemory, "update_agent_memory", "running", map[string]interface{}{
			"runtime_id": runtimeID,
			"arguments":  map[string]interface{}{"key": "profile"},
		}),
	})
	metadata = mergeSkillInvocationMetadata(metadata, []map[string]interface{}{
		newSkillInvocation("tool_call", skills.SkillAgentMemory, "update_agent_memory", "success", map[string]interface{}{
			"runtime_id": runtimeID,
			"result":     map[string]interface{}{"key": "profile"},
		}),
	})

	invocations, ok := metadata["skill_invocations"].([]interface{})
	if !ok || len(invocations) != 1 {
		t.Fatalf("skill_invocations = %#v, want one merged invocation", metadata["skill_invocations"])
	}
	invocation, _ := invocations[0].(map[string]interface{})
	if invocation["status"] != "success" || invocation["runtime_id"] != runtimeID {
		t.Fatalf("invocation = %#v, want success with runtime id", invocation)
	}
}

func TestMessageEndPayloadPreservesTraceMetadata(t *testing.T) {
	conversationID := uuid.New()
	messageID := uuid.New()
	payload := messageEndPayload(&PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: conversationID},
		Message:      &runtimemodel.Message{ID: messageID},
	}, map[string]interface{}{
		"usage": map[string]interface{}{"total_tokens": 1},
		"context_control": map[string]interface{}{
			"agent_memory": map[string]interface{}{"planner_status": "success_update"},
		},
		"skill_invocations": []interface{}{
			map[string]interface{}{"kind": "memory_planner"},
		},
		"has_trace": true,
	})

	metadata, ok := payload["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("metadata = %#v, want map", payload["metadata"])
	}
	if metadata["has_trace"] != true || metadata["skill_invocations"] == nil || metadata["context_control"] == nil {
		t.Fatalf("metadata = %#v, want trace metadata preserved", metadata)
	}
}

func TestProcessTimelineRecorderMergesSkillCallStartAndEnd(t *testing.T) {
	prepared := preparedTimelineTestChat()
	recorder := newProcessTimelineRecorder(context.Background(), context.Background(), &service{}, prepared, nil)

	recorder.RecordEvent(streamEventSkillCallStart, map[string]interface{}{
		"conversation_id":   prepared.Conversation.ID.String(),
		"message_id":        prepared.Message.ID.String(),
		"skill_id":          skills.SkillCalculator,
		"tool_name":         "calculate",
		"arguments_summary": map[string]interface{}{"expression": "1+1"},
	})
	recorder.RecordEvent(streamEventSkillCallEnd, map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"skill_id":        skills.SkillCalculator,
		"tool_name":       "calculate",
		"status":          "success",
		"result":          map[string]interface{}{"value": 2},
	})

	invocation := onlyTimelineInvocation(t, prepared)
	if invocation["status"] != "success" || invocation["runtime_id"] == "" {
		t.Fatalf("invocation = %#v, want merged success with runtime_id", invocation)
	}
	if invocation["arguments"] == nil || invocation["result"] == nil {
		t.Fatalf("invocation = %#v, want arguments and result preserved", invocation)
	}
}

func TestProcessTimelineRecorderDoesNotDuplicateStreamBackedTrace(t *testing.T) {
	prepared := preparedTimelineTestChat()
	recorder := newProcessTimelineRecorder(context.Background(), context.Background(), &service{}, prepared, nil)

	recorder.RecordEvent(streamEventSkillCallStart, map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"skill_id":        skills.SkillCalculator,
		"tool_name":       "calculate",
	})
	trace := skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillCalculator,
		ToolName: "calculate",
		Status:   "success",
	}
	recorder.RecordEvent(streamEventSkillCallEnd, map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"skill_id":        skills.SkillCalculator,
		"tool_name":       "calculate",
		"status":          "success",
	})
	recorder.RecordTrace([]skills.SkillTrace{trace}, trace)

	invocations, ok := prepared.Message.Metadata["skill_invocations"].([]interface{})
	if !ok || len(invocations) != 1 {
		t.Fatalf("skill_invocations = %#v, want one invocation", prepared.Message.Metadata["skill_invocations"])
	}
}

func TestProcessTimelineRecorderSkipsInternalDiagnosticTrace(t *testing.T) {
	prepared := preparedTimelineTestChat()
	recorder := newProcessTimelineRecorder(context.Background(), context.Background(), &service{}, prepared, nil)
	trace := skills.SkillTrace{
		Kind:     "memory_planner",
		SkillID:  skills.SkillAgentMemory,
		ToolName: "plan_agent_memory",
		Status:   "success_update",
	}

	recorder.RecordTrace([]skills.SkillTrace{trace}, trace)

	if invocations := prepared.Message.Metadata["skill_invocations"]; invocations != nil {
		t.Fatalf("skill_invocations = %#v, want no persisted planner invocation", invocations)
	}
}

func TestEmitAgentMemoryMutationEventUsesIndependentMemoryEvent(t *testing.T) {
	prepared := preparedTimelineTestChat()
	var got *StreamEvent
	(&service{}).emitAgentMemoryMutationEvent(context.Background(), prepared, skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillAgentMemory,
		ToolName: agentMemoryToolUpdate,
		Status:   "success",
		Result:   map[string]interface{}{"key": "profile", "content": "Call the user Captain."},
	}, func(event StreamEvent) error {
		got = &event
		return nil
	})
	if got == nil {
		t.Fatal("memory mutation event was not emitted")
	}
	if got.EventType != streamEventMemoryUpdate {
		t.Fatalf("event type = %q, want %q", got.EventType, streamEventMemoryUpdate)
	}
	if got.Payload["memory_scope"] != "agent" || got.Payload["action"] != "update" || got.Payload["key"] != "profile" || got.Payload["content"] != "Call the user Captain." {
		t.Fatalf("payload = %#v, want agent memory update payload with full content", got.Payload)
	}
}

func TestEmitUserMemoryMutationEventUsesIndependentMemoryEvent(t *testing.T) {
	prepared := preparedTimelineTestChat()
	var got *StreamEvent
	(&service{}).emitUserMemoryMutationEvent(context.Background(), prepared, skills.SkillTrace{
		Kind:   "user_memory",
		Status: "success",
	}, map[string]interface{}{
		"action":   "update",
		"entry_id": "entry-1",
		"content":  "Call the user Captain.",
	}, func(event StreamEvent) error {
		got = &event
		return nil
	})
	if got == nil {
		t.Fatal("memory mutation event was not emitted")
	}
	if got.EventType != streamEventMemoryUpdate {
		t.Fatalf("event type = %q, want %q", got.EventType, streamEventMemoryUpdate)
	}
	if got.Payload["memory_scope"] != "account" || got.Payload["action"] != "update" || got.Payload["entry_id"] != "entry-1" {
		t.Fatalf("payload = %#v, want account memory update payload", got.Payload)
	}
}

func TestProcessTimelineRecorderAggregatesIntermediateAnswerChunks(t *testing.T) {
	prepared := preparedTimelineTestChat()
	recorder := newProcessTimelineRecorder(context.Background(), context.Background(), &service{}, prepared, nil)
	basePayload := map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"answer_id":       "answer-1",
		"title":           "Draft",
		"delta":           true,
	}

	first := copyStringAnyMap(basePayload)
	first["content"] = "hello "
	first["done"] = false
	recorder.RecordEvent(streamEventIntermediateAnswer, first)
	second := copyStringAnyMap(basePayload)
	second["content"] = "world"
	second["done"] = false
	recorder.RecordEvent(streamEventIntermediateAnswer, second)
	done := copyStringAnyMap(basePayload)
	done["content"] = ""
	done["done"] = true
	recorder.RecordEvent(streamEventIntermediateAnswer, done)

	invocation := onlyTimelineInvocation(t, prepared)
	if invocation["status"] != "success" || invocation["message"] != "hello world" {
		t.Fatalf("invocation = %#v, want aggregated successful intermediate answer", invocation)
	}
}

func TestProcessTimelineRecorderPersistsWorkflowRunEvents(t *testing.T) {
	prepared := preparedTimelineTestChat()
	recorder := newProcessTimelineRecorder(context.Background(), context.Background(), &service{}, prepared, nil)

	recorder.RecordEvent("workflow_started", map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"workflow_run_id": "run-1",
		"workflow_id":     "workflow-1",
		"status":          "running",
		"created_at":      10,
	})
	recorder.RecordEvent("node_started", map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"workflow_run_id": "run-1",
		"node_id":         "node-1",
		"node_type":       "answer",
		"node_title":      "Answer",
		"inputs":          map[string]interface{}{"query": "hello"},
		"status":          "running",
		"created_at":      11,
	})
	recorder.RecordEvent("node_finished", map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"workflow_run_id": "run-1",
		"node_id":         "node-1",
		"node_type":       "answer",
		"node_title":      "Answer",
		"outputs":         map[string]interface{}{"answer": "done"},
		"status":          "succeeded",
		"elapsed_time":    0.2,
		"created_at":      12,
	})
	recorder.RecordEvent("workflow_finished", map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"workflow_run_id": "run-1",
		"status":          "succeeded",
		"outputs":         map[string]interface{}{"answer": "done"},
		"elapsed_time":    0.3,
		"created_at":      13,
	})

	runs, ok := prepared.Message.Metadata["workflow_runs"].([]interface{})
	if !ok || len(runs) != 1 {
		t.Fatalf("workflow_runs = %#v, want one persisted run", prepared.Message.Metadata["workflow_runs"])
	}
	run, _ := runs[0].(map[string]interface{})
	if run["workflow_run_id"] != "run-1" || run["status"] != "succeeded" || run["outputs"] == nil {
		t.Fatalf("run = %#v, want succeeded run with outputs", run)
	}
	nodes, ok := run["nodes"].([]interface{})
	if !ok || len(nodes) != 1 {
		t.Fatalf("nodes = %#v, want one merged node", run["nodes"])
	}
	node, _ := nodes[0].(map[string]interface{})
	if node["status"] != "succeeded" || node["inputs"] == nil || node["outputs"] == nil {
		t.Fatalf("node = %#v, want inputs and outputs preserved", node)
	}
}

func preparedTimelineTestChat() *PreparedChat {
	return &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message: &runtimemodel.Message{
			ID:       uuid.New(),
			Metadata: map[string]interface{}{},
		},
	}
}

func onlyTimelineInvocation(t *testing.T, prepared *PreparedChat) map[string]interface{} {
	t.Helper()
	invocations, ok := prepared.Message.Metadata["skill_invocations"].([]interface{})
	if !ok || len(invocations) != 1 {
		t.Fatalf("skill_invocations = %#v, want one invocation", prepared.Message.Metadata["skill_invocations"])
	}
	invocation, ok := invocations[0].(map[string]interface{})
	if !ok {
		t.Fatalf("invocation type = %T, want map", invocations[0])
	}
	return invocation
}
