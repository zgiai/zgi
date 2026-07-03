package agentmanagement

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmmodelservice "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func TestNormalizeAgentIconParamsWrapsTextIcon(t *testing.T) {
	iconType, icon := normalizeAgentIconParams("", "AI", "")
	if iconType != "text" {
		t.Fatalf("iconType = %q, want text", iconType)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(icon), &payload); err != nil {
		t.Fatalf("icon is not JSON: %v", err)
	}
	if payload["icon"] != "AI" || payload["icon_background"] != defaultAgentTextIconBackground {
		t.Fatalf("payload = %#v, want normalized text icon", payload)
	}
}

func TestNormalizeAgentIconParamsUsesRequestedBackground(t *testing.T) {
	iconType, icon := normalizeAgentIconParams("text", "P3", "#0f766e")
	if iconType != "text" {
		t.Fatalf("iconType = %q, want text", iconType)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(icon), &payload); err != nil {
		t.Fatalf("icon is not JSON: %v", err)
	}
	if payload["icon"] != "P3" || payload["icon_background"] != "#0f766e" {
		t.Fatalf("payload = %#v, want requested text icon background", payload)
	}
}

func TestNormalizeAgentIconParamsKeepsImageIcon(t *testing.T) {
	iconType, icon := normalizeAgentIconParams("image", "file-1", "")
	if iconType != "image" || icon != "file-1" {
		t.Fatalf("normalizeAgentIconParams() = %q, %q, want image file-1", iconType, icon)
	}
}

func TestDefaultAgentTextIcon(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "客服助手", want: "客服"},
		{name: "Sales Bot", want: "SB"},
		{name: "agent", want: "AG"},
		{name: "  ---  ", want: "AI"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := defaultAgentTextIcon(tt.name); got != tt.want {
				t.Fatalf("defaultAgentTextIcon(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestAgentScopeFromRuntimeRejectsNonAIChatCallers(t *testing.T) {
	for _, invokeFrom := range []tools.ToolInvokeFrom{
		tools.ToolInvokeFromAgent,
		tools.ToolInvokeFromWorkflow,
		tools.ToolInvokeFromAPI,
	} {
		t.Run(string(invokeFrom), func(t *testing.T) {
			_, err := agentScopeFromRuntime(&tools.ToolRuntime{
				TenantID:   "org-1",
				InvokeFrom: invokeFrom,
				RuntimeParameters: map[string]interface{}{
					"organization_id": "org-1",
					"workspace_id":    "workspace-1",
				},
			}, "org-1", "account-1")
			if err == nil {
				t.Fatal("agentScopeFromRuntime() error = nil, want non-AIChat caller rejection")
			}
			if !strings.Contains(err.Error(), "only available from AIChat runtime") {
				t.Fatalf("agentScopeFromRuntime() error = %q, want AIChat-only boundary", err)
			}
		})
	}
}

func TestAgentScopeFromRuntimeAcceptsAIChatCaller(t *testing.T) {
	scope, err := agentScopeFromRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "workspace-1",
		},
	}, "org-1", "account-1")
	if err != nil {
		t.Fatalf("agentScopeFromRuntime() error = %v", err)
	}
	if scope.OrganizationID != "org-1" || scope.WorkspaceID != "workspace-1" || scope.AccountID != "account-1" {
		t.Fatalf("agentScopeFromRuntime() = %#v, want scoped AIChat caller", scope)
	}
}

func TestCreateAgentAppliesDefaultTextIconAndPayloadEvidence(t *testing.T) {
	service := &fakeAgentManagementService{}
	perms := &fakeWorkspacePermissionService{allowed: true}
	tool := newCreateAgentTool(service, perms).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "workspace-1",
		},
	})

	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"name":        "Smoke Helper",
		"description": "Agent identity smoke",
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if perms.workspaceID != "workspace-1" || perms.accountID != "account-1" || perms.permissionCode != workspacemodel.WorkspacePermissionAgentManage {
		t.Fatalf("permission check = org %q workspace %q account %q permission %q, want current workspace agent manage", perms.organizationID, perms.workspaceID, perms.accountID, perms.permissionCode)
	}
	if service.createAgentCalls != 1 || service.lastCreateAgentWorkspaceID != "workspace-1" {
		t.Fatalf("CreateAgent calls = %d workspace=%q, want one current workspace call", service.createAgentCalls, service.lastCreateAgentWorkspaceID)
	}
	if service.lastCreateAgentRequest["icon_type"] != "text" {
		t.Fatalf("create icon_type = %#v, want text", service.lastCreateAgentRequest["icon_type"])
	}
	iconText := stringValue(service.lastCreateAgentRequest, "icon")
	var iconPayload map[string]interface{}
	if err := json.Unmarshal([]byte(iconText), &iconPayload); err != nil {
		t.Fatalf("create icon is not JSON: %v", err)
	}
	if iconPayload["icon"] != "SH" || iconPayload["icon_background"] != defaultAgentTextIconBackground {
		t.Fatalf("icon payload = %#v, want default initials and background", iconPayload)
	}
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	payload := messages[0].Data
	if payload["agent_name"] != "Smoke Helper" || payload["agent_description"] != "Agent identity smoke" {
		t.Fatalf("payload identity = %#v, want top-level name/description evidence", payload)
	}
	if payload["agent_icon_type"] != "text" || payload["agent_icon"] == "" {
		t.Fatalf("payload icon = type %#v icon %#v, want top-level icon evidence", payload["agent_icon_type"], payload["agent_icon"])
	}
}

func TestAgentMemorySlotsParamParsesJSON(t *testing.T) {
	slots, ok, err := agentMemorySlotsParam(map[string]interface{}{
		"agent_memory_slots": `[{"key":"profile","description":"User profile facts","enabled":true,"sort_order":2}]`,
	}, "agent_memory_slots")
	if err != nil {
		t.Fatalf("agentMemorySlotsParam() error = %v", err)
	}
	if !ok {
		t.Fatal("agentMemorySlotsParam() ok = false, want true")
	}
	if len(slots) != 1 {
		t.Fatalf("agentMemorySlotsParam() len = %d, want 1", len(slots))
	}
	if slots[0].Key != "profile" || slots[0].Description != "User profile facts" || !slots[0].Enabled || slots[0].SortOrder != 2 {
		t.Fatalf("agentMemorySlotsParam() = %#v, want parsed slot", slots[0])
	}
}

func TestAgentMemorySlotsParamCopiesTypedSlots(t *testing.T) {
	input := []dto.AgentMemorySlotConfig{{Key: "profile", Enabled: true}}
	slots, ok, err := agentMemorySlotsParam(map[string]interface{}{"slots": input}, "agent_memory_slots", "slots")
	if err != nil {
		t.Fatalf("agentMemorySlotsParam() error = %v", err)
	}
	if !ok {
		t.Fatal("agentMemorySlotsParam() ok = false, want true")
	}
	slots[0].Key = "changed"
	if input[0].Key != "profile" {
		t.Fatalf("agentMemorySlotsParam() mutated input key = %q, want profile", input[0].Key)
	}
}

func TestAgentMemorySlotsParamRejectsObject(t *testing.T) {
	_, ok, err := agentMemorySlotsParam(map[string]interface{}{
		"agent_memory_slots": `{"key":"profile"}`,
	}, "agent_memory_slots")
	if !ok {
		t.Fatal("agentMemorySlotsParam() ok = false, want true for present invalid value")
	}
	if err == nil {
		t.Fatal("agentMemorySlotsParam() error = nil, want invalid JSON array error")
	}
}

func TestAgentDatabaseBindingsParamParsesJSON(t *testing.T) {
	bindings, ok, err := agentDatabaseBindingsParam(map[string]interface{}{
		"bindings": `[{"data_source_id":"db-1","table_ids":["table-1"],"writable_table_ids":["table-1"]}]`,
	}, "bindings")
	if err != nil {
		t.Fatalf("agentDatabaseBindingsParam() error = %v", err)
	}
	if !ok {
		t.Fatal("agentDatabaseBindingsParam() ok = false, want true")
	}
	want := []dto.AgentDatabaseBinding{{DataSourceID: "db-1", TableIDs: []string{"table-1"}, WritableTableIDs: []string{"table-1"}}}
	if !reflect.DeepEqual(bindings, want) {
		t.Fatalf("agentDatabaseBindingsParam() = %#v, want %#v", bindings, want)
	}
}

func TestAgentWorkflowBindingsParamRejectsObject(t *testing.T) {
	_, ok, err := agentWorkflowBindingsParam(map[string]interface{}{
		"bindings": `{"binding_id":"workflow-1"}`,
	}, "bindings")
	if !ok {
		t.Fatal("agentWorkflowBindingsParam() ok = false, want true for present invalid value")
	}
	if err == nil {
		t.Fatal("agentWorkflowBindingsParam() error = nil, want invalid JSON array error")
	}
}

func TestReplaceAgentKnowledgeBindingsPreservesOtherConfigFields(t *testing.T) {
	agentID := "agent-1"
	current := &dto.AgentDraftRuntimeConfigResponse{
		AgentID:     agentID,
		WorkspaceID: "agent-workspace",
		Config: dto.AgentConfigResponse{
			AgentID:             agentID,
			SystemPrompt:        "keep prompt",
			ModelProvider:       "openai",
			Model:               "gpt-4o",
			ModelParameters:     map[string]interface{}{"temperature": float64(0.2)},
			EnabledSkillIDs:     []string{"time"},
			AgentMemoryEnabled:  true,
			FileUpload:          true,
			HomeTitle:           "Home",
			InputPlaceholder:    "Ask",
			ThemeColor:          "blue",
			SuggestedQuestions:  []string{"What changed?"},
			KnowledgeDatasetIDs: []string{"dataset-old"},
			DatabaseBindings:    []dto.AgentDatabaseBinding{{DataSourceID: "db-1", TableIDs: []string{"table-1"}}},
			WorkflowBindings:    []dto.AgentWorkflowBinding{{BindingID: "workflow-1", AgentID: "workflow-1", WorkflowID: "wf-1", VersionStrategy: "latest_published"}},
		},
	}
	service := &fakeAgentManagementService{
		draftConfig: current,
		agents: map[string]interface{}{
			agentID: map[string]interface{}{"id": agentID, "name": "Support Agent", "workspace_id": "agent-workspace"},
		},
	}
	tool := newReplaceAgentKnowledgeBindingsTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "runtime-workspace",
		},
	})
	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id":         agentID,
		"dataset_ids":      `["dataset-new"]`,
		"retrieval_config": `{"top_k":3}`,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if service.getDraftConfigCalls != 1 || service.updateConfigCalls != 1 {
		t.Fatalf("service calls get=%d update=%d, want 1/1", service.getDraftConfigCalls, service.updateConfigCalls)
	}
	if !reflect.DeepEqual(service.lastConfigRequest.KnowledgeDatasetIDs, []string{"dataset-new"}) {
		t.Fatalf("KnowledgeDatasetIDs = %#v, want dataset-new", service.lastConfigRequest.KnowledgeDatasetIDs)
	}
	if service.lastConfigRequest.KnowledgeRetrievalConfig["top_k"] != float64(3) {
		t.Fatalf("KnowledgeRetrievalConfig = %#v, want top_k 3", service.lastConfigRequest.KnowledgeRetrievalConfig)
	}
	if service.lastConfigRequest.SystemPrompt != "keep prompt" || service.lastConfigRequest.ModelProvider != "openai" || service.lastConfigRequest.Model != "gpt-4o" {
		t.Fatalf("preserved config = %#v, want prompt/model fields preserved", service.lastConfigRequest)
	}
	if !reflect.DeepEqual(service.lastConfigRequest.DatabaseBindings, current.Config.DatabaseBindings) {
		t.Fatalf("DatabaseBindings = %#v, want preserved %#v", service.lastConfigRequest.DatabaseBindings, current.Config.DatabaseBindings)
	}
	if !reflect.DeepEqual(service.lastConfigRequest.WorkflowBindings, current.Config.WorkflowBindings) {
		t.Fatalf("WorkflowBindings = %#v, want preserved %#v", service.lastConfigRequest.WorkflowBindings, current.Config.WorkflowBindings)
	}
	if len(messages) != 1 || messages[0].Data["workspace_id"] != "agent-workspace" {
		t.Fatalf("payload = %#v, want agent workspace", messages)
	}
}

func TestUpdateAgentConfigRequiresProviderWhenChangingModel(t *testing.T) {
	agentID := "agent-1"
	service := &fakeAgentManagementService{
		draftConfig: &dto.AgentDraftRuntimeConfigResponse{
			AgentID:     agentID,
			WorkspaceID: "agent-workspace",
			Config: dto.AgentConfigResponse{
				AgentID:       agentID,
				ModelProvider: "deepseek",
				Model:         "deepseek-chat",
			},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
		},
	})
	_, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id": agentID,
		"model":    "gpt-4o",
	}, nil, nil, nil)
	if err == nil {
		t.Fatal("Invoke() error = nil, want missing model_provider error")
	}
	if service.updateConfigCalls != 0 {
		t.Fatalf("UpdateAgentConfig calls = %d, want 0", service.updateConfigCalls)
	}
}

func TestUpdateAgentConfigRequiresModelWhenChangingProvider(t *testing.T) {
	agentID := "agent-1"
	service := &fakeAgentManagementService{
		draftConfig: &dto.AgentDraftRuntimeConfigResponse{
			AgentID:     agentID,
			WorkspaceID: "agent-workspace",
			Config: dto.AgentConfigResponse{
				AgentID:       agentID,
				ModelProvider: "deepseek",
				Model:         "deepseek-chat",
			},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
		},
	})
	_, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id":       agentID,
		"model_provider": "openai",
	}, nil, nil, nil)
	if err == nil {
		t.Fatal("Invoke() error = nil, want missing model error")
	}
	if !strings.Contains(err.Error(), "model is required when changing model_provider") {
		t.Fatalf("Invoke() error = %q, want missing model error", err.Error())
	}
	if service.updateConfigCalls != 0 {
		t.Fatalf("UpdateAgentConfig calls = %d, want 0", service.updateConfigCalls)
	}
}

func TestAgentToolGovernanceArgumentEnrichmentAddsAgentDisplayContext(t *testing.T) {
	agentID := "agent-1"
	service := &fakeAgentManagementService{
		agents: map[string]interface{}{
			agentID: map[string]interface{}{"id": agentID, "name": "Support Agent", "workspace_id": "agent-workspace"},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "runtime-workspace",
		},
	})
	enricher, ok := tool.(tools.ToolGovernanceArgumentEnricher)
	if !ok {
		t.Fatal("tool does not implement ToolGovernanceArgumentEnricher")
	}
	enriched := enricher.EnrichGovernanceArguments(context.Background(), "account-1", map[string]interface{}{
		"agent_id":      agentID,
		"system_prompt": "updated prompt",
	})
	if enriched["agent_name"] != "Support Agent" {
		t.Fatalf("agent_name = %#v, want Support Agent", enriched["agent_name"])
	}
	if enriched["workspace_id"] != "agent-workspace" {
		t.Fatalf("workspace_id = %#v, want agent-workspace", enriched["workspace_id"])
	}
}

func TestAgentToolGovernanceArgumentEnrichmentAddsChangedFieldsPreview(t *testing.T) {
	agentID := "agent-1"
	service := &fakeAgentManagementService{
		draftConfig: &dto.AgentDraftRuntimeConfigResponse{
			AgentID:     agentID,
			WorkspaceID: "agent-workspace",
			Config: dto.AgentConfigResponse{
				AgentID:            agentID,
				SystemPrompt:       "old prompt",
				HomeTitle:          "Old title",
				InputPlaceholder:   "Ask me",
				SuggestedQuestions: []string{"old one"},
			},
		},
		agents: map[string]interface{}{
			agentID: map[string]interface{}{"id": agentID, "name": "Support Agent", "workspace_id": "agent-workspace"},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "runtime-workspace",
		},
	})
	enricher, ok := tool.(tools.ToolGovernanceArgumentEnricher)
	if !ok {
		t.Fatal("tool does not implement ToolGovernanceArgumentEnricher")
	}
	enriched := enricher.EnrichGovernanceArguments(context.Background(), "account-1", map[string]interface{}{
		"agent_id":            agentID,
		"system_prompt":       "new prompt",
		"home_title":          "New title",
		"input_placeholder":   "Ask me",
		"suggested_questions": []interface{}{"new one", "new two"},
	})
	got, ok := enriched["changed_fields_preview"].([]string)
	if !ok {
		t.Fatalf("changed_fields_preview = %#v, want []string", enriched["changed_fields_preview"])
	}
	want := []string{"system_prompt", "home_title", "suggested_questions"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("changed_fields_preview = %#v, want %#v", got, want)
	}
}

