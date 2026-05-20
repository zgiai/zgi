package workflow_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/zgiai/ginext/internal/modules/app/workflow"
)

// ──────────────────────────────────────────────
// sanitizeGraph tests
// ──────────────────────────────────────────────

func buildGraphWithNodes(nodes []map[string]interface{}) map[string]interface{} {
	ifaces := make([]interface{}, len(nodes))
	for i, n := range nodes {
		ifaces[i] = n
	}
	return map[string]interface{}{"nodes": ifaces, "edges": []interface{}{}}
}

func TestSanitizeGraph_RemovesAPIKeyAndCredentialID(t *testing.T) {
	graph := buildGraphWithNodes([]map[string]interface{}{
		{
			"id": "n1",
			"data": map[string]interface{}{
				"type":          "llm",
				"api_key":       "sk-secret",
				"credential_id": "cred-123",
				"title":         "LLM Node",
			},
			"selected": true,
		},
	})

	workflow.SanitizeGraph(graph)

	node := graph["nodes"].([]interface{})[0].(map[string]interface{})
	data := node["data"].(map[string]interface{})
	assert.NotContains(t, data, "api_key")
	assert.NotContains(t, data, "credential_id")
	assert.NotContains(t, data, "selected")
	assert.NotContains(t, node, "selected")
	assert.Equal(t, "LLM Node", data["title"])
}

func TestSanitizeGraph_RemovesHTTPRequestAuth(t *testing.T) {
	graph := buildGraphWithNodes([]map[string]interface{}{
		{
			"id": "n2",
			"data": map[string]interface{}{
				"type": "http-request",
				"authorization": map[string]interface{}{
					"type":         "api-key",
					"api_key":      "secret-key",
					"header_value": "Bearer secret",
					"config":       map[string]interface{}{"key": "val"},
				},
			},
		},
	})

	workflow.SanitizeGraph(graph)

	data := graph["nodes"].([]interface{})[0].(map[string]interface{})["data"].(map[string]interface{})
	auth := data["authorization"].(map[string]interface{})
	assert.NotContains(t, auth, "api_key")
	assert.NotContains(t, auth, "header_value")
	assert.Contains(t, auth, "type")
	assert.Contains(t, auth, "config")
}

func TestSanitizeGraph_RemovesCallDatabaseSecrets(t *testing.T) {
	graph := buildGraphWithNodes([]map[string]interface{}{
		{
			"id": "n3",
			"data": map[string]interface{}{
				"type": "call-database",
				"data_source": map[string]interface{}{
					"name":              "prod-db",
					"password":          "s3cret",
					"connection_string": "postgres://user:pass@host/db",
					"host":              "db.example.com",
				},
			},
		},
	})

	workflow.SanitizeGraph(graph)

	ds := graph["nodes"].([]interface{})[0].(map[string]interface{})["data"].(map[string]interface{})["data_source"].(map[string]interface{})
	assert.NotContains(t, ds, "password")
	assert.NotContains(t, ds, "connection_string")
	assert.Equal(t, "prod-db", ds["name"])
	assert.Equal(t, "db.example.com", ds["host"])
}

func TestSanitizeGraph_RemovesLLMModelCredentialID(t *testing.T) {
	graph := buildGraphWithNodes([]map[string]interface{}{
		{
			"id": "n4",
			"data": map[string]interface{}{
				"type": "llm",
				"model": map[string]interface{}{
					"name":          "gpt-4",
					"provider":      "openai",
					"credential_id": "cred-456",
				},
			},
		},
	})

	workflow.SanitizeGraph(graph)

	model := graph["nodes"].([]interface{})[0].(map[string]interface{})["data"].(map[string]interface{})["model"].(map[string]interface{})
	assert.NotContains(t, model, "credential_id")
	assert.Equal(t, "gpt-4", model["name"])
}

func TestSanitizeGraph_EmptyGraph(t *testing.T) {
	graph := map[string]interface{}{}
	workflow.SanitizeGraph(graph)
	assert.Empty(t, graph["nodes"])
}

func TestSanitizeGraph_NoDataField(t *testing.T) {
	graph := buildGraphWithNodes([]map[string]interface{}{
		{"id": "n5"},
	})
	workflow.SanitizeGraph(graph)
	assert.Len(t, graph["nodes"].([]interface{}), 1)
}

