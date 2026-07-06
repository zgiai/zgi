package service

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

type skillConfigWorkspacePermissionService struct {
	allowed map[workspacemodel.WorkspacePermissionCode]bool
	codes   []workspacemodel.WorkspacePermissionCode
}

func (s *skillConfigWorkspacePermissionService) CheckWorkspacePermission(_ context.Context, _, _, _ string, code workspacemodel.WorkspacePermissionCode) (bool, error) {
	s.codes = append(s.codes, code)
	return s.allowed[code], nil
}

func contextualAIChatFileSkillCatalogForTest() []skills.SkillDiscoveryMetadata {
	return []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillCalculator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillAgentManagement, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillConsoleNavigator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillFileGenerator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillFileManager, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillFileReader, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
	}
}

func contextualConsoleFilesAllCapabilityPartsForTest() *chatRequestParts {
	operationContext := map[string]interface{}{
		"schema":  "zgi.aichat.operation_context.v1",
		"version": 1,
		"resources": []interface{}{
			map[string]interface{}{
				"resource_type": "page",
				"resource_id":   "console.files",
				"title":         "console.files",
				"href":          "/console/files",
				"capability_ids": []interface{}{
					"file.list_visible",
					"file.read",
					"file.delete",
					"file.create",
				},
				"metadata": map[string]interface{}{
					"page":  "console.files",
					"route": "/console/files",
				},
			},
			map[string]interface{}{
				"resource_type": "file",
				"resource_id":   "file-1",
				"title":         "report.pdf",
				"href":          "/console/files",
				"capability_ids": []interface{}{
					"file.read",
					"file.delete",
				},
				"metadata": map[string]interface{}{
					"page":         "console.files",
					"file_id":      "file-1",
					"name":         "report.pdf",
					"workspace_id": "workspace-1",
				},
			},
		},
		"capabilities": []interface{}{
			map[string]interface{}{"id": "file.list_visible"},
			map[string]interface{}{"id": "file.read"},
			map[string]interface{}{"id": "file.delete"},
			map[string]interface{}{"id": "file.create"},
		},
	}
	return &chatRequestParts{
		Surface:             aiChatSurfaceContextualSidebar,
		RuntimeContext:      "route=/console/files",
		RawOperationContext: operationContext,
		OperationContext:    operationContext,
	}
}

func contextualConsoleAgentsManageCapabilityPartsForTest() *chatRequestParts {
	workspaceID := uuid.New().String()
	operationContext := map[string]interface{}{
		"schema":  "zgi.aichat.operation_context.v1",
		"version": 1,
		"resources": []interface{}{
			map[string]interface{}{
				"resource_type": "page",
				"resource_id":   "console.agents",
				"title":         "console.agents",
				"href":          "/console/agents",
				"capability_ids": []interface{}{
					"agent.list_visible",
					"agent.create_from_page",
					"agent.update_identity",
					"agent.delete_visible",
				},
				"metadata": map[string]interface{}{
					"page":         "console.agents",
					"route":        "/console/agents",
					"workspace_id": workspaceID,
				},
			},
			map[string]interface{}{
				"resource_type": "agent",
				"resource_id":   "agent-1",
				"title":         "Support Bot",
				"href":          "/console/agents/agent-1/agent",
				"metadata": map[string]interface{}{
					"resource_kind": "agent",
					"agent_id":      "agent-1",
					"name":          "Support Bot",
					"agent_type":    "AGENT",
					"workspace_id":  workspaceID,
				},
			},
		},
		"capabilities": []interface{}{
			map[string]interface{}{"id": "agent.list_visible"},
			map[string]interface{}{"id": "agent.create_from_page"},
			map[string]interface{}{"id": "agent.update_identity"},
			map[string]interface{}{"id": "agent.delete_visible"},
		},
	}
	return &chatRequestParts{
		Surface:             aiChatSurfaceContextualSidebar,
		RuntimeContext:      "route=/console/agents",
		RawOperationContext: operationContext,
		OperationContext:    operationContext,
	}
}

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

func TestEffectiveAgentSkillIDsRejectsSidebarManagedAssetSkills(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillAgentKnowledge, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}, RequiredConfig: []string{skills.SkillRequiredConfigAgentKnowledge}},
		{ID: skills.SkillAgentManagement, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillCalculator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAgent}},
		{ID: skills.SkillConsoleNavigator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillFileManager, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
	}

	got := effectiveAgentSkillIDs(
		[]string{
			skills.SkillAgentManagement,
			skills.SkillFileManager,
			skills.SkillConsoleNavigator,
			skills.SkillCalculator,
			skills.SkillAgentKnowledge,
		},
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
		{ID: skills.SkillConsoleNavigator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillFileManager, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillFileReader, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
	}
	parts := consoleFilesSnapshotTestParts("delete the first file", []consoleFilesTestFile{
		{ID: "file-1", Name: "old.pdf", Extension: "pdf", MimeType: "application/pdf"},
	})
	parts.Surface = aiChatSurfaceContextualSidebar

	got := addContextualAIChatSkillIDs(
		[]string{skills.SkillCalculator},
		[]string{skills.SkillCalculator, skills.SkillConsoleNavigator, skills.SkillFileManager, skills.SkillFileReader},
		catalog,
		parts,
	)
	want := []string{skills.SkillCalculator, skills.SkillConsoleNavigator, skills.SkillFileManager, skills.SkillFileReader}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("contextual skills = %#v, want %#v", got, want)
	}
}

