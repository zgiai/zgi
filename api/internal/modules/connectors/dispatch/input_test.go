package dispatch

import (
	"encoding/json"
	"testing"
)

func TestInputDefaultsCanonicalizeOptionalInputOnly(t *testing.T) {
	schema := map[string]any{"required": []any{"query"}, "properties": map[string]any{
		"query":   map[string]any{"default": "must not fill required"},
		"mode":    map[string]any{"default": "auto"},
		"private": map[string]any{"readOnly": true, "default": "server value"},
	}}
	input := map[string]any{"query": "query"}
	implicit := InputWithDefaults(schema, input)
	explicit := InputWithDefaults(schema, map[string]any{"query": "query", "mode": "auto"})
	left, _ := json.Marshal(implicit)
	right, _ := json.Marshal(explicit)
	if string(left) != string(right) || input["mode"] != nil || implicit["private"] != nil {
		t.Fatal("defaults changed identity or caller input")
	}
	missing := InputWithDefaults(schema, nil)
	if missing["query"] != nil {
		t.Fatal("default fabricated required input")
	}
	if explicitNull := InputWithDefaults(schema, map[string]any{"mode": nil}); explicitNull["mode"] != nil {
		t.Fatal("explicit null replaced")
	}
}