func TestAgentToolGovernanceArgumentEnrichmentAddsRemovedSkillDisplayName(t *testing.T) {
	agentID := "agent-1"
	service := &fakeAgentManagementService{
		draftConfig: &dto.AgentDraftRuntimeConfigResponse{
			AgentID:     agentID,
			WorkspaceID: "agent-workspace",
			Config: dto.AgentConfigResponse{
				AgentID:         agentID,
				ModelProvider:   "openai",
				Model:           "gpt-4o",
				EnabledSkillIDs: []string{"chart-generator"},
			},
		},
		agents: map[string]interface{}{
			agentID: map[string]interface{}{"id": agentID, "name": "Support Agent", "workspace_id": "agent-workspace"},
		},
		skillCandidatesResp: &dto.AgentSkillCandidatesResponse{
			AgentID:     agentID,
			WorkspaceID: "agent-workspace",
			Count:       1,
			Data: []dto.AgentSkillCandidate{{
				SkillID: "chart-generator",
				Name:    "Chart Generator",
			}},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
		},
	})
	enricher, ok := tool.(tools.ToolGovernanceArgumentEnricher)
	if !ok {
		t.Fatal("tool does not implement ToolGovernanceArgumentEnricher")
	}
	enriched := enricher.EnrichGovernanceArguments(context.Background(), "account-1", map[string]interface{}{
		"agent_id":          agentID,
		"enabled_skill_ids": []interface{}{},
	})
	displayNames := mapFromAny(enriched["display_names"])
	skills := mapFromAny(displayNames["skills"])
	if skills["chart-generator"] != "Chart Generator" {
		t.Fatalf("display_names.skills = %#v, want removed skill name", skills)
	}
}

func TestAgentToolGovernanceArgumentEnrichmentFallsBackForRemovedSkillDisplayName(t *testing.T) {
	agentID := "agent-1"
	service := &fakeAgentManagementService{
		draftConfig: &dto.AgentDraftRuntimeConfigResponse{
			AgentID:     agentID,
			WorkspaceID: "agent-workspace",
			Config: dto.AgentConfigResponse{
				AgentID:         agentID,
				EnabledSkillIDs: []string{"chart-generator"},
			},
		},
		agents: map[string]interface{}{
			agentID: map[string]interface{}{"id": agentID, "name": "Support Agent", "workspace_id": "agent-workspace"},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
		},
	})
	enricher, ok := tool.(tools.ToolGovernanceArgumentEnricher)
	if !ok {
		t.Fatal("tool does not implement ToolGovernanceArgumentEnricher")
	}
	enriched := enricher.EnrichGovernanceArguments(context.Background(), "account-1", map[string]interface{}{
		"agent_id":          agentID,
		"enabled_skill_ids": []interface{}{},
	})
	displayNames := mapFromAny(enriched["display_names"])
	skills := mapFromAny(displayNames["skills"])
	if skills["chart-generator"] != "Chart Generator" {
		t.Fatalf("display_names.skills = %#v, want fallback skill name", skills)
	}
	preview := mapFromAny(enriched["binding_change_preview"])
	assertStringSliceContains(t, stringSliceFromAny(preview["removed_resource_names"]), "Chart Generator")
	assertStringSliceContains(t, stringSliceFromAny(preview["removed_resource_ids"]), "chart-generator")
}

func TestAgentToolGovernanceArgumentEnrichmentAddsPartialUnbindPreview(t *testing.T) {
	agentID := "agent-1"
	service := &fakeAgentManagementService{
		draftConfig: &dto.AgentDraftRuntimeConfigResponse{
			AgentID:     agentID,
			WorkspaceID: "agent-workspace",
			Config: dto.AgentConfigResponse{
				AgentID: agentID,
				DatabaseBindings: []dto.AgentDatabaseBinding{{
					DataSourceID: "db-1",
					TableIDs:     []string{"table-1", "table-2"},
				}},
			},
		},
		agents: map[string]interface{}{
			agentID: map[string]interface{}{"id": agentID, "name": "Support Agent", "workspace_id": "agent-workspace"},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
		},
	})
	enricher, ok := tool.(tools.ToolGovernanceArgumentEnricher)
	if !ok {
		t.Fatal("tool does not implement ToolGovernanceArgumentEnricher")
	}
	enriched := enricher.EnrichGovernanceArguments(context.Background(), "account-1", map[string]interface{}{
		"agent_id": agentID,
		"database_bindings": []interface{}{
			map[string]interface{}{"data_source_id": "db-1", "table_ids": []interface{}{"table-2"}},
		},
		"display_names": map[string]interface{}{
			"database_tables": map[string]string{
				"db-1:table-1": "Old Orders",
				"db-1:table-2": "Current Orders",
			},
		},
	})
	preview := mapFromAny(enriched["binding_change_preview"])
	if preview["binding_kind"] != "multiple" && preview["binding_kind"] != "database_table" {
		t.Fatalf("binding_change_preview = %#v, want database table unbind preview", preview)
	}
	if preview["change_action"] != "unbind" {
		t.Fatalf("binding_change_preview.change_action = %#v, want unbind; preview=%#v", preview["change_action"], preview)
	}
	if preview["removed_resource_count"] != 1 || preview["resource_count"] != 1 {
		t.Fatalf("preview counts = removed:%#v resource:%#v, want 1", preview["removed_resource_count"], preview["resource_count"])
	}
	assertStringSliceContains(t, stringSliceFromAny(preview["removed_resource_names"]), "Old Orders")
	assertStringSliceContains(t, stringSliceFromAny(preview["removed_resource_ids"]), "db-1:table-1")
	assertStringSliceContains(t, stringSliceFromAny(preview["resource_ids"]), "db-1:table-1")
	changes, ok := enriched["binding_changes_preview"].([]map[string]interface{})
	if !ok || len(changes) != 1 {
		t.Fatalf("binding_changes_preview = %#v, want one preview change", enriched["binding_changes_preview"])
	}
	if changes[0]["change_action"] != "unbind" {
		t.Fatalf("binding_changes_preview[0].change_action = %#v, want unbind", changes[0]["change_action"])
	}
}

func TestAgentToolGovernanceArgumentEnrichmentKeepsMixedBindingActions(t *testing.T) {
	agentID := "agent-1"
	service := &fakeAgentManagementService{
		draftConfig: &dto.AgentDraftRuntimeConfigResponse{
			AgentID:     agentID,
			WorkspaceID: "agent-workspace",
			Config: dto.AgentConfigResponse{
				AgentID:             agentID,
				EnabledSkillIDs:     []string{"chart-generator"},
				KnowledgeDatasetIDs: []string{},
				DatabaseBindings: []dto.AgentDatabaseBinding{{
					DataSourceID: "db-1",
					TableIDs:     []string{"table-remove", "table-keep"},
				}},
				WorkflowBindings: []dto.AgentWorkflowBinding{},
			},
		},
		agents: map[string]interface{}{
			agentID: map[string]interface{}{"id": agentID, "name": "Support Agent", "workspace_id": "agent-workspace"},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
		},
	})
	enricher, ok := tool.(tools.ToolGovernanceArgumentEnricher)
	if !ok {
		t.Fatal("tool does not implement ToolGovernanceArgumentEnricher")
	}
	enriched := enricher.EnrichGovernanceArguments(context.Background(), "account-1", map[string]interface{}{
		"agent_id":                 agentID,
		"remove_enabled_skill_ids": []interface{}{"chart-generator"},
		"add_knowledge_dataset_ids": []interface{}{
			"dataset-add",
		},
		"remove_database_bindings": []interface{}{
			map[string]interface{}{"data_source_id": "db-1", "table_ids": []interface{}{"table-remove"}},
		},
		"add_workflow_bindings": []interface{}{
			map[string]interface{}{
				"binding_id":       "workflow-add",
				"label":            "Escalation Flow",
				"agent_id":         "workflow-agent-add",
				"workflow_id":      "wf-add",
				"version_strategy": "latest_published",
			},
		},
		"display_names": map[string]interface{}{
			"skills": map[string]string{
				"chart-generator": "Chart Generator",
			},
			"knowledge_bases": map[string]string{
				"dataset-add": "New Knowledge",
			},
			"database_tables": map[string]string{
				"db-1:table-remove": "Remove Table",
				"db-1:table-keep":   "Keep Table",
			},
			"workflows": map[string]string{
				"workflow-add": "Escalation Flow",
			},
		},
	})

	if enriched["agent_name"] != "Support Agent" {
		t.Fatalf("agent_name = %#v, want Support Agent", enriched["agent_name"])
	}
	preview := mapFromAny(enriched["binding_change_preview"])
	if preview["binding_kind"] != "multiple" || preview["change_action"] != "replace" {
		t.Fatalf("binding_change_preview = %#v, want mixed replace preview", preview)
	}
	changes, ok := enriched["binding_changes_preview"].([]map[string]interface{})
	if !ok || len(changes) != 4 {
		t.Fatalf("binding_changes_preview = %#v, want four directional changes", enriched["binding_changes_preview"])
	}
	changesByField := map[string]map[string]interface{}{}
	for _, change := range changes {
		field, _ := change["field"].(string)
		changesByField[field] = change
	}
	assertAgentBindingPreviewChange(t, changesByField["enabled_skill_ids"], "agent_skill", "unbind", "Chart Generator")
	assertAgentBindingPreviewChange(t, changesByField["knowledge_dataset_ids"], "knowledge_base", "bind", "New Knowledge")
	assertAgentBindingPreviewChange(t, changesByField["database_bindings"], "database_table", "unbind", "Remove Table")
	assertAgentBindingPreviewChange(t, changesByField["workflow_bindings"], "workflow", "bind", "Escalation Flow")
}

func TestAgentToolGovernanceArgumentEnrichmentNormalizesSyntheticDatabaseRemoval(t *testing.T) {
	agentID := "agent-1"
	service := &fakeAgentManagementService{
		draftConfig: &dto.AgentDraftRuntimeConfigResponse{
			AgentID:     agentID,
			WorkspaceID: "agent-workspace",
			Config: dto.AgentConfigResponse{
				AgentID: agentID,
				DatabaseBindings: []dto.AgentDatabaseBinding{{
					DataSourceID: "real-db-1",
					TableIDs:     []string{"test1"},
				}},
				WorkflowBindings: []dto.AgentWorkflowBinding{{
					BindingID:  "workflow-binding-1",
					WorkflowID: "workflow-1",
					AgentID:    "workflow-agent-1",
					Label:      "新功能测试",
				}},
			},
		},
		agents: map[string]interface{}{
			agentID: map[string]interface{}{"id": agentID, "name": "Support Agent", "workspace_id": "agent-workspace"},
		},
		databaseTablesRespByDataSource: map[string]*dto.AgentDatabaseTablesResponse{
			"real-db-1": {
				DataSourceID: "real-db-1",
				Data: []dto.AgentDatabaseTableCandidate{{
					DataSourceID: "real-db-1",
					TableID:      "test1",
					Name:         "测试库1.test1",
				}},
			},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
		},
	})
	params := map[string]interface{}{
		"agent_id":                 agentID,
		"remove_database_bindings": `[{"data_source_id":"test-db","table_ids":["test1"]}]`,
		"remove_workflow_bindings": `[{"binding_id":"workflow-binding-1"}]`,
		"display_names": map[string]interface{}{
			"database_tables": map[string]string{
				"test-db:test1": "测试库1.test1",
			},
			"workflows": map[string]string{
				"workflow-binding-1": "新功能测试",
			},
		},
	}
	enricher, ok := tool.(tools.ToolGovernanceArgumentEnricher)
	if !ok {
		t.Fatal("tool does not implement ToolGovernanceArgumentEnricher")
	}
	enriched := enricher.EnrichGovernanceArguments(context.Background(), "account-1", params)
	normalizedRemovals, ok, err := agentDatabaseBindingsParam(enriched, "remove_database_bindings")
	if err != nil || !ok || len(normalizedRemovals) != 1 {
		t.Fatalf("remove_database_bindings = %#v ok=%v err=%v, want one normalized binding", enriched["remove_database_bindings"], ok, err)
	}
	if normalizedRemovals[0].DataSourceID != "real-db-1" || !reflect.DeepEqual(normalizedRemovals[0].TableIDs, []string{"test1"}) {
		t.Fatalf("normalized remove_database_bindings = %#v, want real-db-1/test1", normalizedRemovals)
	}
	changes, ok := enriched["binding_changes_preview"].([]map[string]interface{})
	if !ok || len(changes) != 2 {
		t.Fatalf("binding_changes_preview = %#v, want database and workflow unbind previews", enriched["binding_changes_preview"])
	}
	changesByField := map[string]map[string]interface{}{}
	for _, change := range changes {
		field := stringValue(change, "field")
		if field != "" {
			changesByField[field] = change
		}
	}
	assertAgentBindingPreviewChange(t, changesByField["database_bindings"], "database_table", "unbind", "测试库1.test1")
	assertAgentBindingPreviewChange(t, changesByField["workflow_bindings"], "workflow", "unbind", "新功能测试")
	preview := mapFromAny(enriched["binding_change_preview"])
	if preview["binding_kind"] != "multiple" || preview["change_action"] != "unbind" || preview["resource_count"] != 2 {
		t.Fatalf("binding_change_preview = %#v, want multiple unbind of two resources", preview)
	}

	messages, err := tool.Invoke(context.Background(), "account-1", params, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(service.lastConfigRequest.DatabaseBindings) != 0 {
		t.Fatalf("DatabaseBindings = %#v, want empty after synthetic removal normalization", service.lastConfigRequest.DatabaseBindings)
	}
	if len(service.lastConfigRequest.WorkflowBindings) != 0 {
		t.Fatalf("WorkflowBindings = %#v, want empty after removal", service.lastConfigRequest.WorkflowBindings)
	}
	payloadChanges, ok := messages[0].Data["binding_changes"].([]map[string]interface{})
	if !ok || len(payloadChanges) != 2 {
		t.Fatalf("binding_changes = %#v, want database and workflow unbind results", messages[0].Data["binding_changes"])
	}
}

func TestAgentToolGovernanceArgumentEnrichmentAddsReplaceDatabaseUnbindPreview(t *testing.T) {
	agentID := "agent-1"
	service := &fakeAgentManagementService{
		draftConfig: &dto.AgentDraftRuntimeConfigResponse{
			AgentID:     agentID,
			WorkspaceID: "agent-workspace",
			Config: dto.AgentConfigResponse{
				AgentID: agentID,
				DatabaseBindings: []dto.AgentDatabaseBinding{{
					DataSourceID: "db-1",
					TableIDs:     []string{"table-1"},
				}},
			},
		},
		agents: map[string]interface{}{
			agentID: map[string]interface{}{"id": agentID, "name": "Support Agent", "workspace_id": "agent-workspace"},
		},
	}
	tool := newReplaceAgentDatabaseBindingsTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
		},
	})
	enricher, ok := tool.(tools.ToolGovernanceArgumentEnricher)
	if !ok {
		t.Fatal("tool does not implement ToolGovernanceArgumentEnricher")
	}
	enriched := enricher.EnrichGovernanceArguments(context.Background(), "account-1", map[string]interface{}{
		"agent_id": agentID,
		"bindings": []interface{}{},
		"display_names": map[string]interface{}{
			"database_tables": map[string]string{
				"db-1:table-1": "Orders",
			},
		},
	})

	if enriched["agent_name"] != "Support Agent" {
		t.Fatalf("agent_name = %#v, want Support Agent", enriched["agent_name"])
	}
	if _, ok := enriched["database_bindings"]; !ok {
		t.Fatalf("database_bindings = %#v, want normalized governance field", enriched["database_bindings"])
	}
	preview := mapFromAny(enriched["binding_change_preview"])
	if preview["binding_kind"] != "database_table" || preview["change_action"] != "unbind" {
		t.Fatalf("binding_change_preview = %#v, want database table unbind", preview)
	}
	if preview["resource_count"] != 1 || preview["removed_resource_count"] != 1 {
		t.Fatalf("preview counts = %#v, want one removed resource", preview)
	}
	assertStringSliceContains(t, stringSliceFromAny(preview["removed_resource_names"]), "Orders")
}

