package skilltrace

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestToolGovernanceDecisionPayloadIncludesAssetOperationAudit(t *testing.T) {
	payload := ToolGovernanceDecisionPayload(PayloadIDs{
		ConversationID: "conversation-1",
		MessageID:      "message-1",
	}, skills.SkillTrace{
		Kind:     "tool_governance",
		SkillID:  skills.SkillFileReader,
		ToolName: "delete_file",
		Status:   string(toolgovernance.DecisionStatusNeedsApproval),
		Governance: &toolgovernance.Decision{
			Status:           toolgovernance.DecisionStatusNeedsApproval,
			RequiresApproval: true,
			CorrelationID:    "corr-1",
			Manifest: toolgovernance.Manifest{
				ToolID:    "file.delete",
				Effect:    toolgovernance.EffectDelete,
				AssetType: "file",
				RiskLevel: toolgovernance.RiskLevelHigh,
			},
			AssetOperationAudit: map[string]interface{}{
				"schema_version":  "tool_governance.asset_operation.v1",
				"correlation_id":  "corr-1",
				"tool_id":         "file.delete",
				"approval_status": "pending",
			},
		},
	})

	audit, ok := payload["asset_operation_audit"].(map[string]interface{})
	if !ok || audit["tool_id"] != "file.delete" || audit["approval_status"] != "pending" {
		t.Fatalf("asset_operation_audit = %#v, want governance audit payload", payload["asset_operation_audit"])
	}
}

func TestSkillCallErrorPayloadPreservesGuardrailBlockedStatus(t *testing.T) {
	payload := SkillCallErrorPayload(PayloadIDs{
		ConversationID: "conversation-1",
		MessageID:      "message-1",
	}, skills.SkillTrace{
		Kind:     "guardrail",
		SkillID:  skills.SkillFileGenerator,
		ToolName: "generate_file",
		Status:   "blocked",
		Error:    "Use a safer tool order.",
	}, "error", true)

	if payload["status"] != "blocked" {
		t.Fatalf("status = %#v, want blocked", payload["status"])
	}
	if payload["kind"] != "guardrail" {
		t.Fatalf("kind = %#v, want guardrail", payload["kind"])
	}
}

func TestSkillArtifactsFromToolMessagesIncludesGovernanceOperation(t *testing.T) {
	artifacts := SkillArtifactsFromToolMessages(PayloadIDs{
		ConversationID: "conversation-1",
		MessageID:      "message-1",
	}, skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillFileReader,
		ToolName: "generate_pdf",
		Status:   "success",
		Governance: &toolgovernance.Decision{
			CorrelationID: "corr-file-create",
			AssetOperationAudit: map[string]interface{}{
				"correlation_id":  "corr-file-create",
				"tool_id":         "file.generate_pdf",
				"approval_status": "approved",
			},
		},
	}, []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeFile,
		Text: "http://files.example/file.pdf?download=1",
		Meta: map[string]interface{}{
			"file": map[string]interface{}{
				"id":              "file-1",
				"filename":        "file.pdf",
				"extension":       ".pdf",
				"mime_type":       "application/pdf",
				"size":            int64(128),
				"url":             "http://files.example/file.pdf",
				"download_url":    "http://files.example/file.pdf?download=1",
				"transfer_method": "tool_file",
			},
		},
	}})

	if len(artifacts) != 1 {
		t.Fatalf("artifacts = %#v, want one artifact", artifacts)
	}
	if artifacts[0]["correlation_id"] != "corr-file-create" ||
		artifacts[0]["operation_id"] != "tool_governance:corr-file-create" {
		t.Fatalf("artifact operation fields = %#v", artifacts[0])
	}
	audit, ok := artifacts[0]["asset_operation_audit"].(map[string]interface{})
	if !ok || audit["tool_id"] != "file.generate_pdf" || audit["approval_status"] != "approved" {
		t.Fatalf("asset_operation_audit = %#v, want governance audit payload", artifacts[0]["asset_operation_audit"])
	}
}

