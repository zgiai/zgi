package service

import "strings"

func operationPlanIsTerminal(plan map[string]interface{}) bool {
	switch strings.ToLower(strings.TrimSpace(stringFromAny(plan["status"]))) {
	case operationPlanStatusCompleted:
		return true
	default:
		return operationPlanIsTerminalFailure(plan)
	}
}