func assertAgentBindingPreviewChange(t *testing.T, change map[string]interface{}, kind string, action string, name string) {
	t.Helper()
	if len(change) == 0 {
		t.Fatalf("missing preview change for %s %s", kind, action)
	}
	if change["binding_kind"] != kind || change["change_action"] != action {
		t.Fatalf("preview change = %#v, want %s %s", change, kind, action)
	}
	switch action {
	case "bind":
		assertStringSliceContains(t, stringSliceFromAny(change["added_resource_names"]), name)
	case "unbind":
		assertStringSliceContains(t, stringSliceFromAny(change["removed_resource_names"]), name)
	default:
		assertStringSliceContains(t, stringSliceFromAny(change["resource_names"]), name)
	}
}

func TestAgentToolGovernanceArgumentEnrichmentResolvesVisibleNameIdentifier(t *testing.T) {
	tool := newUpdateAgentConfigTool(&fakeAgentManagementService{}).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "workspace-1",
			"console_agents_visible_agents": []map[string]interface{}{
				{"id": "agent-1", "name": "Support Agent", "workspace_id": "workspace-1"},
			},
		},
	})
	enricher, ok := tool.(tools.ToolGovernanceArgumentEnricher)
	if !ok {
		t.Fatal("tool does not implement ToolGovernanceArgumentEnricher")
	}
	enriched := enricher.EnrichGovernanceArguments(context.Background(), "account-1", map[string]interface{}{
		"agent_id":      "Support Agent",
		"system_prompt": "updated prompt",
	})
	if enriched["agent_id"] != "agent-1" {
		t.Fatalf("agent_id = %#v, want agent-1", enriched["agent_id"])
	}
	if enriched["agent_name"] != "Support Agent" {
		t.Fatalf("agent_name = %#v, want Support Agent", enriched["agent_name"])
	}
	if enriched["workspace_id"] != "workspace-1" {
		t.Fatalf("workspace_id = %#v, want workspace-1", enriched["workspace_id"])
	}
}

func TestAgentToolGovernanceArgumentEnrichmentResolvesWorkspaceNameIdentifier(t *testing.T) {
	service := &fakeAgentManagementService{
		agents: map[string]interface{}{
			"agent-1": map[string]interface{}{"id": "agent-1", "name": "Support Agent", "workspace_id": "workspace-1"},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "workspace-1",
		},
	})
	enricher, ok := tool.(tools.ToolGovernanceArgumentEnricher)
	if !ok {
		t.Fatal("tool does not implement ToolGovernanceArgumentEnricher")
	}
	enriched := enricher.EnrichGovernanceArguments(context.Background(), "account-1", map[string]interface{}{
		"agent_id":      "Support Agent",
		"system_prompt": "updated prompt",
	})
	if enriched["agent_id"] != "agent-1" || enriched["agent_name"] != "Support Agent" {
		t.Fatalf("enriched = %#v, want resolved agent id and display name", enriched)
	}
	if service.lastListAgentsRequest.WorkspaceID != "workspace-1" {
		t.Fatalf("list request = %#v, want current workspace", service.lastListAgentsRequest)
	}
}

func TestGetAgentResolvesVisibleNameIdentifier(t *testing.T) {
	service := &fakeAgentManagementService{
		agents: map[string]interface{}{
			"agent-1": map[string]interface{}{"id": "agent-1", "name": "Support Agent", "workspace_id": "workspace-1"},
		},
	}
	tool := newGetAgentTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "workspace-1",
			"console_agents_visible_agents": []map[string]interface{}{
				{"id": "agent-1", "name": "Support Agent", "workspace_id": "workspace-1"},
			},
		},
	})
	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id": "Support Agent",
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(messages) != 1 || messages[0].Data["status"] != "completed" {
		t.Fatalf("messages = %#v, want completed get agent payload", messages)
	}
	agent := mapFromAny(messages[0].Data["agent"])
	if agent["agent_id"] != "agent-1" || agent["name"] != "Support Agent" {
		t.Fatalf("agent payload = %#v, want resolved Support Agent", agent)
	}
}

func TestGetAgentResolvesUniqueWorkspaceNameIdentifier(t *testing.T) {
	service := &fakeAgentManagementService{
		agents: map[string]interface{}{
			"agent-1": map[string]interface{}{"id": "agent-1", "name": "Support Agent", "workspace_id": "workspace-1"},
			"agent-2": map[string]interface{}{"id": "agent-2", "name": "Other Agent", "workspace_id": "workspace-1"},
		},
	}
	tool := newGetAgentTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "workspace-1",
		},
	})
	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id": "Support Agent",
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if service.lastListAgentsRequest.WorkspaceID != "workspace-1" || service.lastListAgentsRequest.Keyword != "Support Agent" {
		t.Fatalf("list request = %#v, want current workspace keyword search", service.lastListAgentsRequest)
	}
	agent := mapFromAny(messages[0].Data["agent"])
	if agent["agent_id"] != "agent-1" || agent["name"] != "Support Agent" {
		t.Fatalf("agent payload = %#v, want workspace-name resolved Support Agent", agent)
	}
}

func TestGetAgentConfigResolvesUniqueWorkspaceNameIdentifier(t *testing.T) {
	agentID := "agent-1"
	service := &fakeAgentManagementService{
		agents: map[string]interface{}{
			agentID: map[string]interface{}{"id": agentID, "name": "Support Agent", "workspace_id": "workspace-1"},
		},
		draftConfig: &dto.AgentDraftRuntimeConfigResponse{
			AgentID:     agentID,
			WorkspaceID: "workspace-1",
			Config:      dto.AgentConfigResponse{AgentID: agentID, SystemPrompt: "prompt"},
		},
	}
	tool := newGetAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "workspace-1",
		},
	})
	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id": "Support Agent",
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if service.lastDraftConfigAgentID != agentID {
		t.Fatalf("GetAgentDraftRuntimeConfig agentID = %q, want %q", service.lastDraftConfigAgentID, agentID)
	}
	if messages[0].Data["agent_id"] != agentID {
		t.Fatalf("payload agent_id = %#v, want %s", messages[0].Data["agent_id"], agentID)
	}
}

func TestGetAgentConfigResolvesUniqueWorkspaceKeywordIdentifier(t *testing.T) {
	agentID := "agent-1"
	service := &fakeAgentManagementService{
		agents: map[string]interface{}{
			agentID: map[string]interface{}{"id": agentID, "name": "Support Agent Edited", "workspace_id": "workspace-1"},
		},
		draftConfig: &dto.AgentDraftRuntimeConfigResponse{
			AgentID:     agentID,
			WorkspaceID: "workspace-1",
			Config:      dto.AgentConfigResponse{AgentID: agentID, SystemPrompt: "prompt"},
		},
	}
	tool := newGetAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "workspace-1",
		},
	})
	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id": "Support Agent",
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if service.lastDraftConfigAgentID != agentID {
		t.Fatalf("GetAgentDraftRuntimeConfig agentID = %q, want %q", service.lastDraftConfigAgentID, agentID)
	}
	if messages[0].Data["agent_id"] != agentID {
		t.Fatalf("payload agent_id = %#v, want %s", messages[0].Data["agent_id"], agentID)
	}
}

func TestUpdateAgentConfigCanPatchMultipleBindingSections(t *testing.T) {
	agentID := "agent-1"
	current := &dto.AgentDraftRuntimeConfigResponse{
		AgentID:     agentID,
		WorkspaceID: "agent-workspace",
		Config: dto.AgentConfigResponse{
			AgentID:             agentID,
			SystemPrompt:        "keep prompt",
			ModelProvider:       "openai",
			Model:               "gpt-4o",
			EnabledSkillIDs:     []string{"time"},
			KnowledgeDatasetIDs: []string{"dataset-old"},
			DatabaseBindings:    []dto.AgentDatabaseBinding{{DataSourceID: "db-1", TableIDs: []string{"table-1"}}},
			WorkflowBindings:    []dto.AgentWorkflowBinding{{BindingID: "workflow-1", Label: "Approval Flow", AgentID: "workflow-agent-1", WorkflowID: "wf-1", VersionStrategy: "latest_published"}},
		},
	}
	service := &fakeAgentManagementService{
		draftConfig: current,
		agents: map[string]interface{}{
			agentID: map[string]interface{}{"id": agentID, "name": "Support Agent", "workspace_id": "agent-workspace"},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
		},
	})

	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id":              agentID,
		"enabled_skill_ids":     `["time"]`,
		"knowledge_dataset_ids": `[]`,
		"database_bindings":     `[]`,
		"workflow_bindings":     `[]`,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if service.updateConfigCalls != 1 {
		t.Fatalf("UpdateAgentConfig calls = %d, want 1", service.updateConfigCalls)
	}
	if len(service.lastConfigRequest.KnowledgeDatasetIDs) != 0 || len(service.lastConfigRequest.DatabaseBindings) != 0 || len(service.lastConfigRequest.WorkflowBindings) != 0 {
		t.Fatalf("binding request = knowledge:%#v database:%#v workflow:%#v, want all cleared", service.lastConfigRequest.KnowledgeDatasetIDs, service.lastConfigRequest.DatabaseBindings, service.lastConfigRequest.WorkflowBindings)
	}
	if !reflect.DeepEqual(service.lastConfigRequest.EnabledSkillIDs, []string{"time"}) {
		t.Fatalf("EnabledSkillIDs = %#v, want preserved", service.lastConfigRequest.EnabledSkillIDs)
	}
	payload := messages[0].Data
	if payload["agent_name"] != "Support Agent" || payload["binding_kind"] != "multiple" || payload["change_action"] != "unbind" {
		t.Fatalf("payload summary = %#v, want named multi unbind", payload)
	}
	if payload["removed_resource_count"] != 3 || payload["resource_count"] != 3 {
		t.Fatalf("payload counts = removed:%#v resource:%#v, want 3", payload["removed_resource_count"], payload["resource_count"])
	}
	changes, ok := payload["config_changes"].([]map[string]interface{})
	if !ok || len(changes) != 3 {
		t.Fatalf("config_changes = %#v, want three binding changes", payload["config_changes"])
	}
	fields, ok := payload["updated_fields"].([]string)
	if !ok {
		t.Fatalf("updated_fields = %#v, want []string", payload["updated_fields"])
	}
	if reflect.DeepEqual(fields, []string{"enabled_skill_ids", "knowledge_dataset_ids", "database_bindings", "workflow_bindings"}) {
		t.Fatalf("updated_fields = %#v, want actual changed fields without preserved enabled_skill_ids", fields)
	}
	if reflect.DeepEqual(fields, []string{"knowledge_dataset_ids", "database_bindings", "workflow_bindings"}) == false {
		t.Fatalf("updated_fields = %#v, want only actual changed binding fields", fields)
	}
}

func TestUpdateAgentConfigUsesParamAgentNameFallbackWhenLookupMissing(t *testing.T) {
	agentID := "agent-1"
	current := &dto.AgentDraftRuntimeConfigResponse{
		AgentID:     agentID,
		WorkspaceID: "agent-workspace",
		Config: dto.AgentConfigResponse{
			AgentID:       agentID,
			ModelProvider: "openai",
			Model:         "gpt-4o",
			HomeTitle:     "Old title",
		},
	}
	service := &fakeAgentManagementService{
		draftConfig: current,
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
		},
	})

	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id":     agentID,
		"agent_name":   "Frozen Support Agent",
		"workspace_id": "agent-workspace",
		"home_title":   "New title",
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	payload := messages[0].Data
	if payload["agent_name"] != "Frozen Support Agent" {
		t.Fatalf("agent_name = %#v, want frozen argument fallback", payload["agent_name"])
	}
	agent, ok := payload["agent"].(map[string]interface{})
	if !ok || agent["name"] != "Frozen Support Agent" || agent["agent_name"] != "Frozen Support Agent" {
		t.Fatalf("agent payload = %#v, want frozen argument fallback names", payload["agent"])
	}
	if agent["workspace_id"] != "agent-workspace" {
		t.Fatalf("agent workspace = %#v, want frozen workspace fallback", agent["workspace_id"])
	}
}

func TestUpdateAgentConfigAcceptsNestedConfigWrapper(t *testing.T) {
	agentID := "agent-1"
	current := &dto.AgentDraftRuntimeConfigResponse{
		AgentID:     agentID,
		WorkspaceID: "agent-workspace",
		Config: dto.AgentConfigResponse{
			AgentID:             agentID,
			SystemPrompt:        "keep prompt",
			ModelProvider:       "openai",
			Model:               "gpt-4o",
			EnabledSkillIDs:     []string{"chart-generator"},
			KnowledgeDatasetIDs: []string{"dataset-old"},
			DatabaseBindings:    []dto.AgentDatabaseBinding{{DataSourceID: "db-1", TableIDs: []string{"table-1"}}},
			WorkflowBindings:    []dto.AgentWorkflowBinding{{BindingID: "workflow-1", Label: "Approval Flow", AgentID: "workflow-agent-1", WorkflowID: "wf-1", VersionStrategy: "latest_published"}},
		},
	}
	service := &fakeAgentManagementService{
		draftConfig: current,
		agents: map[string]interface{}{
			agentID: map[string]interface{}{"id": agentID, "name": "Support Agent", "workspace_id": "agent-workspace"},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
		},
	})

	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id": agentID,
		"config": map[string]interface{}{
			"enabled_skill_ids":     []interface{}{},
			"knowledge_dataset_ids": []interface{}{},
			"database_bindings":     []interface{}{},
			"workflow_bindings":     []interface{}{},
			"display_names": map[string]interface{}{
				"skills":          map[string]string{"chart-generator": "Chart Generator"},
				"knowledge_bases": map[string]string{"dataset-old": "Old Knowledge"},
				"database_tables": map[string]string{"db-1:table-1": "Orders"},
				"workflows":       map[string]string{"workflow-1": "Approval Flow"},
			},
		},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if service.updateConfigCalls != 1 {
		t.Fatalf("UpdateAgentConfig calls = %d, want 1", service.updateConfigCalls)
	}
	if len(service.lastConfigRequest.EnabledSkillIDs) != 0 ||
		len(service.lastConfigRequest.KnowledgeDatasetIDs) != 0 ||
		len(service.lastConfigRequest.DatabaseBindings) != 0 ||
		len(service.lastConfigRequest.WorkflowBindings) != 0 {
		t.Fatalf("cleared config = skills:%#v knowledge:%#v database:%#v workflow:%#v, want all empty",
			service.lastConfigRequest.EnabledSkillIDs,
			service.lastConfigRequest.KnowledgeDatasetIDs,
			service.lastConfigRequest.DatabaseBindings,
			service.lastConfigRequest.WorkflowBindings,
		)
	}
	payload := messages[0].Data
	if payload["binding_kind"] != "multiple" || payload["change_action"] != "unbind" {
		t.Fatalf("payload summary = %#v, want multi unbind", payload)
	}
	if payload["removed_resource_count"] != 4 || payload["resource_count"] != 4 {
		t.Fatalf("payload counts = removed:%#v resource:%#v, want 4", payload["removed_resource_count"], payload["resource_count"])
	}
	if got := stringSliceFromAny(payload["resource_names"]); !reflect.DeepEqual(got, []string{"Chart Generator", "Old Knowledge", "Orders", "Approval Flow"}) {
		t.Fatalf("resource_names = %#v, want nested display_names", got)
	}
}

