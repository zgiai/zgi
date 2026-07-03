package workflowtest

import "strings"

const (
	WorkflowTestTaskBackendLocal = "local"
	WorkflowTestTaskBackendAsynq = "asynq"
)

func NormalizeTaskBackend(value string) string {
	return normalizeTaskBackend(value)
}

func normalizeTaskBackend(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case WorkflowTestTaskBackendAsynq:
		return WorkflowTestTaskBackendAsynq
	default:
		return WorkflowTestTaskBackendLocal
	}
}