func TestSkillArtifactsFromToolMessagesIncludesManagedFileJSON(t *testing.T) {
	artifacts := SkillArtifactsFromToolMessages(PayloadIDs{
		ConversationID: "conversation-1",
		MessageID:      "message-1",
	}, skills.SkillTrace{
		Kind:     "tool_call",
		SkillID:  skills.SkillFileManager,
		ToolName: "save_file_to_management",
		Status:   "success",
		Governance: &toolgovernance.Decision{
			CorrelationID: "corr-file-save",
			AssetOperationAudit: map[string]interface{}{
				"correlation_id":  "corr-file-save",
				"tool_id":         "file.save_to_management",
				"approval_status": "approved",
			},
		},
	}, []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":          "completed",
			"file_id":         "managed-1",
			"upload_file_id":  "managed-1",
			"filename":        "chart.svg",
			"mime_type":       "image/svg+xml",
			"size":            int64(256),
			"target":          "managed_file",
			"workspace_id":    "workspace-1",
			"transfer_method": "local_file",
			"source_type":     "tool_file",
			"source_file_id":  "tool-1",
			"download_url":    "/console/api/files/managed-1/download",
		},
	}})

	if len(artifacts) != 1 {
		t.Fatalf("artifacts = %#v, want one managed artifact", artifacts)
	}
	artifact := artifacts[0]
	for key, want := range map[string]interface{}{
		"file_id":        "managed-1",
		"upload_file_id": "managed-1",
		"filename":       "chart.svg",
		"target":         "managed_file",
		"workspace_id":   "workspace-1",
		"source_file_id": "tool-1",
		"correlation_id": "corr-file-save",
		"operation_id":   "tool_governance:corr-file-save",
	} {
		if artifact[key] != want {
			t.Fatalf("managed artifact %s = %#v, want %#v in %#v", key, artifact[key], want, artifact)
		}
	}
	audit, ok := artifact["asset_operation_audit"].(map[string]interface{})
	if !ok || audit["tool_id"] != "file.save_to_management" || audit["approval_status"] != "approved" {
		t.Fatalf("asset_operation_audit = %#v, want governance audit payload", artifact["asset_operation_audit"])
	}
}

func TestSummarizeToolResultCompactsAgentKnowledgePayload(t *testing.T) {
	result := SummarizeToolResult(skills.SkillAgentKnowledge, "retrieve_agent_knowledge", []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"query":               "refund policy",
			"status":              "success",
			"result_count":        1,
			"top_score":           0.91,
			"source_summary":      []interface{}{map[string]interface{}{"position": 1, "dataset_name": "Policies"}},
			"context":             "full context should not be copied",
			"context_blocks":      []interface{}{map[string]interface{}{"content": "full block should not be copied"}},
			"retriever_resources": []interface{}{map[string]interface{}{"content": "full resource should not be copied"}},
		},
	}})
	if result["status"] != "success" {
		t.Fatalf("status = %#v, want success", result["status"])
	}
	if _, ok := result["source_summary"]; !ok {
		t.Fatalf("source_summary missing: %#v", result)
	}
	for _, key := range []string{"context", "context_blocks", "retriever_resources"} {
		if _, ok := result[key]; ok {
			t.Fatalf("%s should not be included in compact trace result: %#v", key, result)
		}
	}
}

func TestSummarizeToolResultCompactsInternalKnowledgeListPayload(t *testing.T) {
	result := SummarizeToolResult(skills.SkillInternalKnowledge, "list_accessible_knowledge_bases", []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"query":           "refund",
			"status":          "fallback",
			"result_count":    1,
			"fallback_used":   true,
			"limit":           20,
			"warnings":        []interface{}{"no match"},
			"knowledge_bases": []interface{}{map[string]interface{}{"dataset_id": "ds-1", "name": "Policies"}},
		},
	}})
	if result["status"] != "fallback" {
		t.Fatalf("status = %#v, want fallback", result["status"])
	}
	if result["fallback_used"] != true {
		t.Fatalf("fallback_used = %#v, want true", result["fallback_used"])
	}
	if _, ok := result["knowledge_bases"]; ok {
		t.Fatalf("knowledge_bases should not be included in compact trace result: %#v", result)
	}
}