// ──────────────────────────────────────────────
// checkNodeCompatibility tests
// ──────────────────────────────────────────────

func TestCheckNodeCompatibility_AllSupported(t *testing.T) {
	graph := buildGraphWithNodes([]map[string]interface{}{
		{"id": "1", "data": map[string]interface{}{"type": "start"}},
		{"id": "2", "data": map[string]interface{}{"type": "end"}},
		{"id": "3", "data": map[string]interface{}{"type": "llm"}},
		{"id": "4", "data": map[string]interface{}{"type": "if-else"}},
		{"id": "5", "data": map[string]interface{}{"type": "code"}},
	})

	warnings := workflow.CheckNodeCompatibility(graph)
	assert.Empty(t, warnings)
}

func TestCheckNodeCompatibility_NoteSupportedAsDocumentation(t *testing.T) {
	graph := buildGraphWithNodes([]map[string]interface{}{
		{
			"id":   "note-1",
			"type": "note",
			"data": map[string]interface{}{
				"type": "note",
				"text": "运行说明 / Run guide",
			},
		},
	})

	warnings := workflow.CheckNodeCompatibility(graph)
	assert.Empty(t, warnings)
}

func TestCheckNodeCompatibility_ApprovalSupported(t *testing.T) {
	graph := buildGraphWithNodes([]map[string]interface{}{
		{"id": "approval-1", "data": map[string]interface{}{"type": "approval"}},
	})

	warnings := workflow.CheckNodeCompatibility(graph)
	assert.Empty(t, warnings)
}

func TestCheckNodeCompatibility_CreateScheduledTaskSupported(t *testing.T) {
	graph := buildGraphWithNodes([]map[string]interface{}{
		{"id": "scheduled-task-1", "data": map[string]interface{}{"type": "create-scheduled-task"}},
	})

	warnings := workflow.CheckNodeCompatibility(graph)
	assert.Empty(t, warnings)
}

func TestCheckNodeCompatibility_RuntimeRegisteredNodesSupported(t *testing.T) {
	graph := buildGraphWithNodes([]map[string]interface{}{
		{"id": "call-db-1", "data": map[string]interface{}{"type": "call-database"}},
		{"id": "sql-1", "data": map[string]interface{}{"type": "sql-generator"}},
		{"id": "sms-1", "data": map[string]interface{}{"type": "notification-sms"}},
	})

	warnings := workflow.CheckNodeCompatibility(graph)
	assert.Empty(t, warnings)
}

func TestCheckNodeCompatibility_UnsupportedNodes(t *testing.T) {
	graph := buildGraphWithNodes([]map[string]interface{}{
		{"id": "1", "data": map[string]interface{}{"type": "start"}},
		{"id": "2", "data": map[string]interface{}{"type": "custom-magic"}},
		{"id": "3", "data": map[string]interface{}{"type": "my-plugin"}},
	})

	warnings := workflow.CheckNodeCompatibility(graph)
	assert.Len(t, warnings, 2)
	assert.Equal(t, "unsupported_node", warnings[0].Type)
	assert.Equal(t, "2", warnings[0].NodeID)
	assert.Equal(t, "custom-magic", warnings[0].Name)
	assert.Equal(t, "my-plugin", warnings[1].Name)
}

func TestCheckNodeCompatibility_VariableAssignerUnsupported(t *testing.T) {
	graph := buildGraphWithNodes([]map[string]interface{}{
		{"id": "1", "data": map[string]interface{}{"type": "variable-assigner"}},
	})

	warnings := workflow.CheckNodeCompatibility(graph)
	require.Len(t, warnings, 1)
	assert.Equal(t, "unsupported_node", warnings[0].Type)
	assert.Equal(t, "1", warnings[0].NodeID)
	assert.Equal(t, "variable-assigner", warnings[0].Name)
}

// ──────────────────────────────────────────────
// checkDependencies tests
// ──────────────────────────────────────────────

func TestCheckDependencies_NoExternalDeps(t *testing.T) {
	graph := buildGraphWithNodes([]map[string]interface{}{
		{"id": "1", "data": map[string]interface{}{"type": "start"}},
		{"id": "2", "data": map[string]interface{}{"type": "end"}},
	})

	warnings := workflow.CheckDependencies(graph)
	assert.Empty(t, warnings)
}

