package service

import (
	"fmt"
	"testing"
)

func TestSkillLoopRuntimeStateSnapshotLedgerKeepsEarlyPhaseEvidence(t *testing.T) {
	ledger := make([]interface{}, 0, 14)
	ledger = append(ledger, map[string]interface{}{
		"status":        "completed",
		"skill_id":      "file-generator",
		"tool_name":     "generate_file",
		"invocation_id": "runtime_id:generate-1",
	})
	for index := 0; index < 13; index++ {
		ledger = append(ledger, map[string]interface{}{
			"status":        "completed",
			"skill_id":      "test-skill",
			"tool_name":     "later_tool",
			"invocation_id": fmt.Sprintf("runtime_id:later-%d", index),
		})
	}

	got := skillLoopRuntimeStateSnapshotLedger(map[string]interface{}{
		"operation_plan": map[string]interface{}{
			operationPlanEvidenceLedgerKey: ledger,
		},
	})
	if len(got) != len(ledger) {
		t.Fatalf("completion evidence ledger length = %d, want %d", len(got), len(ledger))
	}
	if got[0]["skill_id"] != "file-generator" || got[0]["tool_name"] != "generate_file" {
		t.Fatalf("early phase evidence was dropped: %#v", got[0])
	}
}