func TestSummarizeToolResultCompactsDatabasePayloads(t *testing.T) {
	tests := []struct {
		name      string
		toolName  string
		payload   map[string]interface{}
		countKey  string
		wantCount int
		omitKey   string
	}{
		{
			name:     "list databases",
			toolName: "list_accessible_databases",
			payload: map[string]interface{}{
				"databases": []map[string]interface{}{{"data_source_id": "db-1"}, {"data_source_id": "db-2"}},
			},
			countKey:  "databases_count",
			wantCount: 2,
			omitKey:   "databases",
		},
		{
			name:     "describe table",
			toolName: "describe_database_table",
			payload: map[string]interface{}{
				"data_source": map[string]interface{}{
					"data_source_id": "db-1",
					"name":           "CRM",
					"schema_name":    "public",
				},
				"table": map[string]interface{}{
					"table_id":            "tbl-1",
					"name":                "customers",
					"physical_table_id":   123,
					"physical_table_name": "zgi_base_tbl_customers",
				},
				"columns": []map[string]interface{}{{"name": "id"}, {"name": "title"}},
			},
			countKey:  "columns_count",
			wantCount: 2,
			omitKey:   "columns",
		},
		{
			name:     "query records",
			toolName: "query_table_records",
			payload: map[string]interface{}{
				"data_source": map[string]interface{}{
					"data_source_id": "db-1",
					"name":           "CRM",
					"schema_name":    "public",
				},
				"table": map[string]interface{}{
					"table_id":            "tbl-1",
					"name":                "customers",
					"physical_table_id":   123,
					"physical_table_name": "zgi_base_tbl_customers",
				},
				"records":   []map[string]interface{}{{"id": "1"}, {"id": "2"}},
				"total_num": 2,
				"has_more":  false,
			},
			countKey:  "records_count",
			wantCount: 2,
			omitKey:   "records",
		},
		{
			name:     "mutation",
			toolName: "update_table_records",
			payload: map[string]interface{}{
				"data_source": map[string]interface{}{
					"data_source_id": "db-1",
					"name":           "CRM",
					"schema_name":    "public",
				},
				"table": map[string]interface{}{
					"table_id":            "tbl-1",
					"name":                "customers",
					"physical_table_id":   123,
					"physical_table_name": "zgi_base_tbl_customers",
				},
				"affected_rows": 3,
			},
			countKey:  "affected_rows",
			wantCount: 3,
			omitKey:   "table",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SummarizeToolResult(skills.SkillAgentDatabase, tt.toolName, []tools.ToolInvokeMessage{{
				Type: tools.ToolInvokeMessageTypeJSON,
				Data: tt.payload,
			}})
			if result[tt.countKey] != tt.wantCount {
				t.Fatalf("%s = %#v, want %d", tt.countKey, result[tt.countKey], tt.wantCount)
			}
			if _, ok := result[tt.omitKey]; ok {
				t.Fatalf("%s should not be included in compact trace result: %#v", tt.omitKey, result)
			}
			for _, key := range []string{"data_source", "data_source_id", "table", "table_id", "physical_table_id", "physical_table_name", "records", "columns"} {
				if _, ok := result[key]; ok {
					t.Fatalf("%s should not be included in compact trace result: %#v", key, result)
				}
			}
			if tt.payload["data_source"] != nil && result["database_name"] != "CRM" {
				t.Fatalf("database_name = %#v, want CRM in %#v", result["database_name"], result)
			}
			if tt.payload["table"] != nil && result["table_name"] != "customers" {
				t.Fatalf("table_name = %#v, want customers in %#v", result["table_name"], result)
			}
		})
	}
}

func TestSummarizeToolResultCompactsAgentWorkflowPayload(t *testing.T) {
	result := SummarizeToolResult(skills.SkillAgentWorkflow, "run_agent_workflow", []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":          "succeeded",
			"workflow_run_id": "run-1",
			"elapsed_time":    1.2,
			"output_keys":     []string{"answer"},
			"primary_output":  "done",
			"outputs":         map[string]interface{}{"answer": "done", "debug": "full output"},
		},
	}})
	if result["status"] != "succeeded" || result["workflow_run_id"] != "run-1" || result["primary_output"] != "done" {
		t.Fatalf("workflow summary = %#v, want status/run/primary output", result)
	}
	if _, ok := result["outputs"]; ok {
		t.Fatalf("outputs should not be included in compact trace result: %#v", result)
	}
}

