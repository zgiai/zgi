package parameterextractor

import "testing"

func validParameterExtractorConfig(parameters []map[string]any) map[string]any {
	return map[string]any{
		"id": "parameter-extractor-node",
		"data": map[string]any{
			"title":          "parameter-extractor",
			"type":           "parameter-extractor",
			"query":          []any{"sys", "query"},
			"instruction":    "extract parameters",
			"reasoning_mode": "prompt",
			"model": map[string]any{
				"provider":          "openai",
				"name":              "gpt-4o",
				"mode":              "chat",
				"completion_params": map[string]any{},
			},
			"parameters": parameters,
		},
	}
}

func TestParseParameterExtractorNodeData_NormalizesBooleanType(t *testing.T) {
	nodeData, _, err := parseParameterExtractorNodeDataFromConfig(validParameterExtractorConfig([]map[string]any{
		{
			"name":        "has_invoice",
			"type":        "boolean",
			"description": "whether invoice is present",
			"required":    false,
		},
	}))
	if err != nil {
		t.Fatalf("parseParameterExtractorNodeDataFromConfig() error = %v", err)
	}
	if got := nodeData.Parameters[0].Type; got != ParameterTypeBool {
		t.Fatalf("parameter type = %q, want %q", got, ParameterTypeBool)
	}
}

func TestParseParameterExtractorNodeData_AcceptsArrayBooleanType(t *testing.T) {
	nodeData, _, err := parseParameterExtractorNodeDataFromConfig(validParameterExtractorConfig([]map[string]any{
		{
			"name":        "flags",
			"type":        "array[boolean]",
			"description": "boolean flags",
			"required":    false,
		},
	}))
	if err != nil {
		t.Fatalf("parseParameterExtractorNodeDataFromConfig() error = %v", err)
	}
	if got := nodeData.Parameters[0].Type; got != ParameterTypeArrayBool {
		t.Fatalf("parameter type = %q, want %q", got, ParameterTypeArrayBool)
	}
}

func TestParameterExtractorArrayBooleanValidationAndTransform(t *testing.T) {
	params := []ParameterConfig{
		{Name: "flags", Type: ParameterTypeArrayBool, Required: true},
	}
	result := map[string]any{
		"flags": []any{true, "false", float64(1), 0},
	}

	if err := NewValidator(params).Validate(result); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	transformed, err := NewResultTransformer(params).Transform(result)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}

	flags, ok := transformed["flags"].([]any)
	if !ok {
		t.Fatalf("transformed flags has type %T, want []any", transformed["flags"])
	}
	want := []bool{true, false, true, false}
	if len(flags) != len(want) {
		t.Fatalf("transformed flags length = %d, want %d", len(flags), len(want))
	}
	for i, expected := range want {
		if flags[i] != expected {
			t.Fatalf("transformed flags[%d] = %v, want %v", i, flags[i], expected)
		}
	}
}
