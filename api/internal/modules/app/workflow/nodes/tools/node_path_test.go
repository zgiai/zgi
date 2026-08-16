package tools

import (
	"reflect"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
)

func TestPrepareToolParameters_UsesNestedSelector(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"source", "payload"}, map[string]any{
		"field": "nested value",
	})

	node := &Node{
		nodeData: NodeData{
			ToolParameters: map[string]*ToolInput{
				"result": {
					Type:  "variable",
					Value: []string{"source", "payload", "field"},
				},
			},
		},
	}

	params, err := node.prepareToolParameters(vp)
	if err != nil {
		t.Fatalf("prepareToolParameters() error = %v", err)
	}

	got, ok := params["result"].(string)
	if !ok {
		t.Fatalf("params[result] = %T, want string", params["result"])
	}
	if got != "nested value" {
		t.Fatalf("params[result] = %q, want %q", got, "nested value")
	}
}

func TestPrepareToolParameters_CoercesScalarToSingletonArray(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"source", "url"}, "https://example.com")

	node := &Node{
		nodeData: NodeData{
			ToolParameters: map[string]*ToolInput{
				"urls": {
					Type:   "variable",
					Value:  []string{"source", "url"},
					Coerce: "singleton_array",
				},
			},
		},
	}

	params, err := node.prepareToolParameters(vp)
	if err != nil {
		t.Fatalf("prepareToolParameters() error = %v", err)
	}
	want := []interface{}{"https://example.com"}
	if got := params["urls"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("params[urls] = %#v, want %#v", got, want)
	}
}

func TestPrepareToolParameters_SkipsNilVariableSelector(t *testing.T) {
	vp := entities.NewVariablePool()

	node := &Node{
		nodeData: NodeData{
			ToolParameters: map[string]*ToolInput{
				"pdfBase64": {
					Type: "variable",
				},
			},
		},
	}

	params, err := node.prepareToolParameters(vp)
	if err != nil {
		t.Fatalf("prepareToolParameters() error = %v", err)
	}
	if _, exists := params["pdfBase64"]; exists {
		t.Fatalf("params[pdfBase64] should be skipped, got %#v", params["pdfBase64"])
	}
}

func TestPrepareToolParameters_SkipsEmptyVariableSelector(t *testing.T) {
	vp := entities.NewVariablePool()

	node := &Node{
		nodeData: NodeData{
			ToolParameters: map[string]*ToolInput{
				"pdfBase64": {
					Type:  "variable",
					Value: []string{},
				},
			},
		},
	}

	params, err := node.prepareToolParameters(vp)
	if err != nil {
		t.Fatalf("prepareToolParameters() error = %v", err)
	}
	if _, exists := params["pdfBase64"]; exists {
		t.Fatalf("params[pdfBase64] should be skipped, got %#v", params["pdfBase64"])
	}
}

func TestPrepareToolParameters_KeepsMixedAndSkipsEmptyVariable(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"iter", "item"}, map[string]any{
		"url": "https://example.com/file.pdf",
	})

	node := &Node{
		nodeData: NodeData{
			ToolParameters: map[string]*ToolInput{
				"url": {
					Type:  "mixed",
					Value: "{{#iter.item.url#}}",
				},
				"pdfBase64": {
					Type: "variable",
				},
			},
		},
	}

	params, err := node.prepareToolParameters(vp)
	if err != nil {
		t.Fatalf("prepareToolParameters() error = %v", err)
	}

	if got, ok := params["url"].(string); !ok || got != "https://example.com/file.pdf" {
		t.Fatalf("params[url] = %#v, want resolved url", params["url"])
	}
	if _, exists := params["pdfBase64"]; exists {
		t.Fatalf("params[pdfBase64] should be skipped, got %#v", params["pdfBase64"])
	}
}