func TestReplaceAgentKnowledgeBindingsUsesVisibleAgentNameFallback(t *testing.T) {
	agentID := "agent-1"
	current := &dto.AgentDraftRuntimeConfigResponse{
		AgentID:     agentID,
		WorkspaceID: "agent-workspace",
		Config: dto.AgentConfigResponse{
			AgentID:             agentID,
			SystemPrompt:        "keep prompt",
			ModelProvider:       "openai",
			Model:               "gpt-4o",
			KnowledgeDatasetIDs: []string{"dataset-old"},
		},
	}
	service := &fakeAgentManagementService{draftConfig: current}
	tool := newReplaceAgentKnowledgeBindingsTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
			"console_agents_visible_agents": []map[string]interface{}{
				{
					"id":           agentID,
					"name":         "Visible Support Agent",
					"workspace_id": "agent-workspace",
				},
			},
		},
	})

	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id":    agentID,
		"dataset_ids": `[]`,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	payload := messages[0].Data
	if payload["agent_name"] != "Visible Support Agent" {
		t.Fatalf("agent_name = %#v, want visible Agent name fallback", payload["agent_name"])
	}
	agent, ok := payload["agent"].(map[string]interface{})
	if !ok || agent["name"] != "Visible Support Agent" {
		t.Fatalf("agent payload = %#v, want visible Agent name fallback", payload["agent"])
	}
	if payload["binding_kind"] != "knowledge_base" || payload["change_action"] != "unbind" {
		t.Fatalf("binding summary = kind:%#v action:%#v, want knowledge_base unbind", payload["binding_kind"], payload["change_action"])
	}
	if payload["removed_resource_count"] != 1 || payload["resource_count"] != 1 {
		t.Fatalf("binding counts = removed:%#v resource:%#v, want one removed", payload["removed_resource_count"], payload["resource_count"])
	}
}

func TestAgentGovernanceArgumentsPreferCurrentServiceAgentName(t *testing.T) {
	agentID := "agent-1"
	service := &fakeAgentManagementService{
		draftConfig: &dto.AgentDraftRuntimeConfigResponse{
			AgentID:     agentID,
			WorkspaceID: "agent-workspace",
			Config: dto.AgentConfigResponse{
				AgentID:       agentID,
				ModelProvider: "openai",
				Model:         "gpt-4o",
			},
		},
		agents: map[string]interface{}{
			agentID: map[string]interface{}{
				"id":           agentID,
				"name":         "Current Service Agent",
				"workspace_id": "agent-workspace",
			},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
			"console_agents_visible_agents": []map[string]interface{}{
				{
					"id":           agentID,
					"name":         "Stale Visible Agent",
					"workspace_id": "agent-workspace",
				},
			},
		},
	})
	enricher, ok := tool.(interface {
		EnrichGovernanceArguments(context.Context, string, map[string]interface{}) map[string]interface{}
	})
	if !ok {
		t.Fatal("update_agent_config tool does not expose governance argument enrichment")
	}

	enriched := enricher.EnrichGovernanceArguments(context.Background(), "account-1", map[string]interface{}{
		"agent_id":     agentID,
		"agent_name":   "Stale Argument Agent",
		"home_title":   "Updated Home",
		"workspace_id": "agent-workspace",
	})

	if got := stringValue(enriched, "agent_name"); got != "Current Service Agent" {
		t.Fatalf("agent_name = %q, want current service name", got)
	}
}

func TestAgentGovernanceArgumentsPreferRecentMutationAgentName(t *testing.T) {
	agentID := "agent-1"
	service := &fakeAgentManagementService{
		draftConfig: &dto.AgentDraftRuntimeConfigResponse{
			AgentID:     agentID,
			WorkspaceID: "agent-workspace",
			Config: dto.AgentConfigResponse{
				AgentID:       agentID,
				ModelProvider: "openai",
				Model:         "gpt-4o",
			},
		},
		agents: map[string]interface{}{
			agentID: map[string]interface{}{
				"id":           agentID,
				"name":         "Stale Service Agent",
				"workspace_id": "agent-workspace",
			},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
			"console_agents_visible_agents": []map[string]interface{}{{
				"id":           agentID,
				"name":         "Stale Visible Agent",
				"workspace_id": "agent-workspace",
			}},
			"console_agents_recent_agent_updates": []map[string]interface{}{{
				"id":           agentID,
				"agent_id":     agentID,
				"name":         "Fresh Mutation Agent",
				"agent_name":   "Fresh Mutation Agent",
				"workspace_id": "agent-workspace",
			}},
		},
	})
	enricher, ok := tool.(interface {
		EnrichGovernanceArguments(context.Context, string, map[string]interface{}) map[string]interface{}
	})
	if !ok {
		t.Fatal("update_agent_config tool does not expose governance argument enrichment")
	}

	enriched := enricher.EnrichGovernanceArguments(context.Background(), "account-1", map[string]interface{}{
		"agent_id":     agentID,
		"agent_name":   "Stale Argument Agent",
		"agents":       `[{"agent_id":"agent-1","name":"Stale Argument Agent","workspace_id":"agent-workspace"}]`,
		"home_title":   "Updated Home",
		"workspace_id": "agent-workspace",
	})

	if got := stringValue(enriched, "agent_name"); got != "Fresh Mutation Agent" {
		t.Fatalf("agent_name = %q, want recent mutation name", got)
	}
	agents := mapsFromAny(enriched["agents"])
	if len(agents) != 1 {
		t.Fatalf("agents = %#v, want one enriched governance target", enriched["agents"])
	}
	if got := stringValue(agents[0], "agent_name"); got != "Fresh Mutation Agent" {
		t.Fatalf("agents[0].agent_name = %q, want Fresh Mutation Agent", got)
	}
	if got := stringValue(agents[0], "name"); got != "Fresh Mutation Agent" {
		t.Fatalf("agents[0].name = %q, want Fresh Mutation Agent", got)
	}
}

func TestUpdateAgentConfigReturnsDisplayNamesForBindingChanges(t *testing.T) {
	agentID := "agent-1"
	current := &dto.AgentDraftRuntimeConfigResponse{
		AgentID:     agentID,
		WorkspaceID: "agent-workspace",
		Config: dto.AgentConfigResponse{
			AgentID:             agentID,
			SystemPrompt:        "keep prompt",
			ModelProvider:       "openai",
			Model:               "gpt-4o",
			EnabledSkillIDs:     []string{"time"},
			KnowledgeDatasetIDs: []string{"dataset-old"},
			DatabaseBindings:    []dto.AgentDatabaseBinding{{DataSourceID: "db-1", TableIDs: []string{"table-1"}}},
			WorkflowBindings:    []dto.AgentWorkflowBinding{{BindingID: "workflow-1", Label: "Approval Flow", AgentID: "workflow-agent-1", WorkflowID: "wf-1", VersionStrategy: "latest_published"}},
		},
	}
	service := &fakeAgentManagementService{
		draftConfig: current,
		agents: map[string]interface{}{
			agentID: map[string]interface{}{"id": agentID, "name": "Support Agent", "workspace_id": "agent-workspace"},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
		},
	})

	displayNames := map[string]interface{}{
		"skills": map[string]string{
			"chart-generator": "图表生成器",
			"time":            "时间工具",
		},
		"knowledge_bases": map[string]string{
			"dataset-old": "旧知识库",
			"dataset-new": "新知识库",
		},
		"database_tables": []map[string]interface{}{
			{"data_source_id": "db-1", "table_id": "table-1", "name": "旧订单表"},
			{"data_source_id": "db-1", "table_id": "table-2", "name": "新订单表"},
		},
		"workflows": map[string]string{
			"workflow-1": "审批流程",
			"workflow-2": "退款流程",
		},
	}
	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id":              agentID,
		"enabled_skill_ids":     `["time","chart-generator"]`,
		"knowledge_dataset_ids": `["dataset-new"]`,
		"database_bindings":     `[{"data_source_id":"db-1","table_ids":["table-2"]}]`,
		"workflow_bindings":     `[{"binding_id":"workflow-2","label":"Refund Flow","agent_id":"workflow-agent-2","workflow_id":"wf-2","version_strategy":"latest_published"}]`,
		"display_names":         displayNames,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	payload := messages[0].Data
	changes, ok := payload["binding_changes"].([]map[string]interface{})
	if !ok || len(changes) != 4 {
		t.Fatalf("binding_changes = %#v, want four named changes", payload["binding_changes"])
	}
	changesByField := map[string]map[string]interface{}{}
	for _, change := range changes {
		field := stringValue(change, "field")
		if field != "" {
			changesByField[field] = change
		}
	}
	assertStringSliceContains(t, stringSliceFromAny(changesByField["database_bindings"]["added_resource_ids"]), "db-1:table-2")
	assertStringSliceContains(t, stringSliceFromAny(changesByField["database_bindings"]["removed_resource_ids"]), "db-1:table-1")
	assertStringSliceContains(t, stringSliceFromAny(changesByField["workflow_bindings"]["added_resource_ids"]), "workflow-2")
	assertStringSliceContains(t, stringSliceFromAny(changesByField["workflow_bindings"]["removed_resource_ids"]), "workflow-1")
	assertStringSliceContains(t, stringSliceFromAny(changesByField["enabled_skill_ids"]["added_resource_names"]), "图表生成器")
	assertStringSliceContains(t, stringSliceFromAny(changesByField["knowledge_dataset_ids"]["added_resource_names"]), "新知识库")
	assertStringSliceContains(t, stringSliceFromAny(changesByField["knowledge_dataset_ids"]["removed_resource_names"]), "旧知识库")
	assertStringSliceContains(t, stringSliceFromAny(changesByField["database_bindings"]["added_resource_names"]), "新订单表")
	assertStringSliceContains(t, stringSliceFromAny(changesByField["database_bindings"]["removed_resource_names"]), "旧订单表")
	assertStringSliceContains(t, stringSliceFromAny(changesByField["workflow_bindings"]["added_resource_names"]), "退款流程")
	assertStringSliceContains(t, stringSliceFromAny(changesByField["workflow_bindings"]["removed_resource_names"]), "审批流程")
}

func TestUpdateAgentConfigAutoFillsSkillDisplayNames(t *testing.T) {
	agentID := "agent-1"
	current := &dto.AgentDraftRuntimeConfigResponse{
		AgentID:     agentID,
		WorkspaceID: "agent-workspace",
		Config: dto.AgentConfigResponse{
			AgentID:         agentID,
			ModelProvider:   "openai",
			Model:           "gpt-4o",
			EnabledSkillIDs: []string{},
		},
	}
	service := &fakeAgentManagementService{
		draftConfig: current,
		agents: map[string]interface{}{
			agentID: map[string]interface{}{"id": agentID, "name": "Support Agent", "workspace_id": "agent-workspace"},
		},
		skillCandidatesResp: &dto.AgentSkillCandidatesResponse{
			AgentID:     agentID,
			WorkspaceID: "agent-workspace",
			Count:       1,
			Data: []dto.AgentSkillCandidate{{
				SkillID: "chart-generator",
				Name:    "Chart Generator",
			}},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
		},
	})

	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id":          agentID,
		"enabled_skill_ids": `["chart-generator"]`,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if service.lastSkillCandidatesRequest.Limit != maxAgentBindingCandidateListPageSize || !service.lastSkillCandidatesRequest.IncludeSelected {
		t.Fatalf("skill candidates request = %#v, want broad selected lookup", service.lastSkillCandidatesRequest)
	}
	payload := messages[0].Data
	if payload["binding_kind"] != "agent_skill" || payload["change_action"] != "bind" {
		t.Fatalf("binding summary = kind:%#v action:%#v, want agent_skill bind", payload["binding_kind"], payload["change_action"])
	}
	assertStringSliceContains(t, stringSliceFromAny(payload["added_resource_names"]), "Chart Generator")
	assertStringSliceContains(t, stringSliceFromAny(payload["resource_names"]), "Chart Generator")
	changes, ok := payload["binding_changes"].([]map[string]interface{})
	if !ok || len(changes) != 1 {
		t.Fatalf("binding_changes = %#v, want one skill change", payload["binding_changes"])
	}
	assertStringSliceContains(t, stringSliceFromAny(changes[0]["added_resource_names"]), "Chart Generator")
}

func TestUpdateAgentConfigAutoFillsBindingDisplayNames(t *testing.T) {
	agentID := "agent-1"
	current := &dto.AgentDraftRuntimeConfigResponse{
		AgentID:     agentID,
		WorkspaceID: "agent-workspace",
		Config: dto.AgentConfigResponse{
			AgentID:             agentID,
			ModelProvider:       "openai",
			Model:               "gpt-4o",
			KnowledgeDatasetIDs: []string{},
			DatabaseBindings:    []dto.AgentDatabaseBinding{},
			WorkflowBindings:    []dto.AgentWorkflowBinding{},
		},
	}
	service := &fakeAgentManagementService{
		draftConfig: current,
		agents: map[string]interface{}{
			agentID: map[string]interface{}{"id": agentID, "name": "Support Agent", "workspace_id": "agent-workspace"},
		},
		knowledgeCandidatesResp: &dto.AgentKnowledgeCandidatesResponse{
			Data: []dto.AgentKnowledgeCandidate{{
				DatasetID: "kb-1",
				Name:      "Product KB",
			}},
		},
		databaseTablesRespByDataSource: map[string]*dto.AgentDatabaseTablesResponse{
			"db-1": {
				DataSourceID: "db-1",
				Data: []dto.AgentDatabaseTableCandidate{{
					DataSourceID: "db-1",
					TableID:      "table-1",
					Name:         "Orders",
				}},
			},
		},
		workflowCandidatesResp: &dto.AgentWorkflowBindingCandidatesResponse{
			Data: []dto.AgentWorkflowBindingCandidate{{
				BindingID:  "workflow-binding-1",
				WorkflowID: "wf-1",
				AgentID:    "workflow-agent-1",
				Label:      "Approval Flow",
			}},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
		},
	})

	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id":              agentID,
		"knowledge_dataset_ids": `["kb-1"]`,
		"database_bindings":     `[{"data_source_id":"db-1","table_ids":["table-1"]}]`,
		"workflow_bindings":     `[{"binding_id":"workflow-binding-1","agent_id":"workflow-agent-1","workflow_id":"wf-1","version_strategy":"latest_published"}]`,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	payload := messages[0].Data
	changes, ok := payload["binding_changes"].([]map[string]interface{})
	if !ok || len(changes) != 3 {
		t.Fatalf("binding_changes = %#v, want three named changes", payload["binding_changes"])
	}
	changesByField := map[string]map[string]interface{}{}
	for _, change := range changes {
		field := stringValue(change, "field")
		if field != "" {
			changesByField[field] = change
		}
	}
	assertStringSliceContains(t, stringSliceFromAny(changesByField["database_bindings"]["added_resource_ids"]), "db-1:table-1")
	assertStringSliceContains(t, stringSliceFromAny(changesByField["workflow_bindings"]["added_resource_ids"]), "workflow-binding-1")
	assertStringSliceContains(t, stringSliceFromAny(changesByField["knowledge_dataset_ids"]["added_resource_names"]), "Product KB")
	assertStringSliceContains(t, stringSliceFromAny(changesByField["database_bindings"]["added_resource_names"]), "Orders")
	assertStringSliceContains(t, stringSliceFromAny(changesByField["workflow_bindings"]["added_resource_names"]), "Approval Flow")
}

func TestUpdateAgentConfigReplacesSyntheticDatabaseDisplayName(t *testing.T) {
	agentID := "agent-1"
	current := &dto.AgentDraftRuntimeConfigResponse{
		AgentID:     agentID,
		WorkspaceID: "agent-workspace",
		Config: dto.AgentConfigResponse{
			AgentID:          agentID,
			ModelProvider:    "openai",
			Model:            "gpt-4o",
			DatabaseBindings: []dto.AgentDatabaseBinding{{DataSourceID: "db-1", TableIDs: []string{"table-1"}}},
		},
	}
	service := &fakeAgentManagementService{
		draftConfig: current,
		agents: map[string]interface{}{
			agentID: map[string]interface{}{"id": agentID, "name": "Support Agent", "workspace_id": "agent-workspace"},
		},
		databaseTablesRespByDataSource: map[string]*dto.AgentDatabaseTablesResponse{
			"db-1": {
				DataSourceID: "db-1",
				Data: []dto.AgentDatabaseTableCandidate{{
					DataSourceID: "db-1",
					TableID:      "table-1",
					Name:         "Orders",
				}},
			},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
		},
	})

	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id":          agentID,
		"database_bindings": `[]`,
		"display_names": map[string]interface{}{
			"database_tables": map[string]string{"db-1:table-1": "Database binding 1"},
		},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	payload := messages[0].Data
	changes, ok := payload["binding_changes"].([]map[string]interface{})
	if !ok || len(changes) != 1 {
		t.Fatalf("binding_changes = %#v, want one database unbind change", payload["binding_changes"])
	}
	if changes[0]["change_action"] != "unbind" {
		t.Fatalf("binding change action = %#v, want unbind; change=%#v", changes[0]["change_action"], changes[0])
	}
	removedNames := stringSliceFromAny(changes[0]["removed_resource_names"])
	assertStringSliceContains(t, removedNames, "Orders")
	for _, name := range removedNames {
		if name == "Database binding 1" {
			t.Fatalf("removed_resource_names = %#v, should not keep synthetic display name", removedNames)
		}
	}
}

