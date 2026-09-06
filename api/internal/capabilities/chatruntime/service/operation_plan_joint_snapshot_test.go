package service

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

// Reproduces a mixed outcomes/plan update: the accepted plan's IDs and
// execution bindings must survive, rather than becoming unverified placeholders.
func jointSnapshotMetadata() map[string]interface{} {
	m := map[string]interface{}{}
	applyPlanUpdateTraceMetadata(m, skills.SkillTrace{Kind: "plan_update", Status: "success", Result: map[string]interface{}{
		"outcomes": []interface{}{
			map[string]interface{}{"id": "prepare", "goal": "Prepare input"},
			map[string]interface{}{"id": "compute", "goal": "Compute result"},
			map[string]interface{}{"id": "report", "goal": "Explain result", "depends_on": []string{"compute"}},
		},
		"plan": []interface{}{
			map[string]interface{}{"id": "prepare-step", "step": "Prepare", "status": "completed", "completion_mode": "tool", "expected_action": map[string]interface{}{"skill_id": "pipeline", "tool_name": "prepare"}},
			map[string]interface{}{"id": "compute-step", "step": "Compute", "status": "completed", "completion_mode": "tool", "expected_action": map[string]interface{}{"skill_id": "calculator", "tool_name": "run"}},
			map[string]interface{}{"id": "report-step", "step": "Explain", "status": "completed", "completion_mode": "final_answer"},
		},
	}})
	return m
}

func TestJointSnapshotOutcomeOnlyRevisionPreservesExecutionContract(t *testing.T) {
	m := jointSnapshotMetadata()
	plan := mapFromOperationContext(m["operation_plan"])
	applyPlanUpdateTraceMetadata(m, skills.SkillTrace{Kind: "plan_update", Status: "success", Result: map[string]interface{}{"outcomes": plan["outcomes"]}})
	phases := mapSliceFromAny(mapFromOperationContext(m["operation_plan"])["phases"])
	if phases[0]["id"] != "prepare-step" || len(mapFromOperationContext(phases[0]["expected_action"])) == 0 || phases[2]["completion_mode"] != "final_answer" {
		t.Fatalf("outcome-only update discarded phase contract: %#v", phases)
	}
}

func TestJointSnapshotTerminalFailureCannotBecomePass(t *testing.T) {
	for _, status := range []string{"failed", "cancelled", "stopped"} {
		m := jointSnapshotMetadata()
		plan := mapFromOperationContext(m["operation_plan"])
		plan["status"] = status
		plan["completion_verification"] = map[string]interface{}{"status": status}
		finalizeOperationPlanForCompletedResult(m)
		if mapFromOperationContext(m["completion_verification"])["status"] != status {
			t.Fatalf("terminal %s was overwritten", status)
		}
	}
}

