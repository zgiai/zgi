package skilltrace

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

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
