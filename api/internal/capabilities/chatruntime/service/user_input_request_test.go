package service

import "testing"

func TestMergeUserInputRequestMetadataStoresHydratableRequest(t *testing.T) {
	metadata := mergeUserInputRequestMetadata(map[string]interface{}{"usage": map[string]interface{}{"total_tokens": 3}}, map[string]interface{}{
		"request_id": "call_ask",
		"created_at": int64(123),
		"questions": []map[string]interface{}{
			{
				"id":       "sheet",
				"question": "Which sheet should I process?",
				"options": []map[string]interface{}{
					{"label": "Water"},
					{"label": "Electricity", "description": "Use the electricity sheet"},
				},
			},
		},
	})

	if metadata["usage"] == nil {
		t.Fatalf("usage metadata missing: %#v", metadata)
	}
	request, ok := metadata["user_input_request"].(map[string]interface{})
	if !ok {
		t.Fatalf("user_input_request = %#v, want map", metadata["user_input_request"])
	}
	if request["request_id"] != "call_ask" {
		t.Fatalf("request = %#v, want id", request)
	}
	questions, ok := request["questions"].([]map[string]interface{})
	if !ok || len(questions) != 1 || questions[0]["question"] != "Which sheet should I process?" {
		t.Fatalf("questions = %#v, want persisted questions", request["questions"])
	}
	options, ok := questions[0]["options"].([]map[string]interface{})
	if !ok || len(options) != 2 || options[1]["description"] != "Use the electricity sheet" {
		t.Fatalf("options = %#v, want persisted options", questions[0]["options"])
	}
}
