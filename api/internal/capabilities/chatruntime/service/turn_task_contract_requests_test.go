package service

import "testing"

func TestTurnTaskContractRequestsPreferExplicitContractOverLegacyKeywords(t *testing.T) {
	legacyManagedCreateQuery := "save this generated file to file management"
	parts := &chatRequestParts{
		Query: legacyManagedCreateQuery,
		ModelTurnIntent: &AIChatModelTurnIntent{
			Intent: "answer_or_explain_console_context",
		},
	}
	if turnTaskContractRequestsManagedFileCreate(parts, nil, "") {
		t.Fatal("managed file create used legacy keywords despite explicit answer task contract")
	}

	metadata := map[string]interface{}{
		"turn_task_contract": map[string]interface{}{
			"intent_label": "save_generated_file_to_file_management",
		},
	}
	if !turnTaskContractRequestsManagedFileCreate(parts, metadata, "") {
		t.Fatal("managed file create did not honor metadata task contract")
	}
}