func TestAddContextualAIChatSkillIDsAddsFileGeneratorForConsoleFileCreateCapability(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillCalculator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillConsoleNavigator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillFileGenerator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillFileManager, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillFileReader, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
	}
	parts := consoleFilesCreateCapabilityTestParts("create a txt file in File Management")
	parts.Surface = aiChatSurfaceContextualSidebar

	got := addContextualAIChatSkillIDs(
		[]string{skills.SkillCalculator},
		[]string{skills.SkillCalculator, skills.SkillConsoleNavigator, skills.SkillFileGenerator, skills.SkillFileReader},
		catalog,
		parts,
	)
	want := []string{skills.SkillCalculator, skills.SkillConsoleNavigator, skills.SkillFileGenerator, skills.SkillFileManager}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("contextual skills = %#v, want %#v", got, want)
	}
}

func TestAddContextualAIChatSkillIDsAddsConsoleNavigatorForSidebar(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillCalculator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillConsoleNavigator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
	}

	got := addContextualAIChatSkillIDs(
		[]string{skills.SkillCalculator},
		[]string{skills.SkillCalculator, skills.SkillConsoleNavigator},
		catalog,
		&chatRequestParts{Surface: aiChatSurfaceContextualSidebar, RuntimeContext: "route=/console/files"},
	)
	want := []string{skills.SkillCalculator, skills.SkillConsoleNavigator}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("contextual skills = %#v, want %#v", got, want)
	}
}

func TestAddContextualAIChatSkillIDsAddsAgentManagementForConsoleAgents(t *testing.T) {
	catalog := contextualAIChatFileSkillCatalogForTest()
	parts := contextualConsoleAgentsManageCapabilityPartsForTest()

	got := addContextualAIChatSkillIDs(
		[]string{skills.SkillCalculator},
		[]string{skills.SkillCalculator, skills.SkillConsoleNavigator},
		catalog,
		parts,
	)
	want := []string{skills.SkillAgentManagement, skills.SkillCalculator, skills.SkillConsoleNavigator}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("contextual agent skills = %#v, want %#v", got, want)
	}
}

func TestAddContextualAIChatSkillIDsDefersAgentManagementUntilAgentContext(t *testing.T) {
	catalog := contextualAIChatFileSkillCatalogForTest()
	parts := contextualConsoleFilesAllCapabilityPartsForTest()
	parts.Query = "请导航到智能体页面，并在当前工作空间创建两个临时测试 Agent 草稿"

	got := addContextualAIChatSkillIDsWithCapabilities(
		[]string{skills.SkillCalculator, skills.SkillConsoleNavigator},
		[]string{skills.SkillCalculator, skills.SkillConsoleNavigator},
		catalog,
		parts,
		contextualAIChatSkillCapabilities{Navigation: true, AgentRead: true, AgentManage: true},
	)
	want := []string{skills.SkillCalculator, skills.SkillConsoleNavigator}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cross-page agent create skills = %#v, want %#v", got, want)
	}
}

func TestAddContextualAIChatSkillIDsDoesNotAddAgentManagementForPureAgentsNavigation(t *testing.T) {
	catalog := contextualAIChatFileSkillCatalogForTest()
	parts := contextualConsoleFilesAllCapabilityPartsForTest()
	parts.Query = "打开智能体页面"

	got := addContextualAIChatSkillIDsWithCapabilities(
		[]string{skills.SkillCalculator, skills.SkillConsoleNavigator},
		[]string{skills.SkillCalculator, skills.SkillConsoleNavigator},
		catalog,
		parts,
		contextualAIChatSkillCapabilities{Navigation: true, AgentRead: true, AgentManage: true},
	)
	want := []string{skills.SkillCalculator, skills.SkillConsoleNavigator}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pure route skills = %#v, want %#v", got, want)
	}
}

func TestAddContextualAIChatSkillIDsRespectsOrganizationDisabledUserSelectableSkill(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillCalculator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillConsoleNavigator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillFileManager, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillFileReader, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
	}
	parts := contextualConsoleFilesAllCapabilityPartsForTest()

	got := addContextualAIChatSkillIDsWithCapabilities(
		[]string{skills.SkillCalculator},
		[]string{skills.SkillCalculator},
		catalog,
		parts,
		contextualAIChatSkillCapabilities{FileRead: true},
	)
	want := []string{skills.SkillCalculator}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("contextual skills = %#v, want %#v", got, want)
	}
}

