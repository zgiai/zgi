package skillloop

import "testing"

func TestTerminalOnlyLatestSuccessfulInvocationUsesExecutionEvidenceOnly(t *testing.T) {
	execution := map[string]interface{}{
		"kind": "tool_call", "status": "success", "skill_id": "external-apps", "tool_name": "execute_action",
		"result": map[string]interface{}{"operation_status": "completed", "provider_success_confirmed": true},
	}
	governance := map[string]interface{}{
		"kind": "tool_governance", "status": "allowed", "skill_id": "external-apps", "tool_name": "execute_action",
		"result": map[string]interface{}{"operation_status": "completed", "provider_success_confirmed": true},
	}

	got := terminalOnlyLatestSuccessfulInvocation([]map[string]interface{}{execution, governance})
	if evidenceStringFromAny(got["kind"]) != "tool_call" {
		t.Fatalf("latest evidence kind = %q, want tool_call; evidence=%#v", evidenceStringFromAny(got["kind"]), got)
	}
	if got := terminalOnlyLatestSuccessfulInvocation([]map[string]interface{}{governance}); len(got) != 0 {
		t.Fatalf("governance-only evidence was projected as execution success: %#v", got)
	}
}

func TestTerminalOnlyLatestSuccessfulInvocationAcceptsLegacyKind(t *testing.T) {
	legacy := map[string]interface{}{
		"status": "success", "skill_id": "external-apps", "tool_name": "execute_action",
		"result": map[string]interface{}{"operation_status": "completed", "provider_success_confirmed": true},
	}
	if got := terminalOnlyLatestSuccessfulInvocation([]map[string]interface{}{legacy}); len(got) == 0 {
		t.Fatal("legacy execution evidence without kind was rejected")
	}
}

func TestTerminalOnlyLatestSuccessfulInvocationRejectsUnconfirmedExternalOutcomes(t *testing.T) {
	for _, operationStatus := range []string{"outcome_unknown", "partially_succeeded", "executing"} {
		t.Run(operationStatus, func(t *testing.T) {
			invocation := map[string]interface{}{
				"kind": "tool_call", "status": "success", "skill_id": "external-apps", "tool_name": "execute_action",
				"result": map[string]interface{}{
					"operation_status": operationStatus, "provider_success_confirmed": false,
				},
			}
			if got := terminalOnlyLatestSuccessfulInvocation([]map[string]interface{}{invocation}); len(got) != 0 {
				t.Fatalf("unconfirmed external outcome was projected as success: %#v", got)
			}
		})
	}
}