func TestUpdateAgentConfigCompletesWorkflowBindingFromCandidateID(t *testing.T) {
	agentID := "agent-1"
	current := &dto.AgentDraftRuntimeConfigResponse{
		AgentID:     agentID,
		WorkspaceID: "agent-workspace",
		Config: dto.AgentConfigResponse{
			AgentID:          agentID,
			ModelProvider:    "openai",
			Model:            "gpt-4o",
			WorkflowBindings: []dto.AgentWorkflowBinding{},
		},
	}
	service := &fakeAgentManagementService{
		draftConfig: current,
		agents: map[string]interface{}{
			agentID: map[string]interface{}{"id": agentID, "name": "Support Agent", "workspace_id": "agent-workspace"},
		},
		workflowCandidatesResp: &dto.AgentWorkflowBindingCandidatesResponse{
			Data: []dto.AgentWorkflowBindingCandidate{{
				BindingID:       "workflow-binding-1",
				WorkflowID:      "wf-1",
				AgentID:         "workflow-agent-1",
				Label:           "Approval Flow",
				VersionStrategy: "latest_published",
			}},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
		},
	})

	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id":              agentID,
		"add_workflow_bindings": `[{"binding_id":"workflow-binding-1"}]`,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(service.lastConfigRequest.WorkflowBindings) != 1 {
		t.Fatalf("WorkflowBindings = %#v, want one completed binding", service.lastConfigRequest.WorkflowBindings)
	}
	got := service.lastConfigRequest.WorkflowBindings[0]
	if got.BindingID != "workflow-binding-1" || got.AgentID != "workflow-agent-1" || got.WorkflowID != "wf-1" || got.Label != "Approval Flow" {
		t.Fatalf("completed workflow binding = %#v, want candidate fields", got)
	}
	payload := messages[0].Data
	assertStringSliceContains(t, stringSliceFromAny(payload["updated_fields"]), "workflow_bindings")
	assertStringSliceContains(t, stringSliceFromAny(payload["added_resource_names"]), "Approval Flow")
}

func TestUpdateAgentConfigUpdatedFieldsUsePersistedResponse(t *testing.T) {
	agentID := "agent-1"
	currentConfig := dto.AgentConfigResponse{
		AgentID:          agentID,
		ModelProvider:    "openai",
		Model:            "gpt-4o",
		WorkflowBindings: []dto.AgentWorkflowBinding{},
	}
	service := &fakeAgentManagementService{
		draftConfig: &dto.AgentDraftRuntimeConfigResponse{
			AgentID:     agentID,
			WorkspaceID: "agent-workspace",
			Config:      currentConfig,
		},
		updateConfigResponse: &currentConfig,
		agents: map[string]interface{}{
			agentID: map[string]interface{}{"id": agentID, "name": "Support Agent", "workspace_id": "agent-workspace"},
		},
		workflowCandidatesResp: &dto.AgentWorkflowBindingCandidatesResponse{
			Data: []dto.AgentWorkflowBindingCandidate{{
				BindingID:       "workflow-binding-1",
				WorkflowID:      "wf-1",
				AgentID:         "workflow-agent-1",
				Label:           "Approval Flow",
				VersionStrategy: "latest_published",
			}},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
		},
	})

	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id":              agentID,
		"add_workflow_bindings": `[{"binding_id":"workflow-binding-1"}]`,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(service.lastConfigRequest.WorkflowBindings) != 1 {
		t.Fatalf("attempted WorkflowBindings = %#v, want one completed binding", service.lastConfigRequest.WorkflowBindings)
	}
	payload := messages[0].Data
	if fields := stringSliceFromAny(payload["updated_fields"]); len(fields) != 0 {
		t.Fatalf("updated_fields = %#v, want no persisted changes", fields)
	}
	if changes := payload["binding_changes"]; changes != nil {
		t.Fatalf("binding_changes = %#v, want omitted when persisted response is unchanged", changes)
	}
}

func TestUpdateAgentConfigIncrementalUnbindPreservesUnmentionedBindings(t *testing.T) {
	agentID := "agent-1"
	current := &dto.AgentDraftRuntimeConfigResponse{
		AgentID:     agentID,
		WorkspaceID: "agent-workspace",
		Config: dto.AgentConfigResponse{
			AgentID:             agentID,
			ModelProvider:       "openai",
			Model:               "gpt-4o",
			EnabledSkillIDs:     []string{"chart-generator", "architecture-diagram-generator"},
			KnowledgeDatasetIDs: []string{"kb-keep", "kb-remove"},
			DatabaseBindings: []dto.AgentDatabaseBinding{
				{DataSourceID: "db-1", TableIDs: []string{"table-keep", "table-remove"}, WritableTableIDs: []string{"table-remove"}},
				{DataSourceID: "db-2", TableIDs: []string{"table-other"}},
			},
			WorkflowBindings: []dto.AgentWorkflowBinding{
				{BindingID: "workflow-keep", Label: "Keep Workflow", AgentID: "workflow-agent-keep", WorkflowID: "wf-keep", VersionStrategy: "latest_published"},
				{BindingID: "workflow-remove", Label: "Remove Workflow", AgentID: "workflow-agent-remove", WorkflowID: "wf-remove", VersionStrategy: "latest_published"},
			},
		},
	}
	service := &fakeAgentManagementService{
		draftConfig: current,
		agents: map[string]interface{}{
			agentID: map[string]interface{}{"id": agentID, "name": "Support Agent", "workspace_id": "agent-workspace"},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
		},
	})

	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id":                     agentID,
		"remove_enabled_skill_ids":     `["architecture-diagram-generator"]`,
		"remove_knowledge_dataset_ids": `["kb-remove"]`,
		"remove_database_bindings":     `[{"data_source_id":"db-1","table_ids":["table-remove"]}]`,
		"remove_workflow_bindings":     `[{"binding_id":"workflow-remove","agent_id":"workflow-agent-remove","workflow_id":"wf-remove"}]`,
		"display_names": map[string]interface{}{
			"skills": map[string]string{
				"chart-generator":                "Chart Generator",
				"architecture-diagram-generator": "Architecture Diagram Generator",
			},
			"knowledge_bases": map[string]string{
				"kb-keep":   "Keep KB",
				"kb-remove": "Remove KB",
			},
			"database_tables": map[string]string{
				"db-1:table-keep":   "Keep Table",
				"db-1:table-remove": "Remove Table",
				"db-2:table-other":  "Other Table",
			},
			"workflows": map[string]string{
				"workflow-keep":   "Keep Workflow",
				"workflow-remove": "Remove Workflow",
			},
		},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}

	if got := service.lastConfigRequest.EnabledSkillIDs; !reflect.DeepEqual(got, []string{"chart-generator"}) {
		t.Fatalf("EnabledSkillIDs = %#v, want only chart-generator", got)
	}
	if got := service.lastConfigRequest.KnowledgeDatasetIDs; !reflect.DeepEqual(got, []string{"kb-keep"}) {
		t.Fatalf("KnowledgeDatasetIDs = %#v, want only kb-keep", got)
	}
	if len(service.lastConfigRequest.DatabaseBindings) != 2 {
		t.Fatalf("DatabaseBindings = %#v, want two remaining data sources", service.lastConfigRequest.DatabaseBindings)
	}
	db1 := service.lastConfigRequest.DatabaseBindings[0]
	if db1.DataSourceID != "db-1" || !reflect.DeepEqual(db1.TableIDs, []string{"table-keep"}) || len(db1.WritableTableIDs) != 0 {
		t.Fatalf("db-1 binding = %#v, want only table-keep and no writable removed table", db1)
	}
	if service.lastConfigRequest.DatabaseBindings[1].DataSourceID != "db-2" {
		t.Fatalf("second database binding = %#v, want db-2 preserved", service.lastConfigRequest.DatabaseBindings[1])
	}
	if len(service.lastConfigRequest.WorkflowBindings) != 1 || service.lastConfigRequest.WorkflowBindings[0].BindingID != "workflow-keep" {
		t.Fatalf("WorkflowBindings = %#v, want only workflow-keep", service.lastConfigRequest.WorkflowBindings)
	}

	payload := messages[0].Data
	if payload["agent_name"] != "Support Agent" {
		t.Fatalf("agent_name = %#v, want Support Agent; payload=%#v", payload["agent_name"], payload)
	}
	if payload["binding_kind"] != "multiple" || payload["change_action"] != "unbind" {
		t.Fatalf("binding summary = kind:%#v action:%#v, want multiple unbind; payload=%#v", payload["binding_kind"], payload["change_action"], payload)
	}
	assertStringSliceContains(t, stringSliceFromAny(payload["resource_ids"]), "architecture-diagram-generator")
	assertStringSliceContains(t, stringSliceFromAny(payload["resource_ids"]), "kb-remove")
	assertStringSliceContains(t, stringSliceFromAny(payload["resource_ids"]), "db-1:table-remove")
	assertStringSliceContains(t, stringSliceFromAny(payload["resource_ids"]), "workflow-remove")
	assertStringSliceContains(t, stringSliceFromAny(payload["removed_resource_ids"]), "db-1:table-remove")
	assertStringSliceContains(t, stringSliceFromAny(payload["removed_resource_names"]), "Remove Table")
	changes, ok := payload["binding_changes"].([]map[string]interface{})
	if !ok || len(changes) != 4 {
		t.Fatalf("binding_changes = %#v, want four unbind changes", payload["binding_changes"])
	}
	changesByField := map[string]map[string]interface{}{}
	for _, change := range changes {
		field := stringValue(change, "field")
		if field != "" {
			changesByField[field] = change
		}
		if change["change_action"] != "unbind" {
			t.Fatalf("change %#v action = %#v, want unbind", change, change["change_action"])
		}
	}
	assertStringSliceContains(t, stringSliceFromAny(changesByField["enabled_skill_ids"]["removed_resource_names"]), "Architecture Diagram Generator")
	assertStringSliceContains(t, stringSliceFromAny(changesByField["knowledge_dataset_ids"]["removed_resource_names"]), "Remove KB")
	assertStringSliceContains(t, stringSliceFromAny(changesByField["database_bindings"]["removed_resource_names"]), "Remove Table")
	assertStringSliceContains(t, stringSliceFromAny(changesByField["workflow_bindings"]["removed_resource_names"]), "Remove Workflow")
	assertStringSliceContains(t, stringSliceFromAny(changesByField["database_bindings"]["final_resource_names"]), "Keep Table")
}

func TestUpdateAgentIdentityAppliesIconBackground(t *testing.T) {
	agentID := "agent-1"
	service := &fakeAgentManagementService{
		updateAgentResponse: map[string]interface{}{
			"id":           agentID,
			"name":         "Post Verify Agent",
			"description":  "updated",
			"workspace_id": "agent-workspace",
			"icon_type":    "text",
		},
	}
	tool := newUpdateAgentIdentityTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
		},
	})
	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id":        agentID,
		"description":     "updated",
		"icon_type":       "text",
		"icon":            "P3",
		"icon_background": "#0f766e",
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if service.updateAgentCalls != 1 || service.lastUpdateAgentID != agentID {
		t.Fatalf("UpdateAgent calls = %d id=%q, want one call for %q", service.updateAgentCalls, service.lastUpdateAgentID, agentID)
	}
	updateIcon, ok := service.lastUpdateRequest["icon"].(string)
	if !ok {
		t.Fatalf("update icon = %#v, want string", service.lastUpdateRequest["icon"])
	}
	var iconPayload map[string]interface{}
	if err := json.Unmarshal([]byte(updateIcon), &iconPayload); err != nil {
		t.Fatalf("update icon is not JSON: %v", err)
	}
	if iconPayload["icon"] != "P3" || iconPayload["icon_background"] != "#0f766e" {
		t.Fatalf("icon payload = %#v, want requested icon and background", iconPayload)
	}
	fields, ok := messages[0].Data["updated_fields"].([]string)
	if !ok {
		t.Fatalf("updated_fields = %#v, want []string", messages[0].Data["updated_fields"])
	}
	wantFields := []string{"description", "icon_type", "icon", "icon_background"}
	if !reflect.DeepEqual(fields, wantFields) {
		t.Fatalf("updated_fields = %#v, want %#v", fields, wantFields)
	}
	payload := messages[0].Data
	if payload["agent_description"] != "updated" || payload["agent_icon_type"] != "text" || payload["agent_icon"] == "" {
		t.Fatalf("payload identity evidence = %#v, want description and icon fields", payload)
	}
}

func TestDeleteAgentsToolReturnsBatchItemResults(t *testing.T) {
	service := &fakeAgentManagementService{
		agents: map[string]interface{}{
			"agent-1": map[string]interface{}{"id": "agent-1", "name": "Agent One", "workspace_id": "workspace-1"},
			"agent-2": map[string]interface{}{"id": "agent-2", "name": "Agent Two", "workspace_id": "workspace-1"},
		},
	}
	tool := newDeleteAgentsTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "workspace-1",
		},
	})

	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agents": []interface{}{
			map[string]interface{}{"agent_id": "agent-1", "name": "Agent One"},
			map[string]interface{}{"agent_id": "agent-2", "name": "Agent Two"},
		},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if !reflect.DeepEqual(service.deletedAgentIDs, []string{"agent-1", "agent-2"}) {
		t.Fatalf("deletedAgentIDs = %#v, want agent-1/agent-2", service.deletedAgentIDs)
	}
	data := messages[0].Data
	if data["status"] != "completed" || data["deleted_count"] != 2 || data["failed_count"] != 0 {
		t.Fatalf("batch payload = %#v, want completed 2/0", data)
	}
	if _, exists := data["route_after_delete"]; exists {
		t.Fatalf("route_after_delete = %#v, want omitted for batch list deletion", data["route_after_delete"])
	}
	items, ok := data["item_results"].([]map[string]interface{})
	if !ok || len(items) != 2 {
		t.Fatalf("item_results = %#v, want two item maps", data["item_results"])
	}
	if items[0]["status"] != "succeeded" || items[1]["status"] != "succeeded" {
		t.Fatalf("item_results = %#v, want succeeded statuses", items)
	}
	group := mapFromAny(data["operation_group"])
	if group["target_count"] != 2 || group["success_count"] != 2 {
		t.Fatalf("operation_group = %#v, want target/success counts", group)
	}
}

func TestDeleteAgentToolPreservesResolvedNameWhenAgentReadIsMissing(t *testing.T) {
	service := &fakeAgentManagementService{}
	tool := newDeleteAgentTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "workspace-1",
		},
	})

	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id":   "agent-1",
		"agent_name": "Agent One",
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if !reflect.DeepEqual(service.deletedAgentIDs, []string{"agent-1"}) {
		t.Fatalf("deletedAgentIDs = %#v, want agent-1", service.deletedAgentIDs)
	}
	data := messages[0].Data
	if data["agent_name"] != "Agent One" {
		t.Fatalf("agent_name = %#v, want Agent One; payload=%#v", data["agent_name"], data)
	}
	agent := mapFromAny(data["agent"])
	if agent["name"] != "Agent One" {
		t.Fatalf("agent payload = %#v, want fallback name", agent)
	}
}

