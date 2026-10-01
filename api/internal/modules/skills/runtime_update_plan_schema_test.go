package skills

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestUpdatePlanSchemaRequiresExactlyOnePayloadForm(t *testing.T) {
	schema, ok := updatePlanMetaTool().Function.Parameters.(map[string]interface{})
	if !ok {
		t.Fatalf("update_plan parameters type = %T, want map", updatePlanMetaTool().Function.Parameters)
	}
	if err := tools.ValidateJSONSchema(schema); err != nil {
		t.Fatalf("ValidateJSONSchema() error = %v", err)
	}

	valid := []map[string]interface{}{
		{"plan": []interface{}{map[string]interface{}{"step": "Run action", "status": "in_progress"}}},
		{"outcomes": []interface{}{map[string]interface{}{"goal": "Run action"}}},
		{"phase_updates": []interface{}{map[string]interface{}{"id": "phase-1", "status": "completed"}}},
	}
	for index, value := range valid {
		if err := tools.ValidateJSONSchemaValue(schema, value); err != nil {
			t.Fatalf("valid payload %d rejected: %v", index, err)
		}
	}

	invalid := []map[string]interface{}{
		{},
		{
			"plan":     []interface{}{map[string]interface{}{"step": "Run action", "status": "in_progress"}},
			"outcomes": []interface{}{map[string]interface{}{"goal": "Run action"}},
		},
		{
			"outcomes":      []interface{}{map[string]interface{}{"goal": "Run action"}},
			"phase_updates": []interface{}{map[string]interface{}{"id": "phase-1"}},
		},
		{"outcomes": []interface{}{map[string]interface{}{"goal": "Run action", "unexpected": true}}},
		{"plan": []interface{}{map[string]interface{}{"step": "Run action", "status": "pending"}}, "unexpected": true},
	}
	for index, value := range invalid {
		if err := tools.ValidateJSONSchemaValue(schema, value); err == nil {
			t.Fatalf("invalid payload %d was accepted: %#v", index, value)
		}
	}
}