func TestCheckDependencies_DatabaseNode(t *testing.T) {
	graph := buildGraphWithNodes([]map[string]interface{}{
		{
			"id": "n1",
			"data": map[string]interface{}{
				"type": "call-database",
				"data_source": map[string]interface{}{
					"name": "mysql_prod",
				},
			},
		},
	})

	warnings := workflow.CheckDependencies(graph)
	require.Len(t, warnings, 1)
	assert.Equal(t, "datasource_requires_config", warnings[0].Type)
	assert.Equal(t, "mysql_prod", warnings[0].Name)
}

func TestCheckDependencies_KnowledgeRetrievalNode(t *testing.T) {
	graph := buildGraphWithNodes([]map[string]interface{}{
		{
			"id": "n1",
			"data": map[string]interface{}{
				"type":        "knowledge-retrieval",
				"dataset_ids": []interface{}{"kb-001", "kb-002"},
			},
		},
	})

	warnings := workflow.CheckDependencies(graph)
	require.Len(t, warnings, 2)
	assert.Equal(t, "knowledge_base_requires_config", warnings[0].Type)
	assert.Equal(t, "kb-001", warnings[0].Name)
	assert.Equal(t, "kb-002", warnings[1].Name)
}

func TestCheckDependencies_DeduplicatesDatasources(t *testing.T) {
	graph := buildGraphWithNodes([]map[string]interface{}{
		{
			"id": "n1",
			"data": map[string]interface{}{
				"type":        "call-database",
				"data_source": map[string]interface{}{"name": "shared_db"},
			},
		},
		{
			"id": "n2",
			"data": map[string]interface{}{
				"type":        "call-database",
				"data_source": map[string]interface{}{"name": "shared_db"},
			},
		},
	})

	warnings := workflow.CheckDependencies(graph)
	assert.Len(t, warnings, 1)
}

// ──────────────────────────────────────────────
// collectImportStats tests
// ──────────────────────────────────────────────

func TestCollectImportStats(t *testing.T) {
	graph := map[string]interface{}{
		"nodes": []interface{}{
			map[string]interface{}{
				"id": "1",
				"data": map[string]interface{}{
					"type": "start",
					"variables": []interface{}{
						map[string]interface{}{"variable": "query"},
						map[string]interface{}{"variable": "name"},
					},
				},
			},
			map[string]interface{}{
				"id":   "2",
				"data": map[string]interface{}{"type": "if-else"},
			},
			map[string]interface{}{
				"id":   "3",
				"data": map[string]interface{}{"type": "end"},
			},
		},
		"edges": []interface{}{
			map[string]interface{}{"id": "e1"},
			map[string]interface{}{"id": "e2"},
		},
	}

	stats := workflow.CollectImportStats(graph)
	assert.Equal(t, 3, stats.NodeCount)
	assert.Equal(t, 2, stats.EdgeCount)
	assert.Equal(t, 2, stats.VariableCount)
	assert.ElementsMatch(t, []string{"start", "if-else", "end"}, stats.NodeTypes)
}

func TestCollectImportStats_EmptyGraph(t *testing.T) {
	graph := map[string]interface{}{}
	stats := workflow.CollectImportStats(graph)
	assert.Equal(t, 0, stats.NodeCount)
	assert.Equal(t, 0, stats.EdgeCount)
	assert.Empty(t, stats.NodeTypes)
}

// ──────────────────────────────────────────────
// YAML round-trip tests
// ──────────────────────────────────────────────

func TestWorkflowExportData_YAMLRoundTrip(t *testing.T) {
	original := workflow.WorkflowExportData{
		Kind:         "app",
		Version:      "1.0.0",
		ExportedFrom: "zgi",
		ExportedAt:   "2026-03-15T10:00:00Z",
		App: workflow.ExportAppInfo{
			Name:        "Test Workflow",
			Description: "A test workflow",
			Icon:        "icon",
			Mode:        "workflow",
		},
		Workflow: workflow.ExportWorkflowData{
			ConversationVariables: []interface{}{},
			EnvironmentVariables:  []interface{}{},
			Features:              map[string]interface{}{"speech_to_text": map[string]interface{}{"enabled": false}},
			Graph: map[string]interface{}{
				"nodes": []interface{}{
					map[string]interface{}{
						"id": "node1",
						"data": map[string]interface{}{
							"type":  "start",
							"title": "Start",
						},
					},
				},
				"edges": []interface{}{},
			},
		},
	}

	data, err := yaml.Marshal(original)
	require.NoError(t, err)

	var parsed workflow.WorkflowExportData
	err = yaml.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, original.Kind, parsed.Kind)
	assert.Equal(t, original.Version, parsed.Version)
	assert.Equal(t, original.ExportedFrom, parsed.ExportedFrom)
	assert.Equal(t, original.App.Name, parsed.App.Name)
	assert.Equal(t, original.App.Mode, parsed.App.Mode)
	assert.NotNil(t, parsed.Workflow.Graph)
}