func TestDeleteAgentsToolRecordsPartialFailure(t *testing.T) {
	service := &fakeAgentManagementService{
		agents: map[string]interface{}{
			"agent-1": map[string]interface{}{"id": "agent-1", "name": "Agent One"},
			"agent-2": map[string]interface{}{"id": "agent-2", "name": "Agent Two"},
		},
		deleteAgentErrByID: map[string]error{"agent-2": errors.New("agent is locked")},
	}
	tool := newDeleteAgentsTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"console_agents_visible_agents": []interface{}{
				map[string]interface{}{"agent_id": "agent-1", "name": "Agent One"},
				map[string]interface{}{"agent_id": "agent-2", "name": "Agent Two"},
			},
		},
	})

	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_ids": []interface{}{"agent-1", "agent-2"},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	data := messages[0].Data
	if data["status"] != "partial_failed" || data["deleted_count"] != 1 || data["failed_count"] != 1 {
		t.Fatalf("batch payload = %#v, want partial_failed 1/1", data)
	}
	items, ok := data["item_results"].([]map[string]interface{})
	if !ok || len(items) != 2 {
		t.Fatalf("item_results = %#v, want two item maps", data["item_results"])
	}
	if items[0]["status"] != "succeeded" || items[1]["status"] != "failed" || items[1]["error"] != "agent is locked" {
		t.Fatalf("item_results = %#v, want one success and one failed item with error", items)
	}
}

func TestUpdateAgentConfigChangesModelWithProvider(t *testing.T) {
	agentID := "agent-1"
	service := &fakeAgentManagementService{
		draftConfig: &dto.AgentDraftRuntimeConfigResponse{
			AgentID:     agentID,
			WorkspaceID: "agent-workspace",
			Config: dto.AgentConfigResponse{
				AgentID:       agentID,
				ModelProvider: "deepseek",
				Model:         "deepseek-chat",
			},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
		},
	})
	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id":             agentID,
		"model_provider":       "openai",
		"model":                "gpt-4o",
		"system_prompt":        "keep answering",
		"agent_memory_enabled": true,
		"file_upload_enabled":  true,
		"home_title":           "Agent Home",
		"input_placeholder":    "Ask the agent",
		"theme_color":          "blue",
		"suggested_questions":  `["hello","status"]`,
		"model_parameters":     `{"temperature":0.2}`,
		"enabled_skill_ids":    `["chart-generator"]`,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if service.updateConfigCalls != 1 {
		t.Fatalf("UpdateAgentConfig calls = %d, want 1", service.updateConfigCalls)
	}
	if service.lastConfigRequest.ModelProvider != "openai" || service.lastConfigRequest.Model != "gpt-4o" {
		t.Fatalf("model request = %s/%s, want openai/gpt-4o", service.lastConfigRequest.ModelProvider, service.lastConfigRequest.Model)
	}
	wantFields := []string{
		"system_prompt",
		"model_provider",
		"model",
		"model_parameters",
		"enabled_skill_ids",
		"agent_memory_enabled",
		"file_upload_enabled",
		"home_title",
		"input_placeholder",
		"theme_color",
		"suggested_questions",
	}
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	gotFields, ok := messages[0].Data["updated_fields"].([]string)
	if !ok {
		t.Fatalf("updated_fields = %#v, want []string", messages[0].Data["updated_fields"])
	}
	if !reflect.DeepEqual(gotFields, wantFields) {
		t.Fatalf("updated_fields = %#v, want %#v", gotFields, wantFields)
	}
	if messages[0].Data["agent_id"] != agentID || messages[0].Data["workspace_id"] != "agent-workspace" {
		t.Fatalf("payload identity = %#v, want agent/workspace evidence", messages[0].Data)
	}
	if messages[0].Data["home_title"] != "Agent Home" || messages[0].Data["input_placeholder"] != "Ask the agent" || messages[0].Data["theme_color"] != "blue" {
		t.Fatalf("payload visible config = %#v, want updated home/input/theme evidence", messages[0].Data)
	}
	questions, ok := messages[0].Data["suggested_questions"].([]string)
	if !ok || !reflect.DeepEqual(questions, []string{"hello", "status"}) {
		t.Fatalf("suggested_questions = %#v, want updated question text evidence", messages[0].Data["suggested_questions"])
	}
	if messages[0].Data["suggested_question_count"] != 2 {
		t.Fatalf("suggested_question_count = %#v, want 2", messages[0].Data["suggested_question_count"])
	}
}

func TestUpdateAgentConfigReportsSatisfiedFieldsForAlreadyCurrentValues(t *testing.T) {
	agentID := "agent-1"
	service := &fakeAgentManagementService{
		draftConfig: &dto.AgentDraftRuntimeConfigResponse{
			AgentID:     agentID,
			WorkspaceID: "agent-workspace",
			Config: dto.AgentConfigResponse{
				AgentID:    agentID,
				ThemeColor: "emerald",
			},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
		},
	})

	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id":    agentID,
		"theme_color": "emerald",
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	payload := messages[0].Data
	if fields := stringSliceFromAny(payload["updated_fields"]); len(fields) != 0 {
		t.Fatalf("updated_fields = %#v, want no actual changed fields", fields)
	}
	if fields := stringSliceFromAny(payload["requested_fields"]); !reflect.DeepEqual(fields, []string{"theme_color"}) {
		t.Fatalf("requested_fields = %#v, want [theme_color]", fields)
	}
	if fields := stringSliceFromAny(payload["satisfied_fields"]); !reflect.DeepEqual(fields, []string{"theme_color"}) {
		t.Fatalf("satisfied_fields = %#v, want [theme_color]", fields)
	}
	if payload["theme_color"] != "emerald" {
		t.Fatalf("theme_color = %#v, want emerald; payload=%#v", payload["theme_color"], payload)
	}
}

func TestUpdateAgentConfigReportsBindingFinalStateForSatisfiedNoop(t *testing.T) {
	agentID := "agent-1"
	service := &fakeAgentManagementService{
		draftConfig: &dto.AgentDraftRuntimeConfigResponse{
			AgentID:     agentID,
			WorkspaceID: "agent-workspace",
			Config: dto.AgentConfigResponse{
				AgentID:         agentID,
				EnabledSkillIDs: nil,
			},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
		},
	})

	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id":                 agentID,
		"remove_enabled_skill_ids": []interface{}{},
		"display_names": map[string]interface{}{
			"skills": map[string]interface{}{
				"chart-generator": "图表生成器",
			},
		},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	payload := messages[0].Data
	if fields := stringSliceFromAny(payload["updated_fields"]); len(fields) != 0 {
		t.Fatalf("updated_fields = %#v, want no actual changed fields", fields)
	}
	if fields := stringSliceFromAny(payload["satisfied_fields"]); !reflect.DeepEqual(fields, []string{"enabled_skill_ids"}) {
		t.Fatalf("satisfied_fields = %#v, want [enabled_skill_ids]", fields)
	}
	states, ok := payload["binding_final_states"].([]map[string]interface{})
	if !ok || len(states) != 1 {
		t.Fatalf("binding_final_states = %#v, want one final state", payload["binding_final_states"])
	}
	if states[0]["binding_kind"] != "agent_skill" || states[0]["final_resource_count"] != 0 {
		t.Fatalf("binding_final_states[0] = %#v, want empty agent_skill final state", states[0])
	}
	if payload["binding_kind"] != "agent_skill" || payload["final_resource_count"] != 0 {
		t.Fatalf("top-level binding state = kind %#v count %#v, want agent_skill/0; payload=%#v", payload["binding_kind"], payload["final_resource_count"], payload)
	}
	if payload["change_action"] != "satisfied" {
		t.Fatalf("top-level binding action = %#v, want satisfied; payload=%#v", payload["change_action"], payload)
	}
}

func TestUpdateAgentConfigReportsResourceBindingFinalStatesForSatisfiedNoop(t *testing.T) {
	agentID := "agent-1"
	service := &fakeAgentManagementService{
		draftConfig: &dto.AgentDraftRuntimeConfigResponse{
			AgentID:     agentID,
			WorkspaceID: "agent-workspace",
			Config: dto.AgentConfigResponse{
				AgentID:             agentID,
				KnowledgeDatasetIDs: []string{"kb-1"},
				DatabaseBindings: []dto.AgentDatabaseBinding{{
					DataSourceID: "db-1",
					TableIDs:     []string{"table-1"},
				}},
				WorkflowBindings: []dto.AgentWorkflowBinding{{
					BindingID:       "workflow-binding-1",
					AgentID:         "workflow-agent-1",
					WorkflowID:      "wf-1",
					VersionStrategy: "latest_published",
				}},
			},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
		},
	})

	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id":              agentID,
		"knowledge_dataset_ids": `["kb-1"]`,
		"database_bindings":     `[{"data_source_id":"db-1","table_ids":["table-1"]}]`,
		"workflow_bindings":     `[{"binding_id":"workflow-binding-1","agent_id":"workflow-agent-1","workflow_id":"wf-1","version_strategy":"latest_published"}]`,
		"display_names": map[string]interface{}{
			"knowledge_bases": map[string]interface{}{"kb-1": "Product KB"},
			"database_tables": map[string]interface{}{"db-1:table-1": "Orders"},
			"workflows":       map[string]interface{}{"workflow-binding-1": "Approval Flow"},
		},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	payload := messages[0].Data
	if fields := stringSliceFromAny(payload["updated_fields"]); len(fields) != 0 {
		t.Fatalf("updated_fields = %#v, want no actual changed fields", fields)
	}
	if fields := stringSliceFromAny(payload["satisfied_fields"]); !reflect.DeepEqual(fields, []string{"knowledge_dataset_ids", "database_bindings", "workflow_bindings"}) {
		t.Fatalf("satisfied_fields = %#v, want knowledge/database/workflow fields", fields)
	}
	states, ok := payload["binding_final_states"].([]map[string]interface{})
	if !ok || len(states) != 3 {
		t.Fatalf("binding_final_states = %#v, want three final states", payload["binding_final_states"])
	}
	statesByField := map[string]map[string]interface{}{}
	for _, state := range states {
		field := stringValue(state, "field")
		if field != "" {
			statesByField[field] = state
		}
	}
	expected := map[string]struct {
		kind string
		id   string
		name string
	}{
		"knowledge_dataset_ids": {kind: "knowledge_base", id: "kb-1", name: "Product KB"},
		"database_bindings":     {kind: "database_table", id: "db-1:table-1", name: "Orders"},
		"workflow_bindings":     {kind: "workflow", id: "workflow-binding-1", name: "Approval Flow"},
	}
	for field, want := range expected {
		state := statesByField[field]
		if len(state) == 0 {
			t.Fatalf("binding_final_states missing %s; states=%#v", field, states)
		}
		if state["binding_kind"] != want.kind || state["change_action"] != "satisfied" || state["final_resource_count"] != 1 {
			t.Fatalf("binding_final_states[%s] = %#v, want kind %s satisfied count 1", field, state, want.kind)
		}
		assertStringSliceContains(t, stringSliceFromAny(state["final_resource_ids"]), want.id)
		assertStringSliceContains(t, stringSliceFromAny(state["final_resource_names"]), want.name)
	}
}

func TestUpdateAgentConfigRejectsUnsupportedThemeColor(t *testing.T) {
	agentID := "agent-1"
	service := &fakeAgentManagementService{
		draftConfig: &dto.AgentDraftRuntimeConfigResponse{
			AgentID:     agentID,
			WorkspaceID: "agent-workspace",
			Config: dto.AgentConfigResponse{
				AgentID:    agentID,
				ThemeColor: "default",
			},
		},
	}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
		},
	})
	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id":    agentID,
		"theme_color": "teal",
	}, nil, nil, nil)
	if err == nil {
		t.Fatalf("Invoke() error = nil, want unsupported theme_color error with messages %#v", messages)
	}
	if !strings.Contains(err.Error(), "theme_color must be one of") || !strings.Contains(err.Error(), "emerald") {
		t.Fatalf("Invoke() error = %q, want supported theme list", err.Error())
	}
	if service.updateConfigCalls != 0 {
		t.Fatalf("UpdateAgentConfig calls = %d, want 0", service.updateConfigCalls)
	}
}

func TestUpdateAgentConfigValidatesModelPairAgainstAvailableModels(t *testing.T) {
	agentID := "agent-1"
	organizationID := uuid.New()
	available := &fakeAvailableModelsService{
		models: []*llmmodelservice.AvailableModel{{
			ID:          uuid.New(),
			Provider:    "openai",
			Name:        "gpt-4o",
			DisplayName: "GPT-4o",
			UseCases:    []string{"text-chat"},
		}},
	}
	service := &fakeAgentManagementService{
		draftConfig: &dto.AgentDraftRuntimeConfigResponse{
			AgentID:     agentID,
			WorkspaceID: "agent-workspace",
			Config: dto.AgentConfigResponse{
				AgentID:       agentID,
				ModelProvider: "deepseek",
				Model:         "deepseek-chat",
			},
		},
	}
	tool := newUpdateAgentConfigTool(service, available).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   organizationID.String(),
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": organizationID.String(),
			"workspace_id":    "agent-workspace",
		},
	})
	_, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id":       agentID,
		"model_provider": "openai",
		"model":          "gpt-4o",
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if available.organizationID != organizationID || available.provider != "openai" || available.useCase != "text-chat" {
		t.Fatalf("available model lookup = org %s provider %q useCase %q, want %s/openai/text-chat", available.organizationID, available.provider, available.useCase, organizationID)
	}
	if service.updateConfigCalls != 1 {
		t.Fatalf("UpdateAgentConfig calls = %d, want 1", service.updateConfigCalls)
	}
	if service.lastConfigRequest.ModelProvider != "openai" || service.lastConfigRequest.Model != "gpt-4o" {
		t.Fatalf("model request = %s/%s, want openai/gpt-4o", service.lastConfigRequest.ModelProvider, service.lastConfigRequest.Model)
	}
}

func TestUpdateAgentConfigRejectsUnavailableModelPair(t *testing.T) {
	agentID := "agent-1"
	organizationID := uuid.New()
	available := &fakeAvailableModelsService{
		models: []*llmmodelservice.AvailableModel{{
			ID:          uuid.New(),
			Provider:    "openai",
			Name:        "gpt-4o",
			DisplayName: "GPT-4o",
			UseCases:    []string{"text-chat"},
		}},
	}
	service := &fakeAgentManagementService{
		draftConfig: &dto.AgentDraftRuntimeConfigResponse{
			AgentID:     agentID,
			WorkspaceID: "agent-workspace",
			Config: dto.AgentConfigResponse{
				AgentID:       agentID,
				ModelProvider: "deepseek",
				Model:         "deepseek-chat",
			},
		},
	}
	tool := newUpdateAgentConfigTool(service, available).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   organizationID.String(),
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": organizationID.String(),
			"workspace_id":    "agent-workspace",
		},
	})
	_, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id":       agentID,
		"model_provider": "openai",
		"model":          "gpt-5-wrong-provider",
	}, nil, nil, nil)
	if err == nil {
		t.Fatal("Invoke() error = nil, want unavailable model pair error")
	}
	if !strings.Contains(err.Error(), "is not available for provider") {
		t.Fatalf("Invoke() error = %q, want unavailable model pair error", err.Error())
	}
	if available.organizationID != organizationID || available.provider != "openai" || available.useCase != "text-chat" {
		t.Fatalf("available model lookup = org %s provider %q useCase %q, want %s/openai/text-chat", available.organizationID, available.provider, available.useCase, organizationID)
	}
	if service.updateConfigCalls != 0 {
		t.Fatalf("UpdateAgentConfig calls = %d, want 0", service.updateConfigCalls)
	}
}

func TestListAgentSkillCandidatesTool(t *testing.T) {
	service := &fakeAgentManagementService{
		skillCandidatesResp: &dto.AgentSkillCandidatesResponse{
			AgentID:         "agent-1",
			WorkspaceID:     "agent-workspace",
			Query:           "chart",
			IncludeSelected: true,
			Count:           1,
			Data: []dto.AgentSkillCandidate{{
				SkillID:     "chart-generator",
				Name:        "Chart generator",
				Description: "Generate charts",
				Selected:    true,
			}},
		},
	}
	tool := newListAgentSkillCandidatesTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "agent-workspace",
		},
	})
	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id":         "agent-1",
		"query":            "chart",
		"include_selected": true,
		"limit":            10,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if service.lastSkillCandidatesRequest.Query != "chart" || service.lastSkillCandidatesRequest.Limit != 10 {
		t.Fatalf("skill request = %#v, want query chart limit 10", service.lastSkillCandidatesRequest)
	}
	if len(messages) != 1 || messages[0].Data["count"] != 1 {
		t.Fatalf("messages = %#v, want one candidate payload", messages)
	}
}

