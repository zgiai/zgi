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
