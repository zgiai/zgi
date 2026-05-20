package template

import (
	"testing"
)

func TestPongo2Transformer(t *testing.T) {
	transformer := NewPongo2TemplateTransformer()

	code := "Hello {{name}}"
	inputs := map[string]interface{}{
		"name": "World",
	}

	runner, preload, err := transformer.TransformCaller(code, inputs)
	if err != nil {
		t.Fatalf("Failed to transform caller: %v", err)
	}

	if runner == "" {
		t.Error("Runner script should not be empty")
	}

	if preload == "" {
		t.Error("Preload script should not be empty")
	}

	// Test response transformation
	mockResponse := "<<RESULT>>Hello World<<RESULT>>"
	result, err := transformer.TransformResponse(mockResponse)
	if err != nil {
		t.Fatalf("Failed to transform response: %v", err)
	}

	if result["result"] != "Hello World" {
		t.Errorf("Expected 'Hello World', got %v", result["result"])
	}
}

func TestTemplateExecutor_Pongo2(t *testing.T) {
	executor := NewTemplateExecutor()

	template := "Hello {{name}}"
	inputs := map[string]interface{}{
		"name": "World",
	}

	response, err := executor.ExecuteWorkflowCodeTemplate(LanguagePongo2, template, inputs)
	if err != nil {
		t.Fatalf("Failed to execute template: %v", err)
	}

	if response == nil {
		t.Error("Expected response to not be nil")
	}

	if result, ok := response["result"]; !ok || result == nil {
		t.Error("Expected result field to exist and not be nil")
	}

	if result, ok := response["result"].(string); ok {
		expected := "Hello World"
		if result != expected {
			t.Errorf("Expected %q, got %q", expected, result)
		}
	}
}
