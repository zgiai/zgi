package skills

import (
	"encoding/json"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestUpdatePlanSchemaIsAChecklist(t *testing.T) {
	schema, ok := updatePlanMetaTool().Function.Parameters.(map[string]interface{})
	if !ok {
		t.Fatalf("update_plan parameters type = %T, want map", updatePlanMetaTool().Function.Parameters)
	}
	if err := tools.ValidateJSONSchema(schema); err != nil {
		t.Fatalf("ValidateJSONSchema() error = %v", err)
	}
	valid := []map[string]interface{}{
		{"plan": []interface{}{map[string]interface{}{"step": "Run action", "status": "in_progress"}}},
		{"explanation": "added a step", "plan": []interface{}{
			map[string]interface{}{"step": "Read", "status": "completed"},
			map[string]interface{}{"step": "Write", "status": "pending"},
		}},
	}
	for index, value := range valid {
		if err := tools.ValidateJSONSchemaValue(schema, value); err != nil {
			t.Fatalf("valid payload %d rejected: %v", index, err)
		}
	}
	invalid := []map[string]interface{}{
		{},
		{"plan": []interface{}{}},
		{"plan": []interface{}{map[string]interface{}{"step": "Run action", "status": "done"}}},
		{"plan": []interface{}{map[string]interface{}{"step": "Run action", "status": "pending", "expected_action": map[string]interface{}{}}}},
		{"outcomes": []interface{}{map[string]interface{}{"goal": "Run action"}}},
	}
	for index, value := range invalid {
		if err := tools.ValidateJSONSchemaValue(schema, value); err == nil {
			t.Fatalf("invalid payload %d was accepted: %#v", index, value)
		}
	}
}

// The schema stays small: its descriptions say what each field is for, and
// the server's own bookkeeping is not explained to the model (ADR 0043).
func TestUpdatePlanToolStaysCompact(t *testing.T) {
	encoded, err := json.Marshal(updatePlanMetaTool())
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > 1200 {
		t.Fatalf("update_plan definition = %d bytes, want at most 1200", len(encoded))
	}
}