func TestAddContextualAIChatSkillIDsSkipsWorkChatSurface(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillCalculator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillAgentManagement, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillConsoleNavigator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillFileManager, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillFileReader, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
	}
	parts := consoleFilesSnapshotTestParts("delete the first file", []consoleFilesTestFile{
		{ID: "file-1", Name: "old.pdf", Extension: "pdf", MimeType: "application/pdf"},
	})
	parts.Surface = aiChatSurfaceWorkChat

	got := addContextualAIChatSkillIDs(
		[]string{skills.SkillCalculator},
		[]string{skills.SkillCalculator, skills.SkillConsoleNavigator, skills.SkillFileManager, skills.SkillFileReader},
		catalog,
		parts,
	)
	want := []string{skills.SkillCalculator}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("contextual skills = %#v, want %#v", got, want)
	}

	agentParts := contextualConsoleAgentsManageCapabilityPartsForTest()
	agentParts.Surface = aiChatSurfaceWorkChat
	got = addContextualAIChatSkillIDs(
		[]string{skills.SkillCalculator},
		[]string{skills.SkillCalculator, skills.SkillConsoleNavigator},
		catalog,
		agentParts,
	)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("work chat agent skills = %#v, want %#v", got, want)
	}
}

func TestAddContextualAIChatSkillIDsWithCapabilitiesUsesTrustedCapabilities(t *testing.T) {
	catalog := contextualAIChatFileSkillCatalogForTest()
	organizationEnabled := []string{
		skills.SkillCalculator,
		skills.SkillConsoleNavigator,
		skills.SkillFileGenerator,
		skills.SkillFileReader,
	}
	parts := contextualConsoleFilesAllCapabilityPartsForTest()

	limited := addContextualAIChatSkillIDsWithCapabilities(
		[]string{skills.SkillCalculator},
		organizationEnabled,
		catalog,
		parts,
		contextualAIChatSkillCapabilities{Navigation: true},
	)
	wantLimited := []string{skills.SkillCalculator, skills.SkillConsoleNavigator}
	if !reflect.DeepEqual(limited, wantLimited) {
		t.Fatalf("limited contextual skills = %#v, want %#v", limited, wantLimited)
	}

	allowed := addContextualAIChatSkillIDsWithCapabilities(
		[]string{skills.SkillCalculator},
		organizationEnabled,
		catalog,
		parts,
		contextualAIChatSkillCapabilities{Navigation: true, FileRead: true, FileDelete: true, FileCreate: true},
	)
	wantAllowed := []string{
		skills.SkillCalculator,
		skills.SkillConsoleNavigator,
		skills.SkillFileGenerator,
		skills.SkillFileManager,
		skills.SkillFileReader,
	}
	if !reflect.DeepEqual(allowed, wantAllowed) {
		t.Fatalf("allowed contextual skills = %#v, want %#v", allowed, wantAllowed)
	}
}

func TestTrustedContextualAIChatSkillCapabilitiesRequireWorkspacePermissionService(t *testing.T) {
	parts := contextualConsoleFilesAllCapabilityPartsForTest()
	got := (&service{}).trustedContextualAIChatSkillCapabilities(context.Background(), Scope{
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
	}, parts)
	want := contextualAIChatSkillCapabilities{Navigation: true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("trusted contextual capabilities without workspace permissions = %#v, want %#v", got, want)
	}
}

func TestTrustedContextualAIChatSkillCapabilitiesUseWorkspacePermissions(t *testing.T) {
	workspaceID := uuid.New()
	permissionService := &skillConfigWorkspacePermissionService{
		allowed: map[workspacemodel.WorkspacePermissionCode]bool{
			workspacemodel.WorkspacePermissionFileDownload:     true,
			workspacemodel.WorkspacePermissionFileManage:       false,
			workspacemodel.WorkspacePermissionFileUploadCreate: true,
		},
	}
	got := (&service{workspacePerms: permissionService}).trustedContextualAIChatSkillCapabilities(context.Background(), Scope{
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
		WorkspaceID:    &workspaceID,
	}, contextualConsoleFilesAllCapabilityPartsForTest())
	want := contextualAIChatSkillCapabilities{Navigation: true, FileRead: true, FileDelete: false, FileCreate: true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("trusted contextual capabilities = %#v, want %#v", got, want)
	}

	wantCodes := []workspacemodel.WorkspacePermissionCode{
		workspacemodel.WorkspacePermissionFileDownload,
		workspacemodel.WorkspacePermissionFileManage,
		workspacemodel.WorkspacePermissionFileUploadCreate,
	}
	if !reflect.DeepEqual(permissionService.codes, wantCodes) {
		t.Fatalf("workspace permission checks = %#v, want %#v", permissionService.codes, wantCodes)
	}
}