func TestSummarizeToolResultPreservesAgentConfigUpdatedFields(t *testing.T) {
	result := SummarizeToolResult(skills.SkillAgentManagement, "update_agent_config", []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":           "completed",
			"effect":           "updated",
			"agent_id":         "agent-1",
			"workspace_id":     "workspace-1",
			"requested_fields": []string{"model_provider", "model", "home_title", "theme_color"},
			"satisfied_fields": []string{"model_provider", "model", "home_title", "theme_color"},
			"updated_fields":   []string{"model_provider", "model", "home_title"},
			"config": map[string]interface{}{
				"system_prompt":        "full prompt should stay out of compact trace",
				"model_provider":       "deepseek",
				"model":                "deepseek-chat",
				"home_title":           "Home",
				"agent_memory_enabled": true,
				"file_upload":          true,
				"enabled_skill_ids":    []interface{}{"time"},
				"suggested_questions":  []interface{}{"配置是否已保存？", "模型是否可用？"},
			},
		},
	}})

	if result["agent_id"] != "agent-1" || result["workspace_id"] != "workspace-1" {
		t.Fatalf("summary identity = %#v, want agent/workspace evidence", result)
	}
	fields, ok := result["updated_fields"].([]string)
	if !ok || len(fields) != 3 || fields[0] != "model_provider" || fields[1] != "model" || fields[2] != "home_title" {
		t.Fatalf("updated_fields = %#v, want exact field evidence", result["updated_fields"])
	}
	satisfiedFields, ok := result["satisfied_fields"].([]string)
	if !ok || len(satisfiedFields) != 4 || satisfiedFields[3] != "theme_color" {
		t.Fatalf("satisfied_fields = %#v, want requested satisfied field evidence", result["satisfied_fields"])
	}
	if _, ok := result["config"]; ok {
		t.Fatalf("config should not be included in compact trace result: %#v", result)
	}
	if result["model_provider"] != "deepseek" || result["model"] != "deepseek-chat" {
		t.Fatalf("model evidence = %#v/%#v, want deepseek/deepseek-chat; summary=%#v", result["model_provider"], result["model"], result)
	}
	if result["agent_memory_enabled"] != true || result["file_upload"] != true || result["enabled_skill_count"] != 1 {
		t.Fatalf("config evidence = %#v, want compact switches/counts", result)
	}
	questions, ok := result["suggested_questions"].([]string)
	if !ok || len(questions) != 2 || questions[0] != "配置是否已保存？" || questions[1] != "模型是否可用？" {
		t.Fatalf("suggested_questions = %#v, want compact question text evidence", result["suggested_questions"])
	}
	if result["suggested_question_count"] != 2 {
		t.Fatalf("suggested_question_count = %#v, want 2", result["suggested_question_count"])
	}
}

func TestSummarizeToolResultPreservesAgentConfigReadModelEvidence(t *testing.T) {
	result := SummarizeToolResult(skills.SkillAgentManagement, "get_agent_config", []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":       "completed",
			"agent_id":     "agent-1",
			"workspace_id": "workspace-1",
			"config": map[string]interface{}{
				"model_provider": "deepseek",
				"model":          "deepseek-chat",
			},
		},
	}})

	if result["status"] != "completed" || result["agent_id"] != "agent-1" || result["workspace_id"] != "workspace-1" {
		t.Fatalf("summary identity = %#v, want completed agent/workspace evidence", result)
	}
	if result["model_provider"] != "deepseek" || result["model"] != "deepseek-chat" {
		t.Fatalf("model evidence = %#v/%#v, want deepseek/deepseek-chat; summary=%#v", result["model_provider"], result["model"], result)
	}
	if _, ok := result["config"]; ok {
		t.Fatalf("config should not be included in compact trace result: %#v", result)
	}
}

func TestSummarizeToolResultPreservesAgentConfigDatabaseTableCount(t *testing.T) {
	result := SummarizeToolResult(skills.SkillAgentManagement, "get_agent_config", []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":   "completed",
			"agent_id": "agent-1",
			"config": map[string]interface{}{
				"database_bindings": []interface{}{
					map[string]interface{}{"table_ids": []interface{}{"table-1", "table-2"}},
				},
			},
		},
	}})

	if result["database_binding_count"] != 1 || result["database_table_count"] != 2 {
		t.Fatalf("database evidence = %#v/%#v, want binding count 1 and table count 2", result["database_binding_count"], result["database_table_count"])
	}
}

func TestSummarizeToolResultPreservesAgentConfigReadModelEvidenceFromStruct(t *testing.T) {
	result := SummarizeToolResult(skills.SkillAgentManagement, "get_agent_config", []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":       "completed",
			"agent_id":     "agent-1",
			"workspace_id": "workspace-1",
			"config": struct {
				ModelProvider     string                   `json:"model_provider"`
				Model             string                   `json:"model"`
				FileUploadEnabled bool                     `json:"file_upload_enabled"`
				AgentMemorySlots  []map[string]interface{} `json:"agent_memory_slots"`
				ModelParameters   map[string]interface{}   `json:"model_parameters"`
			}{
				ModelProvider:     "deepseek",
				Model:             "deepseek-chat",
				FileUploadEnabled: true,
				AgentMemorySlots: []map[string]interface{}{
					{"slot": "preference"},
				},
				ModelParameters: map[string]interface{}{"temperature": 0.7},
			},
		},
	}})

	if result["model_provider"] != "deepseek" || result["model"] != "deepseek-chat" {
		t.Fatalf("model evidence = %#v/%#v, want deepseek/deepseek-chat; summary=%#v", result["model_provider"], result["model"], result)
	}
	if result["file_upload_enabled"] != true || result["memory_slot_config_count"] != 1 || result["model_parameter_count"] != 1 {
		t.Fatalf("config evidence = %#v, want compact switches/counts from struct config", result)
	}
	if _, ok := result["config"]; ok {
		t.Fatalf("config should not be included in compact trace result: %#v", result)
	}
}

