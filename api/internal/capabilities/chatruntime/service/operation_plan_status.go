package service

import "strings"

func operationPlanIsTerminal(plan map[string]interface{}) bool {
	switch strings.ToLower(strings.TrimSpace(stringFromAny(plan["status"]))) {
	case operationPlanStatusCompleted:
		if outcomes := mapSliceFromAny(plan[operationPlanOutcomesKey]); len(outcomes) > 0 {
			return operationPlanOutcomesTerminal(outcomes)
		}
		return true
	default:
		return operationPlanIsTerminalFailure(plan)
	}
}
