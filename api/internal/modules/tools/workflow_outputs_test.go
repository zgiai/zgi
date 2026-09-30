package tools

import "testing"

func TestCompileWorkflowOutputsProjectsArrayObjectLeaves(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"request_id": map[string]interface{}{"type": "string"},
			"cost_usd":   map[string]interface{}{"type": []string{"number", "null"}},
			"results": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"url":   map[string]interface{}{"type": "string", "title": "网页地址"},
						"score": map[string]interface{}{"type": "number"},
					},
				},
			},
		},
	}

	outputs := CompileWorkflowOutputs(schema)
	want := map[string]string{
		"result__cost_usd":       "number",
		"result__request_id":     "string",
		"result__results":        "array[object]",
		"result__results__score": "array[number]",
		"result__results__url":   "array[string]",
	}
	for _, output := range outputs {
		if expected, ok := want[output.ID]; ok {
			if output.Type != expected {
				t.Fatalf("output %s type = %s, want %s", output.ID, output.Type, expected)
			}
			delete(want, output.ID)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing outputs: %#v; got %#v", want, outputs)
	}
}

func TestCompileWorkflowOutputsProjectsNestedObjectLeaves(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"user": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{"type": "string"},
				},
			},
		},
	}

	outputs := CompileWorkflowOutputs(schema)
	found := false
	for _, output := range outputs {
		if output.ID == "result__user__id" && output.Type == "string" {
			found = true
		}
	}
	if !found {
		t.Fatalf("nested scalar output missing: %#v", outputs)
	}
}
