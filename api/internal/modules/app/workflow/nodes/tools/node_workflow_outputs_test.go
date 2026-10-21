package tools

import (
	"reflect"
	"testing"

	toolruntime "github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestProjectWorkflowOutputArrayLeaf(t *testing.T) {
	result := map[string]interface{}{
		"results": []interface{}{
			map[string]interface{}{"url": "https://a.example"},
			map[string]interface{}{"url": "https://b.example"},
		},
	}

	got, ok := projectWorkflowOutput(result, []string{"results", "*", "url"})
	if !ok {
		t.Fatal("projectWorkflowOutput() did not find results[].url")
	}
	want := []interface{}{"https://a.example", "https://b.example"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projectWorkflowOutput() = %#v, want %#v", got, want)
	}
}

func TestConvertResultToOutputsMaterializesTypedProjection(t *testing.T) {
	node := &Node{}
	result := &toolruntime.InvokeResult{Messages: []toolruntime.ToolInvokeMessage{{
		Type: toolruntime.ToolInvokeMessageTypeJSON,
		Data: map[string]interface{}{
			"results": []interface{}{
				map[string]interface{}{"url": "https://a.example"},
				map[string]interface{}{"url": "https://b.example"},
			},
		},
	}}}
	definitions := []toolruntime.ToolWorkflowOutput{{
		ID:   "result__results__url",
		Path: []string{"results", "*", "url"},
		Type: "array[string]",
	}}

	outputs := node.convertResultToOutputs(result, nil, definitions)
	want := []interface{}{"https://a.example", "https://b.example"}
	if !reflect.DeepEqual(outputs["result__results__url"], want) {
		t.Fatalf("projected output = %#v, want %#v", outputs["result__results__url"], want)
	}
	if _, ok := outputs["result"].(map[string]interface{}); !ok {
		t.Fatalf("result output = %#v, want object", outputs["result"])
	}
	if _, ok := outputs["json"].([]map[string]interface{}); !ok {
		t.Fatalf("legacy json output = %#v, want []map", outputs["json"])
	}
}

func TestProjectWorkflowOutputMissingLeafSkipsMissingItems(t *testing.T) {
	result := map[string]interface{}{
		"results": []interface{}{
			map[string]interface{}{"url": "https://a.example"},
			map[string]interface{}{"title": "missing url"},
		},
	}

	got, ok := projectWorkflowOutput(result, []string{"results", "*", "url"})
	if !ok {
		t.Fatal("projectWorkflowOutput() did not find any URL")
	}
	want := []interface{}{"https://a.example"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projectWorkflowOutput() = %#v, want %#v", got, want)
	}
}
