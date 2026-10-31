package service

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

// The plan stores the model's checklist, and a turn that ends with steps open
// keeps it running so the next turn sees the task as unfinished (ADR 0049).
func TestPlanChecklistIsStoredAndOpenStepsKeepItRunning(t *testing.T) {
	metadata := map[string]interface{}{"operation_plan": map[string]interface{}{"evidence_ledger": []interface{}{map[string]interface{}{"key": "kept"}}}}
	applyPlanUpdateTraceMetadata(metadata, skills.SkillTrace{Kind: "plan_update", Status: "success", Result: map[string]interface{}{
		"plan": []interface{}{
			map[string]interface{}{"id": "step-1", "step": "Compute", "status": "completed"},
			map[string]interface{}{"id": "step-2", "step": "Write the file", "status": "in_progress"},
		},
	}})
	plan := mapFromOperationContext(metadata["operation_plan"])
	if plan["version"] != operationPlanChecklistVersion || len(mapSliceFromAny(plan["phases"])) != 2 || plan["status"] != operationPlanStatusRunning || len(mapSliceFromAny(plan["evidence_ledger"])) != 1 {
		t.Fatalf("plan = %#v, want the checklist beside the kept ledger", plan)
	}

	applyOperationPlanTerminalCompletionResultWithSource(metadata, "completed", "main_model", "", nil, nil, "")
	plan = mapFromOperationContext(metadata["operation_plan"])
	verification := mapFromOperationContext(plan["completion_verification"])
	if plan["status"] != operationPlanStatusRunning || verification["status"] != "incomplete" || len(stringSliceFromAny(verification["missing_steps"])) != 1 {
		t.Fatalf("plan after an answer with an open step = %#v", plan)
	}
	if !operationPlanHasIncompleteWork(plan) {
		t.Fatal("an unfinished checklist is not open work")
	}

	applyPlanUpdateTraceMetadata(metadata, skills.SkillTrace{Kind: "plan_update", Status: "success", Result: map[string]interface{}{
		"plan": []interface{}{
			map[string]interface{}{"id": "step-1", "step": "Compute", "status": "completed"},
			map[string]interface{}{"id": "step-2", "step": "Write the file", "status": "skipped"},
		},
	}})
	applyOperationPlanTerminalCompletionResultWithSource(metadata, "completed", "main_model", "", nil, nil, "")
	if plan := mapFromOperationContext(metadata["operation_plan"]); plan["status"] != operationPlanStatusCompleted {
		t.Fatalf("plan after every step is done = %#v", plan)
	}
}

// A plan that only holds the runtime's ledgers is no open task, and the
// phases of an older plan, which nothing advances any more, do not keep a
// passed turn running.
func TestOnlyChecklistStepsKeepATurnOpen(t *testing.T) {
	ledgerOnly := map[string]interface{}{"version": operationPlanVersion, "status": operationPlanStatusRunning, "evidence_ledger": []interface{}{map[string]interface{}{"key": "kept"}}}
	if operationPlanHasIncompleteWork(ledgerOnly) {
		t.Fatal("a plan with ledgers and no steps was taken as open work")
	}

	metadata := map[string]interface{}{"operation_plan": map[string]interface{}{
		"version": operationPlanVersion,
		"phases":  []interface{}{map[string]interface{}{"id": "phase-1", "title": "Open the page", "status": "in_progress"}},
	}}
	applyOperationPlanTerminalCompletionResultWithSource(metadata, "completed", "main_model", "", nil, nil, "")
	if plan := mapFromOperationContext(metadata["operation_plan"]); plan["status"] != operationPlanStatusCompleted {
		t.Fatalf("older plan after a passed turn = %#v, want completed", plan)
	}
}
