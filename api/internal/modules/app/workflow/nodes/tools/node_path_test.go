package tools

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
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
