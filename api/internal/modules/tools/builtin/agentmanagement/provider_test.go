package agentmanagement

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmmodelservice "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	"github.com/zgiai/zgi/api/internal/modules/tools"
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
	service := &fakeAgentManagementService{draftConfig: current}
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
	service := &fakeAgentManagementService{draftConfig: current}
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
	if payload["href"] != "/console/agents/agent-1/agent" {
		t.Fatalf("href = %#v, want Agent detail href", payload["href"])
	}
	gotBindings, ok := payload["database_bindings"].([]dto.AgentDatabaseBinding)
	if !ok || !reflect.DeepEqual(gotBindings, wantBindings) {
		t.Fatalf("database_bindings = %#v, want %#v", payload["database_bindings"], wantBindings)
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

type fakeAgentManagementService struct {
	draftConfig                *dto.AgentDraftRuntimeConfigResponse
	skillCandidatesResp        *dto.AgentSkillCandidatesResponse
	lastSkillCandidatesRequest dto.AgentSkillCandidatesRequest
	updateAgentResponse        interface{}
	updateConfigErr            error
	lastUpdateAgentID          string
	lastUpdateRequest          map[string]interface{}
	updateAgentCalls           int
	lastConfigRequest          dto.AgentConfigRequest
	getDraftConfigCalls        int
	updateConfigCalls          int
	agents                     map[string]interface{}
	deletedAgentIDs            []string
	deleteAgentErrByID         map[string]error
}

func (s *fakeAgentManagementService) GetAgentsListWithPermissions(context.Context, string, dto.GetAgentsListRequest) (*dto.AgentsListResponse, error) {
	return nil, nil
}

func (s *fakeAgentManagementService) GetRunnableWebApps(context.Context, string, dto.GetRunnableWebAppsRequest) (*dto.RunnableWebAppsResponse, error) {
	return nil, nil
}

func (s *fakeAgentManagementService) CreateAgent(context.Context, string, interface{}, string) (interface{}, error) {
	return nil, nil
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

func (s *fakeAgentManagementService) GetAgentDraftRuntimeConfig(context.Context, string, string) (*dto.AgentDraftRuntimeConfigResponse, error) {
	s.getDraftConfigCalls++
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

func (s *fakeAgentManagementService) ListAgentKnowledgeCandidates(context.Context, string, string, dto.AgentKnowledgeCandidatesRequest) (*dto.AgentKnowledgeCandidatesResponse, error) {
	return nil, nil
}

func (s *fakeAgentManagementService) ListAgentDatabaseCandidates(context.Context, string, string, dto.AgentDatabaseCandidatesRequest) (*dto.AgentDatabaseCandidatesResponse, error) {
	return nil, nil
}

func (s *fakeAgentManagementService) ListAgentDatabaseTables(context.Context, string, string, dto.AgentDatabaseTablesRequest) (*dto.AgentDatabaseTablesResponse, error) {
	return nil, nil
}

func (s *fakeAgentManagementService) ListAgentWorkflowBindingCandidates(context.Context, string, string, dto.AgentWorkflowBindingCandidatesRequest) (*dto.AgentWorkflowBindingCandidatesResponse, error) {
	return nil, nil
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
