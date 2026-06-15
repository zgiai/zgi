package entities

import "testing"

func TestConvertValueAcceptsFrontendArrayValueTypes(t *testing.T) {
	tests := []struct {
		name      string
		valueType string
		value     any
		wantType  string
	}{
		{name: "string array", valueType: "array[string]", value: []any{"a", "b"}, wantType: "array[string]"},
		{name: "number array", valueType: "array[number]", value: []any{float64(1), float64(2)}, wantType: "array[number]"},
		{name: "object array", valueType: "array[object]", value: []any{map[string]any{"ok": true}}, wantType: "array[object]"},
		{name: "boolean array", valueType: "array[boolean]", value: []any{true, false}, wantType: "array[boolean]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			segment, _, err := ConvertValue(tt.valueType, tt.value, ValueConversionStrict)
			if err != nil {
				t.Fatalf("expected conversion to succeed, got %v", err)
			}
			if got := string(segment.GetType()); got != tt.wantType {
				t.Fatalf("expected segment type %q, got %q", tt.wantType, got)
			}
		})
	}
}