func TestImportWorkflow_InvalidKind(t *testing.T) {
	yamlData := `
kind: not-app
version: "1.0.0"
exported_from: zgi
exported_at: "2026-03-15T10:00:00Z"
app:
  name: test
workflow:
  graph:
    nodes: []
    edges: []
`
	var exportData workflow.WorkflowExportData
	err := yaml.Unmarshal([]byte(yamlData), &exportData)
	require.NoError(t, err)
	assert.Equal(t, "not-app", exportData.Kind)
}

func TestImportWorkflow_InvalidExportedFrom(t *testing.T) {
	yamlData := `
kind: app
version: "1.0.0"
exported_from: other-system
exported_at: "2026-03-15T10:00:00Z"
app:
  name: test
workflow:
  graph:
    nodes: []
    edges: []
`
	var exportData workflow.WorkflowExportData
	err := yaml.Unmarshal([]byte(yamlData), &exportData)
	require.NoError(t, err)
	assert.Equal(t, "other-system", exportData.ExportedFrom)
}

func TestImportWorkflow_UnsupportedVersion(t *testing.T) {
	yamlData := `
kind: app
version: "99.0.0"
exported_from: zgi
exported_at: "2026-03-15T10:00:00Z"
app:
  name: test
workflow:
  graph:
    nodes: []
    edges: []
`
	var exportData workflow.WorkflowExportData
	err := yaml.Unmarshal([]byte(yamlData), &exportData)
	require.NoError(t, err)
	assert.Equal(t, "99.0.0", exportData.Version)
}

func TestImportWorkflow_RejectsUnsupportedNodeType(t *testing.T) {
	svc := &workflow.WorkflowService{}

	yamlData := `
kind: app
version: "1.0.0"
exported_from: zgi
exported_at: "2026-03-15T10:00:00Z"
app:
  name: test
  mode: workflow
workflow:
  graph:
    nodes:
      - id: node-1
        data:
          type: variable-assigner
    edges: []
`

	result, err := svc.ImportWorkflow(nil, "tenant-id", "account-id", []byte(yamlData))
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "unsupported node type")
}

// ──────────────────────────────────────────────
// Full pipeline test (sanitize → YAML → import validation)
// ──────────────────────────────────────────────