func TestListAgentKnowledgeCandidatesUsesCurrentAgentRouteWhenAgentIDOmitted(t *testing.T) {
	service := &fakeAgentManagementService{
		knowledgeCandidatesResp: &dto.AgentKnowledgeCandidatesResponse{
			AgentID:     "agent-1",
			WorkspaceID: "workspace-1",
			Count:       1,
		},
	}
	tool := newListAgentKnowledgeCandidatesTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id":       "org-1",
			"workspace_id":          "workspace-1",
			"console_current_route": "/console/agents/agent-1/agent",
			"console_agents_page":   true,
			"console_agents_visible_agents": []map[string]interface{}{
				{"id": "agent-1", "name": "Support Agent", "workspace_id": "workspace-1"},
				{"id": "agent-2", "name": "Other Agent", "workspace_id": "workspace-1"},
			},
		},
	})
	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if service.lastKnowledgeCandidatesAgentID != "agent-1" {
		t.Fatalf("ListAgentKnowledgeCandidates agentID = %q, want current route agent-1", service.lastKnowledgeCandidatesAgentID)
	}
	if len(messages) != 1 || messages[0].Data["agent_id"] != "agent-1" {
		t.Fatalf("messages = %#v, want agent-1 candidate payload", messages)
	}
}

func TestGetAgentConfigUsesCurrentAgentRouteWhenAgentIDOmitted(t *testing.T) {
	service := &fakeAgentManagementService{
		draftConfig: &dto.AgentDraftRuntimeConfigResponse{
			AgentID:     "agent-1",
			WorkspaceID: "workspace-1",
			Config: dto.AgentConfigResponse{
				AgentID:       "agent-1",
				ModelProvider: "deepseek",
				Model:         "deepseek-chat",
			},
		},
	}
	tool := newGetAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id":       "org-1",
			"workspace_id":          "workspace-1",
			"console_current_route": "/console/agents/agent-1/agent",
			"console_agents_page":   true,
			"console_agents_visible_agents": []map[string]interface{}{
				{"id": "agent-1", "name": "Support Agent", "workspace_id": "workspace-1"},
				{"id": "agent-2", "name": "Other Agent", "workspace_id": "workspace-1"},
			},
		},
	})

	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if service.lastDraftConfigAgentID != "agent-1" {
		t.Fatalf("GetAgentDraftRuntimeConfig agentID = %q, want current route agent-1", service.lastDraftConfigAgentID)
	}
	if len(messages) != 1 || messages[0].Data["agent_id"] != "agent-1" {
		t.Fatalf("messages = %#v, want agent-1 config payload", messages)
	}
}

func TestListAgentKnowledgeCandidatesDoesNotGuessFromUnselectedVisibleList(t *testing.T) {
	service := &fakeAgentManagementService{}
	tool := newListAgentKnowledgeCandidatesTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id":     "org-1",
			"workspace_id":        "workspace-1",
			"console_agents_page": true,
			"console_agents_visible_agents": []map[string]interface{}{
				{"id": "agent-1", "name": "Support Agent", "workspace_id": "workspace-1"},
				{"id": "agent-2", "name": "Other Agent", "workspace_id": "workspace-1"},
			},
		},
	})
	_, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "agent_id is required") {
		t.Fatalf("Invoke() error = %v, want missing agent_id", err)
	}
	if service.lastKnowledgeCandidatesAgentID != "" {
		t.Fatalf("ListAgentKnowledgeCandidates agentID = %q, want no guessed agent", service.lastKnowledgeCandidatesAgentID)
	}
}

func TestReplaceAgentSkillBindingsPreservesOtherConfigFields(t *testing.T) {
	agentID := "agent-1"
	current := &dto.AgentDraftRuntimeConfigResponse{
		AgentID:     agentID,
		WorkspaceID: "agent-workspace",
		Config: dto.AgentConfigResponse{
			AgentID:             agentID,
			SystemPrompt:        "keep prompt",
			ModelProvider:       "openai",
			Model:               "gpt-4o",
			ModelParameters:     map[string]interface{}{"temperature": float64(0.2)},
			EnabledSkillIDs:     []string{"time"},
			AgentMemoryEnabled:  true,
			FileUpload:          true,
			KnowledgeDatasetIDs: []string{"dataset-old"},
			DatabaseBindings:    []dto.AgentDatabaseBinding{{DataSourceID: "db-1", TableIDs: []string{"table-1"}}},
			WorkflowBindings:    []dto.AgentWorkflowBinding{{BindingID: "workflow-1", AgentID: "workflow-1", WorkflowID: "wf-1", VersionStrategy: "latest_published"}},
		},
	}
	service := &fakeAgentManagementService{draftConfig: current}
	tool := newReplaceAgentSkillBindingsTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "runtime-workspace",
		},
	})
	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id":  agentID,
		"skill_ids": `["time","chart-generator"]`,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if service.getDraftConfigCalls != 1 || service.updateConfigCalls != 1 {
		t.Fatalf("service calls get=%d update=%d, want 1/1", service.getDraftConfigCalls, service.updateConfigCalls)
	}
	if !reflect.DeepEqual(service.lastConfigRequest.EnabledSkillIDs, []string{"time", "chart-generator"}) {
		t.Fatalf("EnabledSkillIDs = %#v, want time/chart-generator", service.lastConfigRequest.EnabledSkillIDs)
	}
	if service.lastConfigRequest.SystemPrompt != "keep prompt" || service.lastConfigRequest.ModelProvider != "openai" || service.lastConfigRequest.Model != "gpt-4o" {
		t.Fatalf("preserved config = %#v, want prompt/model fields preserved", service.lastConfigRequest)
	}
	if !reflect.DeepEqual(service.lastConfigRequest.KnowledgeDatasetIDs, current.Config.KnowledgeDatasetIDs) {
		t.Fatalf("KnowledgeDatasetIDs = %#v, want preserved %#v", service.lastConfigRequest.KnowledgeDatasetIDs, current.Config.KnowledgeDatasetIDs)
	}
	if len(messages) != 1 || messages[0].Data["workspace_id"] != "agent-workspace" {
		t.Fatalf("payload = %#v, want agent workspace", messages)
	}
}

func TestReplaceAgentDatabaseBindingsPropagatesInvalidResourceError(t *testing.T) {
	service := &fakeAgentManagementService{
		draftConfig: &dto.AgentDraftRuntimeConfigResponse{
			AgentID:     "agent-1",
			WorkspaceID: "agent-workspace",
			Config:      dto.AgentConfigResponse{AgentID: "agent-1"},
		},
		updateConfigErr: errors.New("database db-2 not found in agent workspace"),
	}
	tool := newReplaceAgentDatabaseBindingsTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
	})
	_, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id": "agent-1",
		"bindings": `[{"data_source_id":"db-2","table_ids":["table-1"]}]`,
	}, nil, nil, nil)
	if err == nil {
		t.Fatal("Invoke() error = nil, want invalid resource error")
	}
	if err.Error() != "database db-2 not found in agent workspace" {
		t.Fatalf("Invoke() error = %v, want invalid resource error", err)
	}
	if service.updateConfigCalls != 1 {
		t.Fatalf("updateConfigCalls = %d, want 1", service.updateConfigCalls)
	}
}

func TestReplaceAgentDatabaseBindingsReturnsBindingEvidence(t *testing.T) {
	agentID := "agent-1"
	current := &dto.AgentDraftRuntimeConfigResponse{
		AgentID:     agentID,
		WorkspaceID: "agent-workspace",
		Config: dto.AgentConfigResponse{
			AgentID:             agentID,
			SystemPrompt:        "keep prompt",
			ModelProvider:       "openai",
			Model:               "gpt-4o",
			KnowledgeDatasetIDs: []string{"dataset-1"},
			WorkflowBindings:    []dto.AgentWorkflowBinding{{BindingID: "workflow-1", AgentID: "workflow-1", WorkflowID: "wf-1", VersionStrategy: "latest_published"}},
		},
	}
	service := &fakeAgentManagementService{
		draftConfig: current,
		agents: map[string]interface{}{
			agentID: map[string]interface{}{"id": agentID, "name": "Support Agent", "workspace_id": "agent-workspace"},
		},
	}
	tool := newReplaceAgentDatabaseBindingsTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "runtime-workspace",
		},
	})

	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id": agentID,
		"bindings": `[{"data_source_id":"db-1","table_ids":["table-1","table-2"],"writable_table_ids":["table-2"]}]`,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	wantBindings := []dto.AgentDatabaseBinding{{
		DataSourceID:     "db-1",
		TableIDs:         []string{"table-1", "table-2"},
		WritableTableIDs: []string{"table-2"},
	}}
	if !reflect.DeepEqual(service.lastConfigRequest.DatabaseBindings, wantBindings) {
		t.Fatalf("DatabaseBindings = %#v, want %#v", service.lastConfigRequest.DatabaseBindings, wantBindings)
	}
	if !reflect.DeepEqual(service.lastConfigRequest.KnowledgeDatasetIDs, current.Config.KnowledgeDatasetIDs) ||
		!reflect.DeepEqual(service.lastConfigRequest.WorkflowBindings, current.Config.WorkflowBindings) {
		t.Fatalf("unrelated bindings not preserved: request=%#v current=%#v", service.lastConfigRequest, current.Config)
	}
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	payload := messages[0].Data
	if payload["status"] != "completed" || payload["effect"] != "updated" || payload["agent_id"] != agentID || payload["workspace_id"] != "agent-workspace" {
		t.Fatalf("payload identity = %#v, want completed updated agent-workspace evidence", payload)
	}
	if payload["agent_name"] != "Support Agent" {
		t.Fatalf("agent_name = %#v, want Support Agent; payload=%#v", payload["agent_name"], payload)
	}
	if payload["binding_kind"] != "database_table" || payload["change_action"] != "bind" || payload["resource_count"] != 2 {
		t.Fatalf("binding summary = kind:%#v action:%#v count:%#v, want database_table bind 2; payload=%#v", payload["binding_kind"], payload["change_action"], payload["resource_count"], payload)
	}
	if payload["href"] != "/console/agents/agent-1/agent" {
		t.Fatalf("href = %#v, want Agent detail href", payload["href"])
	}
	gotBindings, ok := payload["database_bindings"].([]dto.AgentDatabaseBinding)
	if !ok || !reflect.DeepEqual(gotBindings, wantBindings) {
		t.Fatalf("database_bindings = %#v, want %#v", payload["database_bindings"], wantBindings)
	}
	states, ok := payload["binding_final_states"].([]map[string]interface{})
	if !ok || len(states) != 1 {
		t.Fatalf("binding_final_states = %#v, want one database final state", payload["binding_final_states"])
	}
	if states[0]["binding_kind"] != "database_table" || states[0]["change_action"] != "satisfied" || states[0]["final_resource_count"] != 2 {
		t.Fatalf("binding_final_states[0] = %#v, want satisfied database count 2", states[0])
	}
}

func TestListAgentDatabaseTablesReturnsCopyableBindingCandidates(t *testing.T) {
	tool := newListAgentDatabaseTablesTool(&fakeAgentManagementService{
		databaseTablesRespByDataSource: map[string]*dto.AgentDatabaseTablesResponse{
			"db-1": {
				AgentID:      "agent-1",
				WorkspaceID:  "agent-workspace",
				DataSourceID: "db-1",
				Count:        1,
				Data: []dto.AgentDatabaseTableCandidate{{
					DataSourceID: "db-1",
					TableID:      "table-1",
					Name:         "Orders",
				}},
			},
		},
	}).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "runtime-workspace",
		},
	})

	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id":       "agent-1",
		"data_source_id": "db-1",
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	candidates, ok := messages[0].Data["binding_candidates"].([]map[string]interface{})
	if !ok || len(candidates) != 1 {
		t.Fatalf("binding_candidates = %#v, want one copyable candidate", messages[0].Data["binding_candidates"])
	}
	if candidates[0]["id"] != "db-1:table-1" || candidates[0]["name"] != "Orders" {
		t.Fatalf("binding candidate = %#v, want db-1:table-1 Orders", candidates[0])
	}
	binding := mapFromAny(candidates[0]["binding"])
	if binding["data_source_id"] != "db-1" || !reflect.DeepEqual(stringSliceFromAny(binding["table_ids"]), []string{"table-1"}) {
		t.Fatalf("binding candidate binding = %#v, want db-1/table-1", binding)
	}
}

func TestUpdateAgentConfigParsesDatabaseTableBindingAliases(t *testing.T) {
	agentID := "agent-1"
	current := &dto.AgentDraftRuntimeConfigResponse{
		AgentID:     agentID,
		WorkspaceID: "agent-workspace",
		Config: dto.AgentConfigResponse{
			AgentID:       agentID,
			SystemPrompt:  "keep prompt",
			ModelProvider: "openai",
			Model:         "gpt-4o",
		},
	}
	service := &fakeAgentManagementService{draftConfig: current}
	tool := newUpdateAgentConfigTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "runtime-workspace",
		},
	})

	_, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id":              agentID,
		"add_database_bindings": `[{"database_table_ids":["db-1:table-1","db-1:table-2"]}]`,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	wantBindings := []dto.AgentDatabaseBinding{{
		DataSourceID:     "db-1",
		TableIDs:         []string{"table-1", "table-2"},
		WritableTableIDs: []string{},
	}}
	if !reflect.DeepEqual(service.lastConfigRequest.DatabaseBindings, wantBindings) {
		t.Fatalf("DatabaseBindings = %#v, want %#v", service.lastConfigRequest.DatabaseBindings, wantBindings)
	}
}