func TestTrustedContextualAIChatSkillCapabilitiesUseAgentManagePermission(t *testing.T) {
	workspaceID := uuid.New()
	parts := contextualConsoleAgentsManageCapabilityPartsForTest()
	resources := parts.RawOperationContext["resources"].([]interface{})
	pageResource := resources[0].(map[string]interface{})
	metadata := pageResource["metadata"].(map[string]interface{})
	metadata["workspace_id"] = workspaceID.String()

	permissionService := &skillConfigWorkspacePermissionService{
		allowed: map[workspacemodel.WorkspacePermissionCode]bool{
			workspacemodel.WorkspacePermissionAgentView:   true,
			workspacemodel.WorkspacePermissionAgentManage: true,
		},
	}
	got := (&service{workspacePerms: permissionService}).trustedContextualAIChatSkillCapabilities(context.Background(), Scope{
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
		WorkspaceID:    &workspaceID,
	}, parts)
	want := contextualAIChatSkillCapabilities{Navigation: true, AgentRead: true, AgentManage: true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("trusted contextual agent capabilities = %#v, want %#v", got, want)
	}
}

func TestAddContextualAIChatSkillIDsDoesNotUseTextToInjectCrossPageAgentManagement(t *testing.T) {
	workspaceID := uuid.New()
	catalog := contextualAIChatFileSkillCatalogForTest()
	organizationEnabled := []string{skills.SkillCalculator, skills.SkillConsoleNavigator}
	parts := contextualConsoleFilesAllCapabilityPartsForTest()
	parts.Query = "create two temporary agents in the current workspace"
	resources := parts.RawOperationContext["resources"].([]interface{})
	pageResource := resources[0].(map[string]interface{})
	metadata := pageResource["metadata"].(map[string]interface{})
	metadata["workspace_id"] = workspaceID.String()

	readOnlyPerms := &skillConfigWorkspacePermissionService{
		allowed: map[workspacemodel.WorkspacePermissionCode]bool{
			workspacemodel.WorkspacePermissionAgentView:   true,
			workspacemodel.WorkspacePermissionAgentManage: false,
		},
	}
	readOnlyCapabilities := (&service{workspacePerms: readOnlyPerms}).trustedContextualAIChatSkillCapabilities(context.Background(), Scope{
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
	}, parts)
	readOnly := addContextualAIChatSkillIDsWithCapabilities(
		[]string{skills.SkillCalculator},
		organizationEnabled,
		catalog,
		parts,
		readOnlyCapabilities,
	)
	wantReadOnly := []string{skills.SkillCalculator, skills.SkillConsoleNavigator}
	if !reflect.DeepEqual(readOnly, wantReadOnly) {
		t.Fatalf("read-only cross-page mutation skills = %#v, want %#v", readOnly, wantReadOnly)
	}

	managePerms := &skillConfigWorkspacePermissionService{
		allowed: map[workspacemodel.WorkspacePermissionCode]bool{
			workspacemodel.WorkspacePermissionAgentManage: true,
		},
	}
	manageCapabilities := (&service{workspacePerms: managePerms}).trustedContextualAIChatSkillCapabilities(context.Background(), Scope{
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
	}, parts)
	allowed := addContextualAIChatSkillIDsWithCapabilities(
		[]string{skills.SkillCalculator},
		organizationEnabled,
		catalog,
		parts,
		manageCapabilities,
	)
	wantAllowed := []string{skills.SkillCalculator, skills.SkillConsoleNavigator}
	if !reflect.DeepEqual(allowed, wantAllowed) {
		t.Fatalf("managed cross-page mutation skills = %#v, want %#v", allowed, wantAllowed)
	}
}

func TestTrustedContextualAIChatSkillCapabilitiesUsesOperationContextWorkspace(t *testing.T) {
	workspaceID := uuid.New()
	parts := contextualConsoleFilesAllCapabilityPartsForTest()
	resources := parts.RawOperationContext["resources"].([]interface{})
	fileResource := resources[1].(map[string]interface{})
	metadata := fileResource["metadata"].(map[string]interface{})
	metadata["workspace_id"] = workspaceID.String()

	permissionService := &skillConfigWorkspacePermissionService{
		allowed: map[workspacemodel.WorkspacePermissionCode]bool{
			workspacemodel.WorkspacePermissionFileDownload:     true,
			workspacemodel.WorkspacePermissionFileManage:       true,
			workspacemodel.WorkspacePermissionFileUploadCreate: true,
		},
	}
	got := (&service{workspacePerms: permissionService}).trustedContextualAIChatSkillCapabilities(context.Background(), Scope{
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
	}, parts)
	want := contextualAIChatSkillCapabilities{Navigation: true, FileRead: true, FileDelete: true, FileCreate: true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("trusted contextual capabilities from operation context workspace = %#v, want %#v", got, want)
	}
}