func TestSummarizeToolResultPreservesAgentMemorySlotReplacementEvidence(t *testing.T) {
	result := SummarizeToolResult(skills.SkillAgentManagement, "replace_agent_memory_slots", []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":   "completed",
			"effect":   "updated",
			"agent_id": "agent-1",
			"href":     "/console/agents/agent-1/agent",
			"config": map[string]interface{}{
				"agent_memory_slots": []interface{}{
					map[string]interface{}{"key": "preference", "description": "User preferences", "enabled": true},
					map[string]interface{}{"key": "constraints", "description": "Important constraints", "enabled": true},
				},
			},
		},
	}})

	if result["status"] != "completed" || result["effect"] != "updated" || result["agent_id"] != "agent-1" {
		t.Fatalf("summary identity = %#v, want completed updated agent evidence", result)
	}
	if result["href"] != "/console/agents/agent-1/agent" {
		t.Fatalf("href = %#v, want Agent detail href; summary=%#v", result["href"], result)
	}
	if result["memory_slot_config_count"] != 2 {
		t.Fatalf("memory_slot_config_count = %#v, want 2; summary=%#v", result["memory_slot_config_count"], result)
	}
	if _, ok := result["config"]; ok {
		t.Fatalf("config should not be included in compact trace result: %#v", result)
	}
	if _, ok := result["agent_memory_slots"]; ok {
		t.Fatalf("agent_memory_slots should stay out of compact trace result: %#v", result)
	}
}

func TestSummarizeToolResultCompactsAgentManagementBindingPayloads(t *testing.T) {
	tests := []struct {
		name         string
		toolName     string
		payload      map[string]interface{}
		bindingKind  string
		resourceName string
		forbidden    []string
	}{
		{
			name:     "knowledge binding replacement",
			toolName: "replace_agent_knowledge_bindings",
			payload: map[string]interface{}{
				"status":      "completed",
				"effect":      "updated",
				"agent_id":    "agent-1",
				"agent_name":  "Support Agent",
				"dataset_ids": []interface{}{"kb-1"},
				"knowledge_bases": []interface{}{
					map[string]interface{}{"dataset_id": "kb-1", "name": "Policies"},
				},
			},
			bindingKind:  "knowledge_base",
			resourceName: "Policies",
			forbidden:    []string{"agent-1", "kb-1"},
		},
		{
			name:     "database binding replacement",
			toolName: "replace_agent_database_bindings",
			payload: map[string]interface{}{
				"status":     "completed",
				"effect":     "updated",
				"agent_id":   "agent-1",
				"agent_name": "Support Agent",
				"bindings": []interface{}{map[string]interface{}{
					"data_source_id":   "db-1",
					"data_source_name": "CRM",
					"table_ids":        []interface{}{"table-1"},
					"tables": []interface{}{
						map[string]interface{}{"table_id": "table-1", "table_name": "customers"},
					},
				}},
			},
			bindingKind:  "database_table",
			resourceName: "CRM.customers",
			forbidden:    []string{"agent-1", "db-1", "table-1"},
		},
		{
			name:     "workflow binding replacement",
			toolName: "replace_agent_workflow_bindings",
			payload: map[string]interface{}{
				"status":     "completed",
				"effect":     "updated",
				"agent_id":   "agent-1",
				"agent_name": "Support Agent",
				"bindings": []interface{}{map[string]interface{}{
					"binding_id":  "binding-1",
					"workflow_id": "workflow-1",
					"label":       "Approval Flow",
				}},
			},
			bindingKind:  "workflow",
			resourceName: "Approval Flow",
			forbidden:    []string{"agent-1", "binding-1", "workflow-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SummarizeToolResult(skills.SkillAgentManagement, tt.toolName, []tools.ToolInvokeMessage{{
				Type: tools.ToolInvokeMessageTypeJSON,
				Data: tt.payload,
			}})
			if result["binding_kind"] != tt.bindingKind || result["resource_count"] != 1 || result["agent_name"] != "Support Agent" {
				t.Fatalf("summary = %#v, want binding kind/count/agent name", result)
			}
			names, ok := result["resource_names"].([]string)
			if !ok || len(names) != 1 || names[0] != tt.resourceName {
				t.Fatalf("resource_names = %#v, want %q", result["resource_names"], tt.resourceName)
			}
			for _, key := range []string{"agent_id", "dataset_ids", "knowledge_dataset_ids", "database_bindings", "workflow_bindings", "bindings"} {
				if _, ok := result[key]; ok {
					t.Fatalf("%s should not be included in compact binding trace result: %#v", key, result)
				}
			}
			for _, rawID := range tt.forbidden {
				if summaryContainsString(result, rawID) {
					t.Fatalf("summary contains raw id %q: %#v", rawID, result)
				}
			}
		})
	}
}