func TestReplaceAgentWorkflowBindingsReturnsBindingEvidence(t *testing.T) {
	agentID := "agent-1"
	current := &dto.AgentDraftRuntimeConfigResponse{
		AgentID:     agentID,
		WorkspaceID: "agent-workspace",
		Config: dto.AgentConfigResponse{
			AgentID:             agentID,
			SystemPrompt:        "keep prompt",
			ModelProvider:       "openai",
			Model:               "gpt-4o",
			KnowledgeDatasetIDs: []string{"dataset-1"},
			DatabaseBindings:    []dto.AgentDatabaseBinding{{DataSourceID: "db-1", TableIDs: []string{"table-1"}}},
		},
	}
	service := &fakeAgentManagementService{draftConfig: current}
	tool := newReplaceAgentWorkflowBindingsTool(service).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   "org-1",
		InvokeFrom: tools.ToolInvokeFromAIChat,
		RuntimeParameters: map[string]interface{}{
			"organization_id": "org-1",
			"workspace_id":    "runtime-workspace",
		},
	})

	messages, err := tool.Invoke(context.Background(), "account-1", map[string]interface{}{
		"agent_id": agentID,
		"bindings": `[{"binding_id":"workflow-2","label":"Approval Flow","agent_id":"workflow-agent-2","workflow_id":"wf-2","version_strategy":"latest_published","timeout_seconds":600}]`,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	wantBindings := []dto.AgentWorkflowBinding{{
		BindingID:       "workflow-2",
		Label:           "Approval Flow",
		AgentID:         "workflow-agent-2",
		WorkflowID:      "wf-2",
		VersionStrategy: "latest_published",
		TimeoutSeconds:  600,
	}}
	if !reflect.DeepEqual(service.lastConfigRequest.WorkflowBindings, wantBindings) {
		t.Fatalf("WorkflowBindings = %#v, want %#v", service.lastConfigRequest.WorkflowBindings, wantBindings)
	}
	if !reflect.DeepEqual(service.lastConfigRequest.KnowledgeDatasetIDs, current.Config.KnowledgeDatasetIDs) ||
		!reflect.DeepEqual(service.lastConfigRequest.DatabaseBindings, current.Config.DatabaseBindings) {
		t.Fatalf("unrelated bindings not preserved: request=%#v current=%#v", service.lastConfigRequest, current.Config)
	}
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	payload := messages[0].Data
	if payload["status"] != "completed" || payload["effect"] != "updated" || payload["agent_id"] != agentID || payload["workspace_id"] != "agent-workspace" {
		t.Fatalf("payload identity = %#v, want completed updated agent-workspace evidence", payload)
	}
	gotBindings, ok := payload["workflow_bindings"].([]dto.AgentWorkflowBinding)
	if !ok || !reflect.DeepEqual(gotBindings, wantBindings) {
		t.Fatalf("workflow_bindings = %#v, want %#v", payload["workflow_bindings"], wantBindings)
	}
}

func TestListAvailableModelsToolFiltersByUseCaseAndProvider(t *testing.T) {
	organizationID := uuid.New()
	available := &fakeAvailableModelsService{
		models: []*llmmodelservice.AvailableModel{
			{
				ID:              uuid.New(),
				Provider:        "openai",
				Name:            "gpt-4o",
				DisplayName:     "GPT-4o",
				UseCases:        []string{"text-chat", "vision"},
				ContextWindow:   128000,
				MaxOutputTokens: 4096,
				Features: llmmodelmodel.ModelFeatures{
					Streaming:        true,
					FunctionCalling:  true,
					StructuredOutput: true,
				},
				Parameters: llmmodelmodel.ModelParameters{
					SupportsTemperature: true,
					SupportsTopP:        true,
				},
			},
			{
				ID:          uuid.New(),
				Provider:    "openai",
				Name:        "gpt-4o-mini",
				DisplayName: "GPT-4o mini",
				UseCases:    []string{"text-chat"},
			},
		},
	}
	tool := newListAvailableModelsTool(available).ForkToolRuntime(&tools.ToolRuntime{
		TenantID:   organizationID.String(),
		InvokeFrom: tools.ToolInvokeFromAIChat,
	})
	messages, err := tool.Invoke(context.Background(), uuid.New().String(), map[string]interface{}{
		"use_case": "vision",
		"provider": "openai",
		"limit":    1,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if available.organizationID != organizationID {
		t.Fatalf("organizationID = %s, want %s", available.organizationID, organizationID)
	}
	if available.provider != "openai" || available.useCase != "vision" {
		t.Fatalf("filters = provider %q useCase %q, want openai vision", available.provider, available.useCase)
	}
	if len(messages) != 1 || messages[0].Type != tools.ToolInvokeMessageTypeJSON {
		t.Fatalf("messages = %#v, want one JSON message", messages)
	}
	payload := messages[0].Data
	if payload["count"] != 1 || payload["total"] != 2 || payload["truncated"] != true {
		t.Fatalf("payload counts = %#v, want count 1 total 2 truncated true", payload)
	}
	models, ok := payload["models"].([]map[string]interface{})
	if !ok || len(models) != 1 {
		t.Fatalf("models = %#v, want one model payload", payload["models"])
	}
	if models[0]["provider"] != "openai" || models[0]["model"] != "gpt-4o" {
		t.Fatalf("model payload = %#v, want openai/gpt-4o", models[0])
	}
}

func TestNormalizeAgentModelUseCaseDefaultsToTextChat(t *testing.T) {
	if got := normalizeAgentModelUseCase(""); got != "text-chat" {
		t.Fatalf("normalizeAgentModelUseCase(\"\") = %q, want text-chat", got)
	}
	if got := normalizeAgentModelUseCase("function_calling"); got != "function-calling" {
		t.Fatalf("normalizeAgentModelUseCase(function_calling) = %q, want function-calling", got)
	}
	if got := normalizeAgentModelUseCase("all"); got != "" {
		t.Fatalf("normalizeAgentModelUseCase(all) = %q, want empty all filter", got)
	}
}

func assertStringSliceContains(t *testing.T, values []string, want string) {
	t.Helper()
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("values = %#v, want to contain %q", values, want)
}

func boolValue(params map[string]interface{}, key string) bool {
	value, ok := params[key].(bool)
	return ok && value
}

type fakeAvailableModelsService struct {
	models         []*llmmodelservice.AvailableModel
	organizationID uuid.UUID
	provider       string
	useCase        string
}

func (s *fakeAvailableModelsService) ListAvailable(_ context.Context, organizationID uuid.UUID, provider string, useCase string) ([]*llmmodelservice.AvailableModel, error) {
	s.organizationID = organizationID
	s.provider = provider
	s.useCase = useCase
	return s.models, nil
}

type fakeWorkspacePermissionService struct {
	allowed        bool
	organizationID string
	workspaceID    string
	accountID      string
	permissionCode workspacemodel.WorkspacePermissionCode
}

func (s *fakeWorkspacePermissionService) CheckWorkspacePermission(_ context.Context, organizationID, workspaceID, accountID string, permissionCode workspacemodel.WorkspacePermissionCode) (bool, error) {
	s.organizationID = organizationID
	s.workspaceID = workspaceID
	s.accountID = accountID
	s.permissionCode = permissionCode
	return s.allowed, nil
}

type fakeAgentManagementService struct {
	draftConfig                    *dto.AgentDraftRuntimeConfigResponse
	skillCandidatesResp            *dto.AgentSkillCandidatesResponse
	lastSkillCandidatesRequest     dto.AgentSkillCandidatesRequest
	knowledgeCandidatesResp        *dto.AgentKnowledgeCandidatesResponse
	lastKnowledgeCandidatesAgentID string
	lastKnowledgeCandidatesRequest dto.AgentKnowledgeCandidatesRequest
	databaseTablesRespByDataSource map[string]*dto.AgentDatabaseTablesResponse
	workflowCandidatesResp         *dto.AgentWorkflowBindingCandidatesResponse
	createAgentResponse            interface{}
	lastCreateAgentWorkspaceID     string
	lastCreateAgentRequest         map[string]interface{}
	createAgentCalls               int
	updateAgentResponse            interface{}
	updateConfigErr                error
	lastUpdateAgentID              string
	lastUpdateRequest              map[string]interface{}
	updateAgentCalls               int
	updateConfigResponse           *dto.AgentConfigResponse
	lastConfigRequest              dto.AgentConfigRequest
	lastDraftConfigAgentID         string
	getDraftConfigCalls            int
	updateConfigCalls              int
	agents                         map[string]interface{}
	lastListAgentsRequest          dto.GetAgentsListRequest
	deletedAgentIDs                []string
	deleteAgentErrByID             map[string]error
}

func (s *fakeAgentManagementService) GetAgentsListWithPermissions(_ context.Context, _ string, req dto.GetAgentsListRequest) (*dto.AgentsListResponse, error) {
	s.lastListAgentsRequest = req
	resp := &dto.AgentsListResponse{
		Page:  1,
		Limit: req.Limit,
	}
	for id, rawAgent := range s.agents {
		agent := agentPayload(rawAgent)
		name := stringValue(agent, "name")
		if name == "" {
			name = stringValue(agent, "agent_name")
		}
		if req.Keyword != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(req.Keyword)) && !strings.EqualFold(id, req.Keyword) {
			continue
		}
		if req.Name != "" && !strings.EqualFold(name, req.Name) {
			continue
		}
		workspaceID := firstNonEmptyString(stringValue(agent, "workspace_id"), stringValue(agent, "tenant_id"))
		if req.WorkspaceID != "" && workspaceID != "" && req.WorkspaceID != workspaceID {
			continue
		}
		resp.Data = append(resp.Data, dto.AgentListItem{
			ID:           firstNonEmptyString(stringValue(agent, "id"), stringValue(agent, "agent_id"), id),
			Name:         name,
			Description:  stringValue(agent, "description"),
			AgentType:    firstNonEmptyString(stringValue(agent, "agent_type"), "AGENT"),
			WorkspaceID:  workspaceID,
			IsPublished:  boolValue(agent, "is_published"),
			WebAppStatus: stringValue(agent, "web_app_status"),
			CanEdit:      true,
		})
	}
	resp.Total = int64(len(resp.Data))
	return resp, nil
}

func (s *fakeAgentManagementService) GetRunnableWebApps(context.Context, string, dto.GetRunnableWebAppsRequest) (*dto.RunnableWebAppsResponse, error) {
	return nil, nil
}

func (s *fakeAgentManagementService) CreateAgent(_ context.Context, workspaceID string, req interface{}, _ string) (interface{}, error) {
	s.createAgentCalls++
	s.lastCreateAgentWorkspaceID = workspaceID
	s.lastCreateAgentRequest = mapFromAny(req)
	if s.createAgentResponse != nil {
		return s.createAgentResponse, nil
	}
	resp := copyStringAnyMap(s.lastCreateAgentRequest)
	resp["id"] = firstNonEmptyString(stringValue(resp, "id"), "agent-created")
	resp["workspace_id"] = workspaceID
	return resp, nil
}

func (s *fakeAgentManagementService) GetAgent(_ context.Context, agentID string) (interface{}, error) {
	if s.agents != nil {
		if agent, ok := s.agents[agentID]; ok {
			return agent, nil
		}
	}
	return nil, nil
}

func (s *fakeAgentManagementService) UpdateAgent(_ context.Context, agentID string, req interface{}) (interface{}, error) {
	s.updateAgentCalls++
	s.lastUpdateAgentID = agentID
	s.lastUpdateRequest = mapFromAny(req)
	if s.updateAgentResponse != nil {
		if resp := mapFromAny(s.updateAgentResponse); len(resp) > 0 {
			if _, ok := resp["icon"]; !ok {
				if icon := stringValue(s.lastUpdateRequest, "icon"); icon != "" {
					resp["icon"] = icon
				}
			}
			if _, ok := resp["icon_type"]; !ok {
				if iconType := stringValue(s.lastUpdateRequest, "icon_type"); iconType != "" {
					resp["icon_type"] = iconType
				}
			}
			return resp, nil
		}
		return s.updateAgentResponse, nil
	}
	resp := copyStringAnyMap(s.lastUpdateRequest)
	resp["id"] = agentID
	return resp, nil
}

func (s *fakeAgentManagementService) GetAgentConfig(context.Context, string, string) (*dto.AgentConfigResponse, error) {
	if s.draftConfig == nil {
		return nil, errors.New("draft config not found")
	}
	return &s.draftConfig.Config, nil
}

func (s *fakeAgentManagementService) GetAgentDraftRuntimeConfig(_ context.Context, agentID string, _ string) (*dto.AgentDraftRuntimeConfigResponse, error) {
	s.getDraftConfigCalls++
	s.lastDraftConfigAgentID = agentID
	if s.draftConfig == nil {
		return nil, errors.New("draft config not found")
	}
	return s.draftConfig, nil
}

func (s *fakeAgentManagementService) UpdateAgentConfig(_ context.Context, _ string, _ string, req dto.AgentConfigRequest) (*dto.AgentConfigResponse, error) {
	s.updateConfigCalls++
	s.lastConfigRequest = req
	if s.updateConfigErr != nil {
		return nil, s.updateConfigErr
	}
	if s.updateConfigResponse != nil {
		resp := *s.updateConfigResponse
		return &resp, nil
	}
	resp := s.draftConfig.Config
	resp.SystemPrompt = req.SystemPrompt
	resp.ModelProvider = req.ModelProvider
	resp.Model = req.Model
	resp.ModelParameters = copyStringAnyMap(req.ModelParameters)
	resp.AgentMemoryEnabled = req.AgentMemoryEnabled
	resp.FileUpload = req.FileUpload
	resp.HomeTitle = req.HomeTitle
	resp.InputPlaceholder = req.InputPlaceholder
	resp.ThemeColor = req.ThemeColor
	resp.SuggestedQuestions = append([]string(nil), req.SuggestedQuestions...)
	resp.KnowledgeDatasetIDs = append([]string(nil), req.KnowledgeDatasetIDs...)
	resp.KnowledgeRetrievalConfig = copyStringAnyMap(req.KnowledgeRetrievalConfig)
	resp.DatabaseBindings = append([]dto.AgentDatabaseBinding(nil), req.DatabaseBindings...)
	resp.WorkflowBindings = append([]dto.AgentWorkflowBinding(nil), req.WorkflowBindings...)
	resp.EnabledSkillIDs = append([]string(nil), req.EnabledSkillIDs...)
	return &resp, nil
}

func (s *fakeAgentManagementService) ListAgentSkillCandidates(_ context.Context, _ string, _ string, req dto.AgentSkillCandidatesRequest) (*dto.AgentSkillCandidatesResponse, error) {
	s.lastSkillCandidatesRequest = req
	if s.skillCandidatesResp != nil {
		return s.skillCandidatesResp, nil
	}
	return &dto.AgentSkillCandidatesResponse{}, nil
}

func (s *fakeAgentManagementService) ListAgentKnowledgeCandidates(_ context.Context, agentID string, _ string, req dto.AgentKnowledgeCandidatesRequest) (*dto.AgentKnowledgeCandidatesResponse, error) {
	s.lastKnowledgeCandidatesAgentID = agentID
	s.lastKnowledgeCandidatesRequest = req
	if s.knowledgeCandidatesResp != nil {
		return s.knowledgeCandidatesResp, nil
	}
	return &dto.AgentKnowledgeCandidatesResponse{}, nil
}

func (s *fakeAgentManagementService) ListAgentDatabaseCandidates(context.Context, string, string, dto.AgentDatabaseCandidatesRequest) (*dto.AgentDatabaseCandidatesResponse, error) {
	return nil, nil
}

func (s *fakeAgentManagementService) ListAgentDatabaseTables(_ context.Context, _ string, _ string, req dto.AgentDatabaseTablesRequest) (*dto.AgentDatabaseTablesResponse, error) {
	if s.databaseTablesRespByDataSource != nil {
		if resp := s.databaseTablesRespByDataSource[req.DataSourceID]; resp != nil {
			return resp, nil
		}
	}
	return &dto.AgentDatabaseTablesResponse{}, nil
}

func (s *fakeAgentManagementService) ListAgentWorkflowBindingCandidates(context.Context, string, string, dto.AgentWorkflowBindingCandidatesRequest) (*dto.AgentWorkflowBindingCandidatesResponse, error) {
	if s.workflowCandidatesResp != nil {
		return s.workflowCandidatesResp, nil
	}
	return &dto.AgentWorkflowBindingCandidatesResponse{}, nil
}

func (s *fakeAgentManagementService) ListAgentMemorySlots(context.Context, string, string) ([]dto.AgentMemorySlotConfig, error) {
	return nil, nil
}

func (s *fakeAgentManagementService) ReplaceAgentMemorySlots(context.Context, string, string, []dto.AgentMemorySlotConfig) ([]dto.AgentMemorySlotConfig, error) {
	return nil, nil
}

func (s *fakeAgentManagementService) ListAgentMemoryValues(context.Context, string, string) (*dto.AgentMemoryValuesResponse, error) {
	return nil, nil
}

func (s *fakeAgentManagementService) UpdateAgentMemoryValue(context.Context, string, string, dto.UpdateAgentMemoryValueRequest) (*dto.AgentMemoryValueResponse, error) {
	return nil, nil
}

func (s *fakeAgentManagementService) ClearAgentMemoryValue(context.Context, string, string, string) (*dto.AgentMemoryValueResponse, error) {
	return nil, nil
}

func (s *fakeAgentManagementService) GenerateAgentSuggestedQuestions(context.Context, string, string, *dto.GenerateAgentSuggestedQuestionsRequest) (*dto.GenerateSuggestedQuestionsResponse, error) {
	return nil, nil
}

func (s *fakeAgentManagementService) PublishAgent(context.Context, string, string, dto.PublishAgentRequest) (*dto.PublishAgentResponse, error) {
	return nil, nil
}

func (s *fakeAgentManagementService) ListAgentPublishedVersions(context.Context, string, string, int, int) (*dto.AgentPublishedVersionsResponse, error) {
	return nil, nil
}

func (s *fakeAgentManagementService) RollbackAgentPublishedVersion(context.Context, string, string, dto.RollbackAgentPublishedVersionRequest) (*dto.AgentConfigResponse, error) {
	return nil, nil
}

func (s *fakeAgentManagementService) GetPublishedAgentWebAppConfig(context.Context, string) (*dto.AgentWebAppRuntimeConfigResponse, error) {
	return nil, nil
}

func (s *fakeAgentManagementService) UpdateWebAppStatus(context.Context, string, dto.UpdateWebAppStatusRequest) (*dto.WebAppStatusResponse, error) {
	return nil, nil
}

func (s *fakeAgentManagementService) DeleteAgent(_ context.Context, agentID string) error {
	s.deletedAgentIDs = append(s.deletedAgentIDs, agentID)
	if s.deleteAgentErrByID != nil {
		if err := s.deleteAgentErrByID[agentID]; err != nil {
			return err
		}
	}
	return nil
}