func TestFilterAIChatSkillIDsForSurfaceRemovesNavigatorOutsideSidebar(t *testing.T) {
	got := filterAIChatSkillIDsForSurface(
		[]string{skills.SkillAgentManagement, skills.SkillCalculator, skills.SkillConsoleNavigator, skills.SkillFileManager, skills.SkillFileReader},
		&chatRequestParts{Surface: aiChatSurfaceWorkChat},
	)
	want := []string{skills.SkillCalculator, skills.SkillFileReader}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered skills = %#v, want %#v", got, want)
	}

	contextual := filterAIChatSkillIDsForSurface(
		[]string{skills.SkillAgentManagement, skills.SkillCalculator, skills.SkillConsoleNavigator, skills.SkillFileManager},
		&chatRequestParts{Surface: aiChatSurfaceContextualSidebar},
	)
	wantContextual := []string{skills.SkillCalculator, skills.SkillConsoleNavigator}
	if !reflect.DeepEqual(contextual, wantContextual) {
		t.Fatalf("contextual filtered skills = %#v, want %#v", contextual, wantContextual)
	}

	unknownSurface := filterAIChatSkillIDsForSurface(
		[]string{skills.SkillAgentManagement, skills.SkillCalculator, skills.SkillConsoleNavigator, skills.SkillFileManager, skills.SkillFileReader},
		nil,
	)
	if !reflect.DeepEqual(unknownSurface, want) {
		t.Fatalf("nil-parts filtered skills = %#v, want %#v", unknownSurface, want)
	}
}

func TestFilterAIChatSkillIDsForSurfaceRemovesSystemAssetSkillsFromExternalPageChat(t *testing.T) {
	got := filterAIChatSkillIDsForSurface(
		[]string{
			skills.SkillAgentManagement,
			skills.SkillCalculator,
			skills.SkillConsoleNavigator,
			skills.SkillFileGenerator,
			skills.SkillFileManager,
			skills.SkillFileReader,
			skills.SkillChartGenerator,
			skills.SkillInternalDatabase,
			skills.SkillInternalKnowledge,
		},
		&chatRequestParts{Surface: aiChatSurfaceExternalPageChat},
	)
	want := []string{skills.SkillCalculator, skills.SkillChartGenerator, skills.SkillFileGenerator, skills.SkillFileReader}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("external page chat skills = %#v, want %#v", got, want)
	}
}

func TestNormalizeAIChatSurfaceMapsExternalPageAliases(t *testing.T) {
	for _, raw := range []string{"external_page_chat", "external-page-chat", "external", "page-chat", "webapp", "agent-webapp"} {
		if got := normalizeAIChatSurface(raw); got != aiChatSurfaceExternalPageChat {
			t.Fatalf("normalizeAIChatSurface(%q) = %q, want %q", raw, got, aiChatSurfaceExternalPageChat)
		}
	}
}

func TestNormalizeRuntimeSurfaceForCallerForcesAgentExternalPageChat(t *testing.T) {
	got := normalizeRuntimeSurfaceForCaller(
		Caller{Type: runtimemodel.ConversationCallerAgent},
		aiChatSurfaceContextualSidebar,
	)
	if got != aiChatSurfaceExternalPageChat {
		t.Fatalf("agent runtime surface = %q, want %q", got, aiChatSurfaceExternalPageChat)
	}

	aichat := normalizeRuntimeSurfaceForCaller(
		Caller{Type: runtimemodel.ConversationCallerAIChat},
		aiChatSurfaceContextualSidebar,
	)
	if aichat != aiChatSurfaceContextualSidebar {
		t.Fatalf("aichat runtime surface = %q, want %q", aichat, aiChatSurfaceContextualSidebar)
	}
}

func TestAddUnconfiguredDefaultSkillIDsAddsMissingDefaultSystemSkill(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillCalculator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillConsoleNavigator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillFileReader, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
	}

	got := addUnconfiguredDefaultSkillIDs(
		[]string{skills.SkillCalculator},
		map[string]struct{}{skills.SkillCalculator: {}},
		catalog,
	)
	want := []string{skills.SkillCalculator, skills.SkillConsoleNavigator, skills.SkillFileReader}
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

func TestFilterSkillIDsForCallerHidesManagedAndSystemAssetSkillsFromManualAIChatPreferences(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillCalculator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillConsoleNavigator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillFileManager, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillFileReader, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillInternalDatabase, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillInternalKnowledge, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
	}

	got := filterSkillIDsForCaller(
		[]string{
			skills.SkillCalculator,
			skills.SkillConsoleNavigator,
			skills.SkillFileManager,
			skills.SkillFileReader,
			skills.SkillInternalDatabase,
			skills.SkillInternalKnowledge,
		},
		catalog,
		runtimemodel.ConversationCallerAIChat,
	)
	want := []string{skills.SkillCalculator, skills.SkillFileReader}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("manual preference skills = %#v, want %#v", got, want)
	}
}

