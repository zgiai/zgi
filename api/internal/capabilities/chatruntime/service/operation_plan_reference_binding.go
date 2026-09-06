package service

import (
	"reflect"
	"strings"
)

func operationPlanEntryHasInvocation(entry map[string]interface{}, id string) bool {
	if id == stringFromAny(entry["invocation_id"]) {
		return true
	}
	for _, alias := range stringSliceFromAny(entry["invocation_aliases"]) {
		if id == alias {
			return true
		}
	}
	return false
}

// Compatibility plans may specify completed tool evidence without an upfront
// expected_action. Bind only references actually present in the runtime ledger;
// all references must describe the same action and authorized target. Narrative
// notes, model statuses, final-answer references and unknown IDs cannot bind it.
func operationPlanBindReferencedPhaseActions(plan map[string]interface{}) {
	phases := mapSliceFromAny(plan["phases"])
	ledger := mapSliceFromAny(plan[operationPlanEvidenceLedgerKey])
	attempts := mapSliceFromAny(plan[operationPlanActionAttemptsKey])
	for _, phase := range phases {
		derived := stringFromAny(phase["action_binding_source"]) == "runtime_evidence_reference"
		if len(mapFromOperationContext(phase["expected_action"])) > 0 && !derived {
			continue
		}
		if derived {
			delete(phase, "expected_action")
		}
		if stringFromAny(phase["completion_mode"]) != "tool" {
			continue
		}
		refs := stringSliceFromAny(phase["evidence_refs"])
		var binding map[string]interface{}
		valid := len(refs) > 0
		for _, ref := range refs {
			var candidate map[string]interface{}
			for i := len(ledger) - 1; i >= 0; i-- {
				entry := ledger[i]
				if stringFromAny(entry["kind"]) != "tool_call" || !operationPlanEvidenceEntrySucceeded(entry) {
					continue
				}
				toolRef := ref == operationPlanToolEvidenceKey(stringFromAny(entry["skill_id"]), stringFromAny(entry["tool_name"]))
				invocationRef := strings.TrimPrefix(ref, "invocation_id:")
				matches := toolRef || operationPlanEntryHasInvocation(entry, invocationRef)
				if !matches {
					continue
				}
				for index := len(attempts) - 1; index >= 0; index-- {
					attempt := attempts[index]
					if !toolRef && stringFromAny(attempt["id"]) != invocationRef {
						continue
					}
					if stringFromAny(attempt["kind"]) != "tool_call" || !operationPlanEntryHasInvocation(entry, stringFromAny(attempt["id"])) {
						continue
					}
					candidate = map[string]interface{}{"skill_id": entry["skill_id"], "tool_name": entry["tool_name"]}
					if target := mapFromOperationContext(attempt["target"]); len(target) > 0 {
						candidate["target"] = copyStringAnyMap(target)
					}
					break
				}
				break
			}
			if candidate == nil || (binding != nil && !reflect.DeepEqual(binding, candidate)) {
				valid = false
				break
			}
			binding = candidate
		}
		if valid && binding != nil {
			for _, outcome := range mapSliceFromAny(plan[operationPlanOutcomesKey]) {
				if stringFromAny(outcome["id"]) != stringFromAny(phase["outcome_id"]) {
					continue
				}
				if targetID := stringFromAny(outcome["target_resource_id"]); targetID != "" {
					matched := false
					for _, value := range mapFromOperationContext(binding["target"]) {
						matched = matched || stringFromAny(value) == targetID
					}
					valid = valid && matched
				}
			}
		}
		if valid && binding != nil {
			phase["expected_action"] = binding
			phase["action_binding_source"] = "runtime_evidence_reference"
		}
	}
	if len(phases) > 0 {
		plan["phases"] = mapsToInterfaceSlice(phases)
	}
}
