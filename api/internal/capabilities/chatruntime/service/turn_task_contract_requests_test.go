package service

import "testing"

func TestTurnTaskContractRequestsPreferExplicitContractOverLegacyKeywords(t *testing.T) {
	legacyManagedCreateQuery := "save this generated file to file management"
	parts := &chatRequestParts{
		Query: legacyManagedCreateQuery,
		ModelTurnIntent: &AIChatModelTurnIntent{
			Intent: "answer_or_explain_zgi_context",
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

func TestTurnTaskContractRequestsUseOperationPlanContractBeforeModelIntent(t *testing.T) {
	parts := &chatRequestParts{
		Query: "explain what file management does",
		ModelTurnIntent: &AIChatModelTurnIntent{
			Intent: "answer_or_explain_zgi_context",
		},
	}
	metadata := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"task_contract": map[string]interface{}{
				"intent": "delete_visible_file",
			},
		},
	}
	if !turnTaskContractRequestsFileDelete(parts, metadata, "") {
		t.Fatal("file delete did not honor operation plan task contract")
	}
}

func TestTurnTaskContractRequestsAllowLegacyOnlyForContinuationOrMissingContract(t *testing.T) {
	query := "delete the first file"
	answerParts := &chatRequestParts{
		Query: query,
		ModelTurnIntent: &AIChatModelTurnIntent{
			Intent: "answer_or_explain_zgi_context",
		},
	}
	if turnTaskContractRequestsFileDelete(answerParts, nil, "") {
		t.Fatal("file delete used legacy keywords for explicit answer task contract")
	}

	continueParts := &chatRequestParts{
		Query: query,
		ModelTurnIntent: &AIChatModelTurnIntent{
			Intent: "continue_previous_task",
		},
	}
	if !turnTaskContractRequestsFileDelete(continueParts, nil, "") {
		t.Fatal("file delete did not allow legacy fallback for continuation task contract")
	}

	missingContractParts := &chatRequestParts{Query: query}
	if !turnTaskContractRequestsFileDelete(missingContractParts, nil, "") {
		t.Fatal("file delete did not allow compatibility fallback when no task contract exists")
	}
}

func TestTurnTaskContractRequestsUseCapabilitiesForArtifactAndReadNeeds(t *testing.T) {
	artifactParts := &chatRequestParts{
		Query: "make something useful",
		ModelTurnIntent: &AIChatModelTurnIntent{
			Intent:                  "answer_or_explain_zgi_context",
			RecommendedCapabilities: []string{"chart_artifact"},
		},
	}
	if !turnTaskContractRequestsTemporaryFileGenerate(artifactParts, nil, "") {
		t.Fatal("temporary artifact generation did not honor recommended capability")
	}

	readParts := &chatRequestParts{
		Query: "tell me about it",
		ModelTurnIntent: &AIChatModelTurnIntent{
			Intent:                  "answer_or_explain_zgi_context",
			RecommendedCapabilities: []string{"visible_file_content"},
		},
	}
	if !turnTaskContractRequestsFileRead(readParts, nil, "") {
		t.Fatal("file read did not honor recommended capability")
	}
}