func TestEffectiveSkillIDsForCallerDropsManagedAndSystemAssetRuntimeOverrides(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillCalculator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillConsoleNavigator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillFileManager, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillFileReader, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillInternalDatabase, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
		{ID: skills.SkillInternalKnowledge, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
	}

	got := effectiveSkillIDsForCaller(
		[]string{
			skills.SkillCalculator,
			skills.SkillConsoleNavigator,
			skills.SkillFileManager,
			skills.SkillFileReader,
			skills.SkillInternalDatabase,
			skills.SkillInternalKnowledge,
		},
		catalog,
		[]string{
			skills.SkillCalculator,
			skills.SkillConsoleNavigator,
			skills.SkillFileManager,
			skills.SkillFileReader,
			skills.SkillInternalDatabase,
			skills.SkillInternalKnowledge,
		},
		runtimemodel.ConversationCallerAIChat,
		nil,
	)
	want := []string{skills.SkillCalculator, skills.SkillFileReader}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime override skills = %#v, want %#v", got, want)
	}
}

func TestValidateSkillIDsForCallerRejectsSidebarManagedConsoleNavigator(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillConsoleNavigator, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
	}

	_, err := validateSkillIDsForCaller(
		[]string{skills.SkillConsoleNavigator},
		catalog,
		[]string{skills.SkillConsoleNavigator},
		runtimemodel.ConversationCallerAIChat,
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "not user selectable") {
		t.Fatalf("validateSkillIDsForCaller(console-navigator) error = %v, want user-selectable rejection", err)
	}
}

func TestValidateSkillIDsForCallerRejectsRuntimeManagedFileManager(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillFileManager, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
	}

	_, err := validateSkillIDsForCaller(
		[]string{skills.SkillFileManager},
		catalog,
		[]string{skills.SkillFileManager},
		runtimemodel.ConversationCallerAIChat,
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "managed by runtime configuration") {
		t.Fatalf("validateSkillIDsForCaller(file-manager) error = %v, want runtime-managed rejection", err)
	}
}

func TestValidateSkillIDsForCallerRejectsSystemAssetSkill(t *testing.T) {
	catalog := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillInternalDatabase, Status: skills.SkillStatusActive, SupportedCallers: []string{skills.SkillCallerAIChat}},
	}

	_, err := validateSkillIDsForCaller(
		[]string{skills.SkillInternalDatabase},
		catalog,
		[]string{skills.SkillInternalDatabase},
		runtimemodel.ConversationCallerAIChat,
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "not user selectable") {
		t.Fatalf("validateSkillIDsForCaller(internal-database) error = %v, want user-selectable rejection", err)
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

func TestSkillRuntimeParametersForPreparedUsesConversationWorkspaceWhenScopeMissing(t *testing.T) {
	workspaceID := uuid.New()
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New(), WorkspaceID: &workspaceID},
		Scope:        Scope{OrganizationID: uuid.New()},
		RunConfig:    RunConfig{},
	}

	params := skillRuntimeParametersForPrepared(prepared)
	if params["workspace_id"] != workspaceID.String() {
		t.Fatalf("workspace_id = %#v, want conversation workspace %s", params["workspace_id"], workspaceID)
	}
}

func TestSkillRuntimeParametersForPreparedDoesNotInferSelectedFileGovernanceAsset(t *testing.T) {
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
	assertNoGovernanceAssets(t, governance)
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
	if params["console_files_page"] != true {
		t.Fatalf("console_files_page = %#v, want true", params["console_files_page"])
	}
	if _, exists := params["file_generation_default_target"]; exists {
		t.Fatalf("file_generation_default_target = %#v, want omitted so generated files stay temporary by default", params["file_generation_default_target"])
	}
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
	if files[1]["file_type"] != "excel" || files[1]["file_type_rank"] != 1 || files[1]["extension_rank"] != 1 {
		t.Fatalf("second visible file ranks = %#v, want first excel/xlsx rank", files[1])
	}
}

func TestSkillRuntimeParametersForPreparedAddsConsoleAgentsVisibleAgents(t *testing.T) {
	prepared := &PreparedChat{
		Scope:     Scope{OrganizationID: uuid.New()},
		RunConfig: RunConfig{},
		parts:     contextualConsoleAgentsManageCapabilityPartsForTest(),
	}

	params := skillRuntimeParametersForPrepared(prepared)
	if params["console_agents_page"] != true {
		t.Fatalf("console_agents_page = %#v, want true", params["console_agents_page"])
	}
	if params["console_current_route"] != "/console/agents" || params["console_agents_current_route"] != "/console/agents" {
		t.Fatalf("console agent route params = %#v / %#v, want /console/agents", params["console_current_route"], params["console_agents_current_route"])
	}
	agents, ok := params["console_agents_visible_agents"].([]map[string]interface{})
	if !ok || len(agents) != 1 {
		t.Fatalf("console_agents_visible_agents = %#v, want one visible agent", params["console_agents_visible_agents"])
	}
	if agents[0]["agent_id"] != "agent-1" || agents[0]["name"] != "Support Bot" || agents[0]["type"] != "agent" {
		t.Fatalf("visible agent = %#v, want agent-1 named Support Bot", agents[0])
	}
}

