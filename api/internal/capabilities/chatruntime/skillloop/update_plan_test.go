package skillloop

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestNormalizePlanSnapshotEnforcesStructuralProgressRules(t *testing.T) {
	_, err := normalizePlanSnapshot([]interface{}{
		map[string]interface{}{"id": "phase-1", "step": "First", "status": "in_progress"},
		map[string]interface{}{"id": "phase-2", "step": "Second", "status": "in_progress"},
	})
	if err == nil {
		t.Fatal("normalizePlanSnapshot() error = nil, want multiple in_progress rejection")
	}

	phasesWithoutEvidence, err := normalizePlanSnapshot([]interface{}{
		map[string]interface{}{"id": "phase-1", "step": "First", "status": "completed"},
		map[string]interface{}{"id": "phase-2", "step": "Optional cleanup", "status": "skipped"},
	})
	if err != nil || len(phasesWithoutEvidence) != 2 {
		t.Fatalf("normalizePlanSnapshot() = %#v, %v; want advisory phases accepted", phasesWithoutEvidence, err)
	}

	phases, err := normalizePlanSnapshot([]interface{}{
		map[string]interface{}{"id": "phase-1", "step": "Read file", "status": "completed", "evidence_refs": []interface{}{"file-reader/read_file"}},
		map[string]interface{}{"step": "Optional cleanup", "status": "skipped", "note": "Not requested"},
	})
	if err != nil {
		t.Fatalf("normalizePlanSnapshot() error = %v", err)
	}
	if phases[1]["id"] != "phase-amendment-1" {
		t.Fatalf("generated amendment ID = %#v, want phase-amendment-1", phases[1]["id"])
	}
	refs := evidenceStringSliceFromAny(phases[0]["evidence_refs"])
	if len(refs) != 1 || refs[0] != "tool:file-reader/read_file" {
		t.Fatalf("evidence_refs = %#v, want canonical tool ref", refs)
	}
}

func TestHandleUpdatePlanCallProducesPersistablePlanTrace(t *testing.T) {
	step := (&Runner{}).handleUpdatePlanCall("call-plan", map[string]interface{}{
		"explanation": "file read completed",
		"plan": []interface{}{map[string]interface{}{
			"id": "phase-1", "step": "Read file", "status": "completed", "evidence_refs": []interface{}{"tool:file-reader/read_file"},
		}},
	}, successfulReadFileEvidence())
	if step.fatalErr != nil || step.trace.Kind != "plan_update" || step.trace.Status != "success" {
		t.Fatalf("handleUpdatePlanCall() step = %#v", step)
	}
	if step.trace.ToolName != skills.MetaToolUpdatePlan {
		t.Fatalf("trace.ToolName = %q, want %q", step.trace.ToolName, skills.MetaToolUpdatePlan)
	}
}

func TestHandleUpdatePlanCallRecordsUnavailableEvidenceAsAuditWarning(t *testing.T) {
	step := (&Runner{}).handleUpdatePlanCall("call-plan", map[string]interface{}{
		"plan": []interface{}{map[string]interface{}{
			"id": "phase-1", "step": "Delete agent", "status": "completed", "evidence_refs": []interface{}{"agent-management/delete_agent"},
		}},
	}, successfulReadFileEvidence())
	if step.fatalErr != nil || step.recoverable || step.trace.Status != "success" {
		t.Fatalf("handleUpdatePlanCall() step = %#v, want successful advisory plan trace", step)
	}
	warnings := evidenceStringSliceFromAny(step.trace.Result["evidence_warnings"])
	if len(warnings) != 1 || warnings[0] != "unresolved_evidence_ref:tool:agent-management/delete_agent" {
		t.Fatalf("evidence_warnings = %#v", warnings)
	}
}

func successfulReadFileEvidence() map[string]interface{} {
	return map[string]interface{}{
		"evidence_ledger": []interface{}{map[string]interface{}{
			"status": "completed", "skill_id": "file-reader", "tool_name": "read_file", "invocation_id": "runtime_id:read-1",
		}},
	}
}
