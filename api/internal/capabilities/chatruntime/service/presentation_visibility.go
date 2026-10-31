package service

import "strings"

// presentationVisibilityInternal marks a record the client keeps but does not
// show as a timeline row (ADR 0050). The client decides visibility from this
// field instead of from skill, tool or action names.
const presentationVisibilityInternal = "internal"

// invocationPresentationVisibility returns how the client presents a
// recorded invocation. An asset observation only records the effect of the
// call that changed the asset, which the person already sees as that call; a
// DigitalHuman task decision is marked where it is recorded, because its own
// workflow card presents it.
func invocationPresentationVisibility(invocation map[string]interface{}) string {
	if visibility := strings.TrimSpace(stringFromAny(invocation["presentation_visibility"])); visibility != "" {
		return visibility
	}
	if isAssetObservationClientAction(invocation) {
		return presentationVisibilityInternal
	}
	return ""
}

func isAssetObservationClientAction(invocation map[string]interface{}) bool {
	if !strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["kind"])), "client_action") {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["action_type"])), "asset_observation") {
		return true
	}
	actionID := strings.ToLower(strings.TrimSpace(stringFromAny(invocation["action_id"])))
	runtimeID := strings.ToLower(strings.TrimSpace(stringFromAny(invocation["runtime_id"])))
	return strings.HasPrefix(actionID, "asset_observation:") ||
		strings.HasPrefix(runtimeID, "client_action:asset_observation:")
}

// annotatePresentationVisibility sets the field on stored invocations, so
// messages recorded before the field existed read the same as new ones.
func annotatePresentationVisibility(metadata map[string]interface{}) {
	invocations := mapSliceFromAny(metadata["skill_invocations"])
	if len(invocations) == 0 {
		return
	}
	changed := false
	for _, invocation := range invocations {
		if invocation["presentation_visibility"] != nil {
			continue
		}
		if visibility := invocationPresentationVisibility(invocation); visibility != "" {
			invocation["presentation_visibility"] = visibility
			changed = true
		}
	}
	if changed {
		metadata["skill_invocations"] = skillInvocationsToInterfaceSlice(invocations)
	}
}
