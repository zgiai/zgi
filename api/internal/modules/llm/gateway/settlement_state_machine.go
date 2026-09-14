package gateway

import "fmt"

// validSettlementStateTransition is shared by synchronous and reconciliation
// paths. Unknown/legacy states are never treated as success.
func validSettlementStateTransition(from, to string) bool {
	if from == to {
		return true
	}
	switch from {
	case "INIT":
		return to == "PENDING" || to == billingAttemptStatusPre
	case billingAttemptStatusPre:
		return to == billingAttemptStatusSettling || to == billingAttemptStatusSettled || to == billingAttemptStatusPartial || to == billingAttemptStatusFailedRetryable || to == billingAttemptStatusRolledBack
	case billingAttemptStatusSettlePending:
		return to == billingAttemptStatusSettling || to == billingAttemptStatusSettled || to == billingAttemptStatusPartial || to == billingAttemptStatusFailedRetryable || to == billingAttemptStatusRolledBack
	case billingAttemptStatusSettling:
		return to == billingAttemptStatusSettled || to == billingAttemptStatusPartial || to == billingAttemptStatusFailedRetryable || to == billingAttemptStatusFailedFinal
	case billingAttemptStatusPartial, billingAttemptStatusFailedRetryable:
		return to == billingAttemptStatusSettling || to == billingAttemptStatusFailedFinal
	default:
		return false
	}
}

func advanceSettlementState(from, to string) error {
	if !validSettlementStateTransition(from, to) {
		return fmt.Errorf("invalid settlement state transition %s -> %s", from, to)
	}
	return nil
}