func TestSummarizeToolResultCountsAgentDatabaseBindingsFromConfig(t *testing.T) {
	result := SummarizeToolResult(skills.SkillAgentManagement, "replace_agent_database_bindings", []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":     "completed",
			"effect":     "updated",
			"agent_name": "Support Agent",
			"config": &dto.AgentConfigResponse{
				DatabaseBindings: []dto.AgentDatabaseBinding{{
					DataSourceID: "db-1",
					TableIDs:     []string{"table-1", "table-2"},
				}},
			},
		},
	}})

	if result["binding_kind"] != "database_table" || result["resource_count"] != 2 {
		t.Fatalf("summary = %#v, want database binding count from typed config", result)
	}
	if summaryContainsString(result, "table-1") || summaryContainsString(result, "table-2") {
		t.Fatalf("summary should not expose raw table ids: %#v", result)
	}
	if _, ok := result["config"]; ok {
		t.Fatalf("config should not be included in compact trace result: %#v", result)
	}
}

func TestSummarizeToolResultPreservesAgentBindingDiffAction(t *testing.T) {
	result := SummarizeToolResult(skills.SkillAgentManagement, "replace_agent_workflow_bindings", []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":                 "completed",
			"effect":                 "updated",
			"agent_id":               "agent-1",
			"agent_name":             "Support Agent",
			"binding_kind":           "workflow",
			"change_action":          "unbind",
			"resource_count":         1,
			"removed_resource_count": 1,
			"removed_resource_names": []string{"Approval Flow"},
			"config": &dto.AgentConfigResponse{
				WorkflowBindings: nil,
			},
		},
	}})

	if result["change_action"] != "unbind" || result["resource_count"] != 1 || result["removed_resource_count"] != 1 {
		t.Fatalf("summary = %#v, want unbind diff counts preserved", result)
	}
	names, ok := result["removed_resource_names"].([]string)
	if !ok || len(names) != 1 || names[0] != "Approval Flow" {
		t.Fatalf("removed_resource_names = %#v, want Approval Flow", result["removed_resource_names"])
	}
	if summaryContainsString(result, "agent-1") {
		t.Fatalf("summary contains raw agent id: %#v", result)
	}
}

func TestSummarizeToolResultPreservesAgentSkillBindingEvidence(t *testing.T) {
	result := SummarizeToolResult(skills.SkillAgentManagement, "replace_agent_skill_bindings", []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":               "completed",
			"effect":               "updated",
			"agent_id":             "agent-1",
			"agent_name":           "Support Agent",
			"binding_kind":         "agent_skill",
			"change_action":        "bind",
			"resource_count":       1,
			"final_resource_count": 1,
			"config": &dto.AgentConfigResponse{
				EnabledSkillIDs: []string{"chart-generator"},
			},
		},
	}})

	if result["binding_kind"] != "agent_skill" || result["change_action"] != "bind" || result["resource_count"] != 1 {
		t.Fatalf("summary = %#v, want agent skill binding evidence", result)
	}
	if result["agent_name"] != "Support Agent" {
		t.Fatalf("agent_name = %#v, want Support Agent", result["agent_name"])
	}
	if summaryContainsString(result, "agent-1") || summaryContainsString(result, "chart-generator") {
		t.Fatalf("summary should not expose raw ids: %#v", result)
	}
	if _, ok := result["config"]; ok {
		t.Fatalf("config should not be included in compact trace result: %#v", result)
	}
}

