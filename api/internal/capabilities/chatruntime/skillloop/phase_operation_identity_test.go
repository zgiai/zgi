package skillloop

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestProjectedExternalActionPhaseOperationIdentityIsCanonicalAndPhaseBound(t *testing.T) {
	connectionID := "connection-wecom"
	candidate := map[string]interface{}{
		"integration_id": "wecom", "action_id": "wecom.message.send", "binding_fingerprint": "binding-send",
		operationPlanServerProjectedConnectionBindingKey: skills.NativeExternalActionConnectionBindingHash(connectionID),
	}
	expected := func() map[string]interface{} {
		return map[string]interface{}{
			"skill_id": skills.SkillExternalApps, "tool_name": "execute_action",
			planExpectedActionServerBindingFingerprintKey: "binding-send",
			"target":           map[string]interface{}{"integration_id": "wecom", "action_id": "wecom.message.send"},
			"target_arguments": map[string]interface{}{"recipient_ref": "alice"},
		}
	}
	state := map[string]interface{}{
		runtimeStateNativeExternalActionCandidatesKey: []interface{}{candidate},
		"operation_plan": map[string]interface{}{"phases": []interface{}{
			map[string]interface{}{"id": "phase-first", "status": "in_progress", operationPlanServerProjectedLedgerEpochKey: "epoch-first", "expected_action": expected()},
			map[string]interface{}{"id": "phase-second", "status": "pending", operationPlanServerProjectedLedgerEpochKey: "epoch-second", "expected_action": expected()},
		}},
	}
	arguments := map[string]interface{}{
		"integration_id": "wecom", "action_id": "wecom.message.send", "connection_id": connectionID,
		"arguments": map[string]interface{}{"recipient_ref": "alice", "content": "first"},
	}
	first := projectedExternalActionPhaseOperationItemID(state, "phase-first", skills.SkillExternalApps, "execute_action", arguments)
	firstReplay := projectedExternalActionPhaseOperationItemID(state, "PHASE-FIRST", skills.SkillExternalApps, "execute_action", arguments)
	second := projectedExternalActionPhaseOperationItemID(state, "phase-second", skills.SkillExternalApps, "execute_action", arguments)
	if first == "" || firstReplay != first {
		t.Fatalf("canonical phase identity first=%q replay=%q", first, firstReplay)
	}
	if second == "" || second == first {
		t.Fatalf("distinct phases shared operation identity first=%q second=%q", first, second)
	}
	if spoofed := projectedExternalActionPhaseOperationItemID(state, "phase-unknown", skills.SkillExternalApps, "execute_action", arguments); spoofed != "" {
		t.Fatalf("unknown phase derived operation identity %q", spoofed)
	}
	arguments["connection_id"] = "connection-attacker"
	if spoofed := projectedExternalActionPhaseOperationItemID(state, "phase-first", skills.SkillExternalApps, "execute_action", arguments); spoofed != "" {
		t.Fatalf("cross-connection spoof derived operation identity %q", spoofed)
	}
}
