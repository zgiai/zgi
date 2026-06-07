package workflow

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
)

func TestValidateConversationVariablesAcceptsFrontendArrayValueTypes(t *testing.T) {
	variables := []dto.Variable{
		{ID: "var_1", Name: "string_arr", ValueType: "array[string]"},
		{ID: "var_2", Name: "number_arr", ValueType: "array[number]"},
		{ID: "var_3", Name: "object_arr", ValueType: "array[object]"},
		{ID: "var_4", Name: "boolean_arr", ValueType: "array[boolean]"},
	}

	if err := ValidateConversationVariables(variables); err != nil {
		t.Fatalf("expected frontend array value types to be valid, got %v", err)
	}
}

func TestValidateConversationVariableValueAcceptsFrontendArrayValueTypes(t *testing.T) {
	tests := []struct {
		name      string
		valueType string
		value     interface{}
	}{
		{name: "string array", valueType: "array[string]", value: []interface{}{"a", "b"}},
		{name: "number array", valueType: "array[number]", value: []interface{}{float64(1), float64(2)}},
		{name: "object array", valueType: "array[object]", value: []interface{}{map[string]interface{}{"ok": true}}},
		{name: "boolean array", valueType: "array[boolean]", value: []interface{}{true, false}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateConversationVariableValue(tt.valueType, tt.value, "items"); err != nil {
				t.Fatalf("expected value to be valid, got %v", err)
			}
		})
	}
}
