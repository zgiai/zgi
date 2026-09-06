package service

import "time"

func markOperationPlanStopped(metadata map[string]interface{}) {
	if metadata == nil {
		return
	}
	metadata["stopped"] = true
	plan := mapFromOperationContext(metadata["operation_plan"])
	if len(plan) == 0 {
		return
	}
	for _, key := range []string{"phases", operationPlanOutcomesKey} {
		items := mapSliceFromAny(plan[key])
		for _, item := range items {
			if operationPlanPhaseOpen(item) {
				item["status"] = "cancelled"
				item["completion_source"] = "runtime_stop"
				item["stopped_at"] = time.Now().UTC().Format(time.RFC3339)
			}
		}
		if len(items) > 0 {
			plan[key] = mapsToInterfaceSlice(items)
		}
	}
	plan["status"], plan["pending_next_action"] = "cancelled", "none"
	plan["completion_verification"] = map[string]interface{}{"status": "cancelled", "source": "runtime_stop", "reason": "execution was stopped before all required work completed"}
	operationPlanSyncStrategyState(plan)
	syncOperationPlanCompletionMetadata(metadata)
	if summary := mapFromOperationContext(metadata["operation_result_summary"]); len(summary) > 0 {
		summary["status"], summary["plan_status"], summary["pending_next_action"] = "cancelled", "cancelled", "none"
	}
}

// A normal final answer may explain a failed task. Preserve that distinction:
// the answer is delivered, but a required bound action failed in the runtime.
func operationPlanFinalizeFailedBindings(plan map[string]interface{}) bool {
	phases := mapSliceFromAny(plan["phases"])
	outcomes := mapSliceFromAny(plan[operationPlanOutcomesKey])
	attempts := mapSliceFromAny(plan[operationPlanActionAttemptsKey])
	failed := false
	for _, phase := range phases {
		required := true
		for _, outcome := range outcomes {
			if stringFromAny(outcome["id"]) == stringFromAny(phase["outcome_id"]) {
				if raw, exists := outcome["required"]; exists {
					required = operationPlanBoolValue(raw)
				}
			}
		}
		if !required {
			continue
		}
		for index := len(attempts) - 1; index >= 0; index-- {
			attempt := attempts[index]
			if !operationPlanPhaseAttemptMatches(plan, phase, attempt) {
				continue
			}
			if stringFromAny(attempt["status"]) == operationPlanStepStatusFailed {
				failed = true
				phase["status"], phase["completion_source"] = "failed", "runtime_action_failure"
				phase["failure_evidence_refs"] = []string{stringFromAny(attempt["id"])}
				for _, outcome := range outcomes {
					if stringFromAny(outcome["id"]) == stringFromAny(phase["outcome_id"]) {
						outcome["status"], outcome["failure_evidence_refs"] = "failed", phase["failure_evidence_refs"]
					}
				}
			}
			break
		}
	}
	if !failed {
		// A legacy tool phase may have no binding yet when the call fails.
		// Do not invent a successful association or blame a specific phase.
		// At terminal answer time, record the task-level failure if required
		// tool work is still unverified and the latest business call failed.
		unverifiedToolWork := false
		for _, phase := range phases {
			if !operationPlanPhaseOpen(phase) || stringFromAny(phase["completion_mode"]) != "tool" || len(mapFromOperationContext(phase["expected_action"])) > 0 {
				continue
			}
			required := true
			for _, outcome := range outcomes {
				if stringFromAny(outcome["id"]) == stringFromAny(phase["outcome_id"]) {
					if raw, exists := outcome["required"]; exists {
						required = operationPlanBoolValue(raw)
					}
				}
			}
			unverifiedToolWork = unverifiedToolWork || required
		}
		if unverifiedToolWork {
			for index := len(attempts) - 1; index >= 0; index-- {
				attempt := attempts[index]
				if stringFromAny(attempt["kind"]) != "tool_call" {
					continue
				}
				if stringFromAny(attempt["status"]) == operationPlanStepStatusFailed {
					plan["status"], plan["pending_next_action"] = "failed", "none"
					plan["completion_verification"] = map[string]interface{}{"status": "failed", "source": "runtime_action_failure", "reason": "required tool work remained unverified after a runtime action failed", "failure_evidence_refs": []string{stringFromAny(attempt["id"])}}
					operationPlanSyncStrategyState(plan)
					return true
				}
				break
			}
		}
		return false
	}
	plan["phases"] = mapsToInterfaceSlice(phases)
	if len(outcomes) > 0 {
		plan[operationPlanOutcomesKey] = mapsToInterfaceSlice(outcomes)
	}
	plan["status"], plan["pending_next_action"] = "failed", "none"
	plan["completion_verification"] = map[string]interface{}{"status": "failed", "source": "runtime_action_failure", "reason": "a required bound action failed; the final answer does not replace execution evidence"}
	operationPlanSyncStrategyState(plan)
	return true
}
