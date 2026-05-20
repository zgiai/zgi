package gateway

import "strings"

const (
	billingContextStatusSuccess = "success"
	billingContextStatusError   = "error"
	billingContextStatusPartial = "partial"
)

func normalizedBillingContextStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

func billingContextStatusIsSuccess(status string) bool {
	normalized := normalizedBillingContextStatus(status)
	return normalized == "" || normalized == billingContextStatusSuccess
}

func billingContextStatusIsPartial(status string) bool {
	return normalizedBillingContextStatus(status) == billingContextStatusPartial
}