func TestPrepareToolParameters_ConnectorOmitsEmptyOptionalValues(t *testing.T) {
	vp := entities.NewVariablePool()
	node := &Node{
		nodeData: NodeData{
			ProviderType: "connector",
			ToolParameters: map[string]*ToolInput{
				"query":                {Type: "mixed", Value: "release notes"},
				"start_published_date": {Type: "constant", Value: ""},
				"end_published_date":   {Type: "mixed", Value: "  "},
				"include_domains":      {Type: "constant", Value: []any{}},
				"enabled":              {Type: "constant", Value: false},
				"limit":                {Type: "constant", Value: 0},
			},
		},
	}

	params, err := node.prepareToolParameters(vp)
	if err != nil {
		t.Fatalf("prepareToolParameters() error = %v", err)
	}
	for _, name := range []string{"start_published_date", "end_published_date", "include_domains"} {
		if _, exists := params[name]; exists {
			t.Fatalf("params[%s] should be omitted, got %#v", name, params[name])
		}
	}
	if params["query"] != "release notes" || params["enabled"] != false || params["limit"] != 0 {
		t.Fatalf("meaningful connector parameters were not preserved: %#v", params)
	}
}

func TestPrepareToolParameters_BuiltinKeepsEmptyValuesForCompatibility(t *testing.T) {
	vp := entities.NewVariablePool()
	node := &Node{
		nodeData: NodeData{
			ProviderType: "builtin",
			ToolParameters: map[string]*ToolInput{
				"optional_text": {Type: "constant", Value: ""},
			},
		},
	}

	params, err := node.prepareToolParameters(vp)
	if err != nil {
		t.Fatalf("prepareToolParameters() error = %v", err)
	}
	if value, exists := params["optional_text"]; !exists || value != "" {
		t.Fatalf("builtin empty value compatibility changed: %#v", params)
	}
}

func TestPrepareToolParameters_ErrsOnScalarVariableSelector(t *testing.T) {
	vp := entities.NewVariablePool()

	node := &Node{
		nodeData: NodeData{
			ToolParameters: map[string]*ToolInput{
				"pdfBase64": {
					Type:  "variable",
					Value: "foo",
				},
			},
		},
	}

	if _, err := node.prepareToolParameters(vp); err == nil {
		t.Fatal("prepareToolParameters() error = nil, want invalid selector error")
	}
}

func TestPrepareToolParameters_ErrsOnDirtyVariableSelector(t *testing.T) {
	vp := entities.NewVariablePool()

	node := &Node{
		nodeData: NodeData{
			ToolParameters: map[string]*ToolInput{
				"pdfBase64": {
					Type:  "variable",
					Value: []interface{}{"a", 1},
				},
			},
		},
	}

	if _, err := node.prepareToolParameters(vp); err == nil {
		t.Fatal("prepareToolParameters() error = nil, want invalid selector error")
	}
}

func TestResolveTemplateVariables_UsesNestedSelector(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"source", "payload"}, map[string]any{
		"field": "nested value",
	})

	node := &Node{}
	got := node.resolveTemplateVariables("value={{#source.payload.field#}}", vp)
	want := "value=nested value"
	if got != want {
		t.Fatalf("resolveTemplateVariables() = %q, want %q", got, want)
	}
}

func TestParseNodeDataFromConfigReadsConnectorConnection(t *testing.T) {
	data, nodeID, err := parseNodeDataFromConfig(map[string]any{
		"id": "tool-node",
		"data": map[string]interface{}{
			"provider_type":   "connector",
			"provider_id":     "github",
			"tool_name":       "list_github_repositories",
			"connection_id":   "connection-123",
			"tool_parameters": map[string]interface{}{},
		},
	})
	if err != nil {
		t.Fatalf("parseNodeDataFromConfig() error = %v", err)
	}
	if nodeID != "tool-node" || data.ConnectionID != "connection-123" || data.ProviderType != "connector" {
		t.Fatalf("parsed connector data = %#v, nodeID = %q", data, nodeID)
	}
}

func TestWorkflowToolRuntimeParametersIncludesConnectorScope(t *testing.T) {
	pool := entities.NewVariablePool()
	pool.Add([]string{"sys", "workflow_run_id"}, "run-123")
	node := &Node{
		NodeStruct: base.NodeStruct{
			TenantID: "legacy-workspace", WorkspaceID: "workspace-123", WorkflowID: "workflow-123", NodeID: "tool-node",
		},
		nodeData: NodeData{ConnectionID: "connection-123"},
	}

	parameters := node.workflowToolRuntimeParameters(pool)
	for key, want := range map[string]string{
		"connection_id": "connection-123", "workspace_id": "workspace-123", "workflow_id": "workflow-123", "workflow_run_id": "run-123", "node_id": "tool-node",
	} {
		if got := parameters[key]; got != want {
			t.Fatalf("runtime parameter %s = %#v, want %q", key, got, want)
		}
	}
}
