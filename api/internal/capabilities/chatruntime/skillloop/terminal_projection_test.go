package skillloop

import "testing"

func TestTerminalOnlyLatestSuccessfulInvocationUsesExecutionEvidenceOnly(t *testing.T) {
	execution := map[string]interface{}{
		"kind": "tool_call", "status": "success", "skill_id": "ordinary-skill", "tool_name": "write_file",
		"result": map[string]interface{}{"saved": true},
	}
	governance := map[string]interface{}{
		"kind": "tool_governance", "status": "allowed", "skill_id": "ordinary-skill", "tool_name": "write_file",
		"result": map[string]interface{}{"saved": true},
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
		"status": "success", "skill_id": "ordinary-skill", "tool_name": "write_file",
		"result": map[string]interface{}{"saved": true},
	}
	if got := terminalOnlyLatestSuccessfulInvocation([]map[string]interface{}{legacy}); len(got) == 0 {
		t.Fatal("legacy execution evidence without kind was rejected")
	}
}

func TestTerminalOnlyLatestSuccessfulInvocationRejectsUnprovenConnectorOutcomes(t *testing.T) {
	for _, status := range []string{"unknown", "partial", "failed", ""} {
		invocation := nativeOutcomeRecord(nativeRunnerOperation(), "completed", status)
		if got := terminalOnlyLatestSuccessfulInvocation([]map[string]interface{}{invocation}); len(got) != 0 {
			t.Fatalf("unproven outcome %q became success", status)
		}
	}
}