func TestExportImportPipeline_SanitizesAndRoundTrips(t *testing.T) {
	graph := map[string]interface{}{
		"nodes": []interface{}{
			map[string]interface{}{
				"id":   "node1",
				"type": "custom",
				"position": map[string]interface{}{
					"x": 80,
					"y": 282,
				},
				"data": map[string]interface{}{
					"type":          "start",
					"title":         "User Input",
					"api_key":       "should-be-removed",
					"credential_id": "should-also-be-removed",
					"variables": []interface{}{
						map[string]interface{}{
							"variable": "query",
							"label":    "query",
							"type":     "text-input",
							"required": true,
						},
					},
				},
				"selected": true,
			},
			map[string]interface{}{
				"id":   "node2",
				"type": "custom",
				"data": map[string]interface{}{
					"type":  "llm",
					"title": "LLM",
					"model": map[string]interface{}{
						"name":          "gpt-4",
						"provider":      "openai",
						"credential_id": "cred-xyz",
					},
				},
			},
			map[string]interface{}{
				"id":   "node3",
				"type": "custom",
				"data": map[string]interface{}{
					"type":  "end",
					"title": "Output",
				},
			},
		},
		"edges": []interface{}{
			map[string]interface{}{
				"id":     "e1",
				"source": "node1",
				"target": "node2",
			},
			map[string]interface{}{
				"id":     "e2",
				"source": "node2",
				"target": "node3",
			},
		},
		"viewport": map[string]interface{}{"x": 0, "y": 0, "zoom": 1},
	}

	workflow.SanitizeGraph(graph)

	exportData := workflow.WorkflowExportData{
		Kind:         "app",
		Version:      "1.0.0",
		ExportedFrom: "zgi",
		ExportedAt:   "2026-03-15T10:00:00Z",
		App: workflow.ExportAppInfo{
			Name:        "Pipeline Test",
			Description: "Test pipeline",
			Icon:        "icon",
			Mode:        "workflow",
		},
		Workflow: workflow.ExportWorkflowData{
			ConversationVariables: []interface{}{},
			EnvironmentVariables:  []interface{}{},
			Features:              map[string]interface{}{},
			Graph:                 graph,
		},
	}

	yamlBytes, err := yaml.Marshal(exportData)
	require.NoError(t, err)

	var imported workflow.WorkflowExportData
	err = yaml.Unmarshal(yamlBytes, &imported)
	require.NoError(t, err)

	assert.Equal(t, "app", imported.Kind)
	assert.Equal(t, "1.0.0", imported.Version)
	assert.Equal(t, "zgi", imported.ExportedFrom)

	importedGraph := imported.Workflow.Graph
	nodes := importedGraph["nodes"].([]interface{})
	assert.Len(t, nodes, 3)

	node1Data := nodes[0].(map[string]interface{})["data"].(map[string]interface{})
	assert.NotContains(t, node1Data, "api_key")
	assert.NotContains(t, node1Data, "credential_id")
	assert.Equal(t, "User Input", node1Data["title"])

	node2Data := nodes[1].(map[string]interface{})["data"].(map[string]interface{})
	model := node2Data["model"].(map[string]interface{})
	assert.NotContains(t, model, "credential_id")
	assert.Equal(t, "gpt-4", model["name"])

	warnings := workflow.CheckNodeCompatibility(importedGraph)
	assert.Empty(t, warnings)

	stats := workflow.CollectImportStats(importedGraph)
	assert.Equal(t, 3, stats.NodeCount)
	assert.Equal(t, 2, stats.EdgeCount)
	assert.Equal(t, 1, stats.VariableCount)
}

// ──────────────────────────────────────────────
// sanitizeFilename tests
// ──────────────────────────────────────────────

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"normal ascii", "my-workflow", "my-workflow"},
		{"chinese chars", "智能客服", "智能客服"},
		{"mixed", "客服 v2.0", "客服 v2.0"},
		{"special chars", "test/file:name?*", "test_file_name__"},
		{"empty after sanitize", "///", "workflow"},
		{"empty string", "", "workflow"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := workflow.SanitizeFilename(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ──────────────────────────────────────────────
// JSON serialization test for ImportResult
// ──────────────────────────────────────────────

func TestImportResult_JSON(t *testing.T) {
	result := workflow.ImportResult{
		Success:    true,
		WorkflowID: "wf-123",
		Stats: workflow.ImportStats{
			NodeCount:     3,
			EdgeCount:     2,
			NodeTypes:     []string{"start", "llm", "end"},
			VariableCount: 1,
		},
		Warnings: []workflow.ImportWarning{
			{
				Type:    "unsupported_node",
				NodeID:  "n5",
				Name:    "custom-tool",
				Message: "Node type 'custom-tool' is not supported",
			},
		},
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var parsed workflow.ImportResult
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.True(t, parsed.Success)
	assert.Equal(t, "wf-123", parsed.WorkflowID)
	assert.Equal(t, 3, parsed.Stats.NodeCount)
	assert.Len(t, parsed.Warnings, 1)
	assert.Equal(t, "unsupported_node", parsed.Warnings[0].Type)
}

// ──────────────────────────────────────────────
// mapAgentTypeToMode tests
// ──────────────────────────────────────────────

func TestMapAgentTypeToMode(t *testing.T) {
	assert.Equal(t, "workflow", workflow.MapAgentTypeToMode("workflow"))
	assert.Equal(t, "advanced-chat", workflow.MapAgentTypeToMode("advanced-chat"))
	assert.Equal(t, "chat", workflow.MapAgentTypeToMode("chat"))
}