func TestSummarizeToolResultPreservesAgentCandidateSamples(t *testing.T) {
	result := SummarizeToolResult(skills.SkillAgentManagement, "list_agent_database_tables", []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status": "completed",
			"count":  2,
			"tables": []interface{}{
				map[string]interface{}{"table_id": "table-1", "name": "customers", "selected": false, "writable": true},
				map[string]interface{}{"table_id": "table-2", "name": "orders", "selected": false, "writable": false},
			},
			"binding_candidates": []interface{}{
				map[string]interface{}{
					"id":       "database-1:table-1",
					"name":     "customers",
					"selected": false,
					"writable": true,
					"binding": map[string]interface{}{
						"data_source_id": "database-1",
						"table_ids":      []interface{}{"table-1"},
					},
				},
			},
		},
	}})

	if result["candidates_count"] != 2 {
		t.Fatalf("summary = %#v, want table candidates count", result)
	}
	samples := recordsFromAny(result["candidate_samples"])
	if len(samples) != 1 {
		t.Fatalf("candidate_samples = %#v, want one binding sample", result["candidate_samples"])
	}
	if samples[0]["id"] != "database-1:table-1" || samples[0]["name"] != "customers" {
		t.Fatalf("candidate sample = %#v, want compact id/name", samples[0])
	}
	binding := recordFromAny(samples[0]["binding"])
	if binding["data_source_id"] != "database-1" {
		t.Fatalf("candidate binding = %#v, want data_source_id", binding)
	}
	if got := compactStringList(binding["table_ids"], 3, 64); len(got) != 1 || got[0] != "table-1" {
		t.Fatalf("candidate binding table_ids = %#v, want table-1", binding["table_ids"])
	}
}

func TestSummarizeToolResultPreservesAgentSkillCandidateEmptyCount(t *testing.T) {
	result := SummarizeToolResult(skills.SkillAgentManagement, "list_agent_skill_candidates", []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status": "completed",
			"query":  "missing-skill",
			"count":  0,
			"data":   []interface{}{},
		},
	}})

	if result["count"] != 0 {
		t.Fatalf("summary count = %#v, want 0; result=%#v", result["count"], result)
	}
	if result["candidates_count"] != 0 {
		t.Fatalf("summary candidates_count = %#v, want 0; result=%#v", result["candidates_count"], result)
	}
	if result["query"] != "missing-skill" {
		t.Fatalf("summary query = %#v, want missing-skill", result["query"])
	}
}

func TestSummarizeToolResultCompactsConsoleNavigationPayload(t *testing.T) {
	result := SummarizeToolResult(skills.SkillConsoleNavigator, "navigate", []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":      "navigation_requested",
			"event_type":  "page_navigation_requested",
			"href":        "/console/files",
			"label":       "Files",
			"reason":      "The user asked to open files.",
			"internal_id": "should-not-leak",
		},
	}})
	for key, want := range map[string]interface{}{
		"status":     "navigation_requested",
		"event_type": "page_navigation_requested",
		"href":       "/console/files",
		"label":      "Files",
		"reason":     "The user asked to open files.",
	} {
		if result[key] != want {
			t.Fatalf("%s = %#v, want %#v in %#v", key, result[key], want, result)
		}
	}
	if _, ok := result["internal_id"]; ok {
		t.Fatalf("internal_id should not be included in compact trace result: %#v", result)
	}
}

func summaryContainsString(value interface{}, needle string) bool {
	switch typed := value.(type) {
	case string:
		return typed == needle
	case map[string]interface{}:
		for _, item := range typed {
			if summaryContainsString(item, needle) {
				return true
			}
		}
	case []string:
		for _, item := range typed {
			if summaryContainsString(item, needle) {
				return true
			}
		}
	case []interface{}:
		for _, item := range typed {
			if summaryContainsString(item, needle) {
				return true
			}
		}
	case []map[string]interface{}:
		for _, item := range typed {
			if summaryContainsString(item, needle) {
				return true
			}
		}
	}
	return false
}

func TestSummarizeToolResultCompactsFileDeletePayload(t *testing.T) {
	result := SummarizeToolResult(skills.SkillFileReader, "delete_file", []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":        "completed",
			"deleted_count": 1,
			"reversible":    false,
			"file": map[string]interface{}{
				"id":           "file-1",
				"name":         "smoke.txt",
				"workspace_id": "workspace-1",
				"extension":    "txt",
				"mime_type":    "text/plain",
				"size":         42,
				"created_by":   "account-1",
			},
		},
	}})
	for key, want := range map[string]interface{}{
		"status":            "completed",
		"deleted_count":     1,
		"reversible":        false,
		"file_id":           "file-1",
		"file_name":         "smoke.txt",
		"file_workspace_id": "workspace-1",
		"file_extension":    "txt",
		"file_mime_type":    "text/plain",
		"file_size":         42,
	} {
		if result[key] != want {
			t.Fatalf("%s = %#v, want %#v in %#v", key, result[key], want, result)
		}
	}
	if _, ok := result["created_by"]; ok {
		t.Fatalf("created_by should not be included in compact trace result: %#v", result)
	}
	if _, ok := result["file_created_by"]; ok {
		t.Fatalf("file_created_by should not be included in compact trace result: %#v", result)
	}
}