// Optional operational replay: fixtures remain outside the repository. This
// exercises real recorded metadata without embedding customer data in tests.
func TestOperationPlanRecordedReplay(t *testing.T) {
	path := os.Getenv("ZGI_OPERATION_PLAN_REPLAY_FILE")
	if path == "" {
		t.Skip("no external replay fixture configured")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var actions []struct {
		Kind string                 `json:"kind"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(data, &actions); err != nil {
		t.Fatal(err)
	}
	m := map[string]interface{}{}
	for _, action := range actions {
		switch action.Kind {
		case "plan_update":
			applyPlanUpdateTraceMetadata(m, skills.SkillTrace{Kind: "plan_update", Status: "success", Result: action.Data})
		case "invocation":
			applyOperationPlanInvocationState(m, []map[string]interface{}{action.Data})
		default:
			t.Fatalf("unknown replay action %q", action.Kind)
		}
	}
	finalizeOperationPlanForCompletedResult(m)
	if path := os.Getenv("ZGI_OPERATION_PLAN_REPLAY_OUTPUT"); path != "" {
		data, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if mapFromOperationContext(m["completion_verification"])["status"] != "pass" {
		t.Fatal("recorded successful execution did not satisfy the preserved plan")
	}
}

func TestJointSnapshotPreservesBindingsAndReconcilesActualEvidence(t *testing.T) {
	for _, status := range []string{"success", "failed", "cancelled", "stopped", "pending", "partial"} {
		t.Run(status, func(t *testing.T) {
			m := jointSnapshotMetadata()
			plan := mapFromOperationContext(m["operation_plan"])
			phases := mapSliceFromAny(plan["phases"])
			if phases[0]["id"] != "prepare-step" || phases[0]["outcome_id"] != "prepare" || len(mapFromOperationContext(phases[0]["expected_action"])) == 0 {
				t.Fatalf("joint snapshot lost execution contract: %#v", phases)
			}
			applyOperationPlanInvocationState(m, []map[string]interface{}{
				{"kind": "tool_call", "status": "success", "runtime_id": "prepare-1", "skill_id": "pipeline", "tool_name": "prepare"},
				{"kind": "tool_call", "status": status, "runtime_id": "compute-1", "skill_id": "calculator", "tool_name": "run"},
			})
			finalizeOperationPlanForCompletedResult(m)
			got := mapFromOperationContext(m["completion_verification"])["status"]
			if (got == "pass") != (status == "success") {
				t.Fatalf("execution %s: verification %v", status, got)
			}
		})
	}
}

func TestJointSnapshotClaimsDoNotReplaceEvidenceOrStrongAcceptance(t *testing.T) {
	for _, scenario := range []string{"no-evidence", "wrong-tool", "wrong-target", "strong-effect"} {
		t.Run(scenario, func(t *testing.T) {
			m := jointSnapshotMetadata()
			plan := mapFromOperationContext(m["operation_plan"])
			phases := mapSliceFromAny(plan["phases"])
			if scenario == "wrong-target" {
				mapFromOperationContext(phases[1]["expected_action"])["target"] = map[string]interface{}{"file_id": "required-file"}
			}
			if scenario == "strong-effect" {
				outcomes := mapSliceFromAny(plan["outcomes"])
				outcomes[1]["acceptance"] = map[string]interface{}{"effects": []interface{}{map[string]interface{}{"type": "file.persisted", "resource_id": "required-file"}}}
				plan["outcomes"] = mapsToInterfaceSlice(outcomes)
			}
			if scenario != "no-evidence" {
				tool := "run"
				if scenario == "wrong-tool" {
					tool = "other"
				}
				applyOperationPlanInvocationState(m, []map[string]interface{}{
					{"kind": "tool_call", "status": "success", "runtime_id": "prepare-1", "skill_id": "pipeline", "tool_name": "prepare"},
					{"kind": "tool_call", "status": "success", "runtime_id": "compute-1", "skill_id": "calculator", "tool_name": tool},
				})
			}
			finalizeOperationPlanForCompletedResult(m)
			if mapFromOperationContext(m["completion_verification"])["status"] == "pass" {
				t.Fatalf("%s must not pass", scenario)
			}
		})
	}
}

func TestJointSnapshotMatchesAuthorizedTargetAndLatestAttempt(t *testing.T) {
	m := jointSnapshotMetadata()
	plan := mapFromOperationContext(m["operation_plan"])
	phases := mapSliceFromAny(plan["phases"])
	mapFromOperationContext(phases[1]["expected_action"])["target"] = map[string]interface{}{"binding_id": "bound-target"}
	applyOperationPlanInvocationState(m, []map[string]interface{}{
		{"kind": "tool_call", "status": "success", "runtime_id": "prepare-1", "skill_id": "pipeline", "tool_name": "prepare"},
		{"kind": "tool_call", "status": "success", "runtime_id": "compute-1", "skill_id": "calculator", "tool_name": "run", "arguments": map[string]interface{}{"operation_plan_target": map[string]interface{}{"binding_id": "bound-target"}}},
	})
	if mapSliceFromAny(mapFromOperationContext(m["operation_plan"])["outcomes"])[1]["status"] != "completed" {
		t.Fatal("authorized target did not match")
	}
	applyOperationPlanInvocationState(m, []map[string]interface{}{{"kind": "tool_call", "status": "failed", "runtime_id": "compute-2", "skill_id": "calculator", "tool_name": "run", "arguments": map[string]interface{}{"binding_id": "bound-target"}}})
	finalizeOperationPlanForCompletedResult(m)
	if mapFromOperationContext(m["completion_verification"])["status"] == "pass" {
		t.Fatal("older success concealed a later failed attempt")
	}
}

func TestJointSnapshotRepeatedToolNeedsUnambiguousPhaseCorrelation(t *testing.T) {
	for _, correlated := range []bool{false, true} {
		m := jointSnapshotMetadata()
		plan := mapFromOperationContext(m["operation_plan"])
		phases := mapSliceFromAny(plan["phases"])
		phases[0]["expected_action"] = map[string]interface{}{"skill_id": "calculator", "tool_name": "run"}
		plan["phases"] = mapsToInterfaceSlice(phases)
		invocations := []map[string]interface{}{}
		for i, id := range []string{"prepare-step", "compute-step"} {
			args := map[string]interface{}{}
			if correlated {
				args["plan_phase_id"] = id
			}
			invocations = append(invocations, map[string]interface{}{"kind": "tool_call", "status": "success", "runtime_id": id, "skill_id": "calculator", "tool_name": "run", "sequence": i + 1, "arguments": args})
		}
		applyOperationPlanInvocationState(m, invocations)
		finalizeOperationPlanForCompletedResult(m)
		if (mapFromOperationContext(m["completion_verification"])["status"] == "pass") != correlated {
			t.Fatalf("correlated=%v: invalid completion", correlated)
		}
	}
}

func TestStopPlanPreservesCompletedWorkAndCancelsRemainingWork(t *testing.T) {
	m := jointSnapshotMetadata()
	applyOperationPlanInvocationState(m, []map[string]interface{}{{"kind": "tool_call", "status": "success", "runtime_id": "prepare-1", "skill_id": "pipeline", "tool_name": "prepare"}})
	markOperationPlanStopped(m)
	p := mapFromOperationContext(m["operation_plan"])
	phases := mapSliceFromAny(p["phases"])
	if p["status"] != "cancelled" || mapFromOperationContext(m["completion_verification"])["status"] != "cancelled" {
		t.Fatal("stop did not persist cancellation")
	}
	if phases[0]["status"] != "completed" || phases[1]["status"] != "cancelled" || phases[2]["status"] != "cancelled" {
		t.Fatalf("wrong partial cancellation: %#v", phases)
	}
	finalizeOperationPlanForCompletedResult(m)
	if mapFromOperationContext(m["completion_verification"])["status"] != "cancelled" {
		t.Fatal("cancellation became pass")
	}
}

func TestJointSnapshotCanBindOnlyValidatedToolReferences(t *testing.T) {
	for _, valid := range []bool{true, false} {
		m := jointSnapshotMetadata()
		plan := mapFromOperationContext(m["operation_plan"])
		phases := mapSliceFromAny(plan["phases"])
		delete(phases[0], "expected_action")
		delete(phases[1], "expected_action")
		phases[0]["evidence_refs"] = []string{"tool:pipeline/prepare"}
		phases[1]["evidence_refs"] = []string{"tool:calculator/run"}
		if !valid {
			phases[1]["evidence_refs"] = []string{"invocation_id:made-up"}
		}
		plan["phases"] = mapsToInterfaceSlice(phases)
		applyOperationPlanInvocationState(m, []map[string]interface{}{
			{"kind": "tool_call", "status": "success", "runtime_id": "prepare-1", "skill_id": "pipeline", "tool_name": "prepare", "arguments": map[string]interface{}{"binding_id": "input-binding"}},
			{"kind": "tool_call", "status": "success", "runtime_id": "compute-1", "skill_id": "calculator", "tool_name": "run"},
		})
		finalizeOperationPlanForCompletedResult(m)
		if (mapFromOperationContext(m["completion_verification"])["status"] == "pass") != valid {
			t.Fatalf("reference validity %v not respected", valid)
		}
	}
}

func TestJointSnapshotExactReferenceKeepsItsOwnAuthorizedTarget(t *testing.T) {
	m := jointSnapshotMetadata()
	plan := mapFromOperationContext(m["operation_plan"])
	phases := mapSliceFromAny(plan["phases"])
	for i, id := range []string{"one", "two"} {
		delete(phases[i], "expected_action")
		phases[i]["evidence_refs"] = []string{"invocation_id:runtime_id:" + id}
	}
	plan["phases"] = mapsToInterfaceSlice(phases)
	applyOperationPlanInvocationState(m, []map[string]interface{}{
		{"kind": "tool_call", "status": "success", "runtime_id": "one", "skill_id": "calculator", "tool_name": "run", "arguments": map[string]interface{}{"binding_id": "first"}},
		{"kind": "tool_call", "status": "success", "runtime_id": "two", "skill_id": "calculator", "tool_name": "run", "arguments": map[string]interface{}{"binding_id": "second"}},
	})
	finalizeOperationPlanForCompletedResult(m)
	if mapFromOperationContext(m["completion_verification"])["status"] != "pass" {
		t.Fatal("distinct exact references did not reconcile")
	}
	phases = mapSliceFromAny(mapFromOperationContext(m["operation_plan"])["phases"])
	for i, target := range []string{"first", "second"} {
		if mapFromOperationContext(mapFromOperationContext(phases[i]["expected_action"])["target"])["binding_id"] != target {
			t.Fatal("reference took another invocation's target")
		}
	}
}

func TestUnboundToolPhaseRemainsUnverifiedWhenTerminalCallFails(t *testing.T) {
	m := jointSnapshotMetadata()
	plan := mapFromOperationContext(m["operation_plan"])
	phases := mapSliceFromAny(plan["phases"])
	delete(phases[1], "expected_action")
	plan["phases"] = mapsToInterfaceSlice(phases)
	applyOperationPlanInvocationState(m, []map[string]interface{}{
		{"kind": "tool_call", "status": "success", "runtime_id": "prepare-1", "skill_id": "pipeline", "tool_name": "prepare"},
		{"kind": "tool_call", "status": "failed", "runtime_id": "compute-1", "skill_id": "calculator", "tool_name": "run"},
	})
	finalizeOperationPlanForCompletedResult(m)
	plan = mapFromOperationContext(m["operation_plan"])
	if plan["status"] != "failed" || mapFromOperationContext(m["completion_verification"])["status"] != "failed" {
		t.Fatal("failed task remained active")
	}
	if mapSliceFromAny(plan["outcomes"])[1]["status"] != "pending" {
		t.Fatal("unbound failure must not fabricate outcome evidence")
	}
}