func TestSkillRuntimeParametersForPreparedAddsRecentAgentMutationUpdates(t *testing.T) {
	prepared := &PreparedChat{
		Scope:     Scope{OrganizationID: uuid.New()},
		RunConfig: RunConfig{},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"skill_invocations": []interface{}{
				map[string]interface{}{
					"kind":      "tool_call",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_identity",
					"status":    "success",
					"result": map[string]interface{}{
						"status":      "completed",
						"agent_id":    "agent-1",
						"agent_name":  "Updated Support Bot",
						"description": "updated description",
						"agent": map[string]interface{}{
							"id":           "agent-1",
							"name":         "Updated Support Bot",
							"workspace_id": "workspace-1",
						},
					},
				},
			},
		}},
		parts: contextualConsoleAgentsManageCapabilityPartsForTest(),
	}

	params := skillRuntimeParametersForPrepared(prepared)
	recent, ok := params["console_agents_recent_agent_updates"].([]map[string]interface{})
	if !ok || len(recent) != 1 {
		t.Fatalf("console_agents_recent_agent_updates = %#v, want one recent Agent update", params["console_agents_recent_agent_updates"])
	}
	if recent[0]["agent_id"] != "agent-1" || recent[0]["name"] != "Updated Support Bot" || recent[0]["description"] != "updated description" {
		t.Fatalf("recent Agent update = %#v, want updated identity evidence", recent[0])
	}
}

func TestSkillRuntimeParametersForPreparedAddsRecentAgentMutationFromOperationPlan(t *testing.T) {
	prepared := &PreparedChat{
		Scope:     Scope{OrganizationID: uuid.New()},
		RunConfig: RunConfig{},
		Message: &runtimemodel.Message{Metadata: map[string]interface{}{
			"operation_plan": map[string]interface{}{
				"tool_result": map[string]interface{}{
					"kind":      "tool_call",
					"skill_id":  skills.SkillAgentManagement,
					"tool_name": "update_agent_identity",
					"status":    "success",
					"result_summary": map[string]interface{}{
						"status":       "completed",
						"effect":       "updated",
						"agent_id":     "agent-1",
						"agent_name":   "Plan Result Agent",
						"workspace_id": "workspace-1",
					},
				},
			},
		}},
		parts: contextualConsoleAgentsManageCapabilityPartsForTest(),
	}

	params := skillRuntimeParametersForPrepared(prepared)
	recent, ok := params["console_agents_recent_agent_updates"].([]map[string]interface{})
	if !ok || len(recent) != 1 {
		t.Fatalf("console_agents_recent_agent_updates = %#v, want one recent Agent update", params["console_agents_recent_agent_updates"])
	}
	if recent[0]["agent_id"] != "agent-1" || recent[0]["name"] != "Plan Result Agent" || recent[0]["workspace_id"] != "workspace-1" {
		t.Fatalf("recent Agent update = %#v, want operation_plan result evidence", recent[0])
	}
}

func TestSkillRuntimeParametersForPreparedDoesNotInferOrdinalFileGovernanceAsset(t *testing.T) {
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
	assertNoGovernanceAssets(t, governance)
}

func TestSkillRuntimeParametersForPreparedDoesNotInferChineseSpreadsheetGovernanceAsset(t *testing.T) {
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
	assertNoGovernanceAssets(t, governance)
}

