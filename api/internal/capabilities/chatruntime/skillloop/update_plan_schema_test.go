package skillloop

import (
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestUpdatePlanToolsForRuntimeStateHidesPhaseUpdatesWithoutPlan(t *testing.T) {
	original := updatePlanSchemaTestTool()
	projected := updatePlanToolsForRuntimeState([]adapter.Tool{original}, map[string]interface{}{})
	properties := evidenceMapFromAny(evidenceMapFromAny(projected[0].Function.Parameters)["properties"])
	if _, exists := properties["phase_updates"]; exists {
		t.Fatal("phase_updates remained visible without an existing plan")
	}
	for _, branch := range evidenceSliceFromAny(evidenceMapFromAny(projected[0].Function.Parameters)["oneOf"]) {
		for _, required := range evidenceStringSliceFromAny(evidenceMapFromAny(branch)["required"]) {
			if required == "phase_updates" {
				t.Fatal("phase_updates oneOf branch remained visible without an existing plan")
			}
		}
	}
	projectedSchema := evidenceMapFromAny(projected[0].Function.Parameters)
	if err := tools.ValidateJSONSchema(projectedSchema); err != nil {
		t.Fatalf("projected update_plan schema is invalid: %v", err)
	}
	for _, payload := range []map[string]interface{}{
		{"plan": []interface{}{}},
		{"outcomes": []interface{}{}},
	} {
		if err := tools.ValidateJSONSchemaValue(projectedSchema, payload); err != nil {
			t.Fatalf("projected update_plan schema rejected %#v: %v", payload, err)
		}
	}
	for _, payload := range []map[string]interface{}{
		{"phase_updates": []interface{}{}},
		{"plan": []interface{}{}, "outcomes": []interface{}{}},
	} {
		if err := tools.ValidateJSONSchemaValue(projectedSchema, payload); err == nil {
			t.Fatalf("projected update_plan schema accepted mutually invalid payload %#v", payload)
		}
	}
	originalProperties := evidenceMapFromAny(evidenceMapFromAny(original.Function.Parameters)["properties"])
	if _, exists := originalProperties["phase_updates"]; !exists {
		t.Fatal("runtime projection mutated the shared source schema")
	}
}

func TestUpdatePlanToolsForRuntimeStateKeepsPhaseUpdatesWithPlan(t *testing.T) {
	projected := updatePlanToolsForRuntimeState([]adapter.Tool{updatePlanSchemaTestTool()}, map[string]interface{}{
		"operation_plan": map[string]interface{}{"phases": []interface{}{map[string]interface{}{"id": "phase-1"}}},
	})
	properties := evidenceMapFromAny(evidenceMapFromAny(projected[0].Function.Parameters)["properties"])
	if _, exists := properties["phase_updates"]; !exists {
		t.Fatal("phase_updates was hidden despite an existing plan")
	}
	projectedSchema := evidenceMapFromAny(projected[0].Function.Parameters)
	if err := tools.ValidateJSONSchema(projectedSchema); err != nil {
		t.Fatalf("update_plan schema with an existing plan is invalid: %v", err)
	}
	if err := tools.ValidateJSONSchemaValue(projectedSchema, map[string]interface{}{
		"phase_updates": []interface{}{},
	}); err != nil {
		t.Fatalf("phase_updates was rejected with an existing plan: %v", err)
	}
	if err := tools.ValidateJSONSchemaValue(projectedSchema, map[string]interface{}{
		"plan": []interface{}{}, "phase_updates": []interface{}{},
	}); err == nil {
		t.Fatal("plan and phase_updates were accepted together despite oneOf")
	}
}

func updatePlanSchemaTestTool() adapter.Tool {
	return adapter.Tool{Type: "function", Function: adapter.Function{
		Name: skills.MetaToolUpdatePlan,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"plan":          map[string]interface{}{"type": "array"},
				"outcomes":      map[string]interface{}{"type": "array"},
				"phase_updates": map[string]interface{}{"type": "array"},
			},
			"oneOf": []interface{}{
				map[string]interface{}{"required": []string{"plan"}},
				map[string]interface{}{"required": []string{"outcomes"}},
				map[string]interface{}{"required": []string{"phase_updates"}},
			},
		},
	}}
}