func TestSummarizeToolResultCompactsFileManagerSavePayload(t *testing.T) {
	result := SummarizeToolResult(skills.SkillFileManager, "save_file_to_management", []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":          "completed",
			"target":          "managed_file",
			"transfer_method": "local_file",
			"source_type":     "tool_file",
			"file_id":         "managed-file-1",
			"upload_file_id":  "managed-file-1",
			"filename":        "星空小猫.svg",
			"temporary_url":   "https://example.invalid/temp.svg",
		},
	}})
	for key, want := range map[string]interface{}{
		"status":          "completed",
		"target":          "managed_file",
		"transfer_method": "local_file",
		"source_type":     "tool_file",
		"file_id":         "managed-file-1",
		"upload_file_id":  "managed-file-1",
		"filename":        "星空小猫.svg",
	} {
		if result[key] != want {
			t.Fatalf("%s = %#v, want %#v in %#v", key, result[key], want, result)
		}
	}
	if _, ok := result["temporary_url"]; ok {
		t.Fatalf("temporary_url should not be included in compact trace result: %#v", result)
	}
}

func TestSummarizeToolResultCompactsFileReadPayload(t *testing.T) {
	const rawContent = "SECRET FILE CONTENT SHOULD NOT BE IN TRACE"
	result := SummarizeToolResult(skills.SkillFileReader, "read_file", []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":            "completed",
			"max_chars":         4000,
			"content_status":    "extracted",
			"content":           rawContent,
			"content_chars":     123,
			"content_truncated": false,
			"from_cache":        true,
			"file": map[string]interface{}{
				"id":           "file-1",
				"name":         "invoice.xlsx",
				"workspace_id": "workspace-1",
				"extension":    "xlsx",
				"mime_type":    "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
				"size":         42,
				"created_by":   "account-1",
			},
		},
	}})
	for key, want := range map[string]interface{}{
		"status":                 "completed",
		"max_chars":              4000,
		"content_status":         "extracted",
		"content_chars":          123,
		"content_truncated":      false,
		"from_cache":             true,
		"content_returned_chars": len([]rune(rawContent)),
		"content_redacted":       true,
		"file_id":                "file-1",
		"file_name":              "invoice.xlsx",
		"file_workspace_id":      "workspace-1",
		"file_extension":         "xlsx",
		"file_size":              42,
	} {
		if result[key] != want {
			t.Fatalf("%s = %#v, want %#v in %#v", key, result[key], want, result)
		}
	}
	for _, key := range []string{"content", "created_by", "file_created_by"} {
		if _, ok := result[key]; ok {
			t.Fatalf("%s should not be included in compact trace result: %#v", key, result)
		}
	}
}

func TestSummarizeToolResultCompactsVisibleFilesPayload(t *testing.T) {
	const rawPreview = "VISIBLE LIST SHOULD NOT CARRY CONTENT PREVIEW"
	result := SummarizeToolResult(skills.SkillFileReader, "list_visible_files", []tools.ToolInvokeMessage{{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"status":         "completed",
			"count":          1,
			"selected_count": 1,
			"files": []map[string]interface{}{{
				"visible_index":   2,
				"file_id":         "file-2",
				"name":            "notes.pdf",
				"workspace_id":    "workspace-1",
				"extension":       "pdf",
				"mime_type":       "application/pdf",
				"selected":        true,
				"content_preview": rawPreview,
			}},
		},
	}})
	if result["status"] != "completed" || result["count"] != 1 || result["selected_count"] != 1 {
		t.Fatalf("summary fields = %#v, want status/count/selected_count", result)
	}
	files, ok := result["files"].([]map[string]interface{})
	if !ok || len(files) != 1 {
		t.Fatalf("files = %#v, want one compact file", result["files"])
	}
	if files[0]["file_id"] != "file-2" || files[0]["name"] != "notes.pdf" || files[0]["content_preview_redacted"] != true {
		t.Fatalf("file summary = %#v, want safe file fields with preview redaction", files[0])
	}
	if _, ok := files[0]["content_preview"]; ok {
		t.Fatalf("content_preview should not be included in compact trace file: %#v", files[0])
	}
}