func TestSkillRuntimeParametersForPreparedDoesNotInferNamedFileGovernanceAsset(t *testing.T) {
	prepared := &PreparedChat{
		Scope:     Scope{OrganizationID: uuid.New()},
		RunConfig: RunConfig{},
		parts: consoleFilesSemanticTestParts("delete file codex-smoke-delete-visible-20260615-1538.txt", []consoleFilesTestFile{
			{ID: "file-1", Name: "codex-smoke-delete-visible-20260615-1538.txt", Extension: "txt"},
			{ID: "file-2", Name: "other.txt", Extension: "txt"},
		}),
	}

	governance := governanceRuntimeParamsFromTest(t, skillRuntimeParametersForPrepared(prepared))
	assertNoGovernanceAssets(t, governance)
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

func TestSkillRuntimeParametersForPreparedIncludesToolGovernanceProfileScope(t *testing.T) {
	prepared := &PreparedChat{
		Scope:     Scope{OrganizationID: uuid.New()},
		Caller:    Caller{Type: runtimemodel.ConversationCallerAgent},
		RunConfig: RunConfig{},
		parts: &chatRequestParts{
			Surface: aiChatSurfaceContextualSidebar,
			RawOperationContext: map[string]interface{}{
				"tool_governance": map[string]interface{}{
					"permission_tier": "advanced",
				},
			},
		},
	}

	governance := governanceRuntimeParamsFromTest(t, skillRuntimeParametersForPrepared(prepared))
	if governance["permission_tier"] != "basic" {
		t.Fatalf("permission_tier = %#v, want basic", governance["permission_tier"])
	}
	if governance["caller_type"] != runtimemodel.ConversationCallerAgent {
		t.Fatalf("caller_type = %#v, want agent", governance["caller_type"])
	}
	if governance["runtime_surface"] != aiChatSurfaceExternalPageChat {
		t.Fatalf("runtime_surface = %#v, want external_page_chat", governance["runtime_surface"])
	}
}

func TestSkillRuntimeParametersForPreparedForcesBasicPermissionTierForExternalPageChat(t *testing.T) {
	prepared := &PreparedChat{
		Scope:     Scope{OrganizationID: uuid.New()},
		Caller:    Caller{Type: runtimemodel.ConversationCallerAIChat},
		RunConfig: RunConfig{},
		parts: &chatRequestParts{
			Surface: aiChatSurfaceExternalPageChat,
			RawOperationContext: map[string]interface{}{
				"tool_governance": map[string]interface{}{
					"permission_tier": "full",
				},
			},
		},
	}

	governance := governanceRuntimeParamsFromTest(t, skillRuntimeParametersForPrepared(prepared))
	if governance["permission_tier"] != "basic" {
		t.Fatalf("permission_tier = %#v, want basic", governance["permission_tier"])
	}
	if governance["runtime_surface"] != aiChatSurfaceExternalPageChat {
		t.Fatalf("runtime_surface = %#v, want external_page_chat", governance["runtime_surface"])
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

func assertNoGovernanceAssets(t *testing.T, governance map[string]interface{}) {
	t.Helper()
	if _, exists := governance["assets"]; exists {
		t.Fatalf("governance assets = %#v, want omitted", governance["assets"])
	}
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

func TestVisibleSkillMetadataHidesNonUserSelectableSystemSkills(t *testing.T) {
	metadata := []skills.SkillDiscoveryMetadata{
		{ID: skills.SkillInternalKnowledge},
		{ID: skills.SkillInternalDatabase},
		{ID: skills.SkillAgentManagement},
		{ID: skills.SkillAgentKnowledge},
		{ID: skills.SkillAgentDatabase},
		{ID: skills.SkillAgentWorkflow},
		{ID: skills.SkillAgentMemory},
		{ID: skills.SkillUserMemory},
		{ID: skills.SkillConsoleNavigator},
		{ID: skills.SkillFileManager},
		{ID: skills.SkillFileReader},
		{ID: skills.SkillCalculator},
	}

	got := visibleSkillMetadata(metadata)
	gotIDs := make([]string, 0, len(got))
	for _, item := range got {
		gotIDs = append(gotIDs, item.ID)
	}
	want := []string{skills.SkillFileReader, skills.SkillCalculator}
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

func TestProcessTimelineRecorderAvoidsRuntimeIDCollisionAcrossContinuations(t *testing.T) {
	prepared := preparedTimelineTestChat()
	firstRecorder := newProcessTimelineRecorder(context.Background(), context.Background(), &service{}, prepared, nil)
	firstRecorder.RecordEvent(streamEventSkillCallStart, map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"skill_id":        skills.SkillConsoleNavigator,
		"tool_name":       "navigate",
		"created_at":      int64(100),
	})
	firstRecorder.RecordEvent(streamEventSkillCallEnd, map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"skill_id":        skills.SkillConsoleNavigator,
		"tool_name":       "navigate",
		"status":          "success",
		"created_at":      int64(101),
		"result":          map[string]interface{}{"href": "/console/files"},
	})

	continuationRecorder := newProcessTimelineRecorder(context.Background(), context.Background(), &service{}, prepared, nil)
	continuationRecorder.RecordEvent(streamEventSkillCallStart, map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"skill_id":        skills.SkillConsoleNavigator,
		"tool_name":       "navigate",
		"created_at":      int64(200),
	})
	continuationRecorder.RecordEvent(streamEventSkillCallEnd, map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"skill_id":        skills.SkillConsoleNavigator,
		"tool_name":       "navigate",
		"status":          "success",
		"created_at":      int64(201),
		"result":          map[string]interface{}{"href": "/console/agents"},
	})

	invocations := skillInvocationsFromMetadata(prepared.Message.Metadata["skill_invocations"])
	if len(invocations) != 2 {
		t.Fatalf("skill_invocations = %#v, want two separate navigate invocations", prepared.Message.Metadata["skill_invocations"])
	}
	if got := stringFromAny(invocations[0]["runtime_id"]); got != "tool_call:console-navigator:navigate::#1" {
		t.Fatalf("first runtime_id = %q, want #1", got)
	}
	if got := stringFromAny(invocations[1]["runtime_id"]); got != "tool_call:console-navigator:navigate::#2" {
		t.Fatalf("second runtime_id = %q, want #2", got)
	}
	if href := stringFromAny(governanceMapFromAny(invocations[0]["result"])["href"]); href != "/console/files" {
		t.Fatalf("first href = %q, want /console/files", href)
	}
	if href := stringFromAny(governanceMapFromAny(invocations[1]["result"])["href"]); href != "/console/agents" {
		t.Fatalf("second href = %q, want /console/agents", href)
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
