package workflow

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/errors/failureprojection"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
)

const workflowPublicFailureMessage = "Workflow run failed."

func sendWorkflowSSEEventForInvocation(ctx context.Context, w http.ResponseWriter, invokeFrom, eventType string, data map[string]interface{}) {
	sendWorkflowSSEEvent(ctx, w, eventType, projectWorkflowEventDataForInvocation(invokeFrom, eventType, data))
}

func sendWorkflowSSEStoredEventForInvocation(ctx context.Context, w http.ResponseWriter, invokeFrom string, event workflowpause.RunEventPayload) {
	projected := event
	projected.Data = projectWorkflowEventDataForInvocation(invokeFrom, event.Event, event.Data)
	sendWorkflowSSEStoredEvent(ctx, w, projected)
}

func projectWorkflowEventDataForInvocation(invokeFrom, eventType string, input map[string]interface{}) map[string]interface{} {
	data := sanitizeWorkflowEventData(input)
	if !workflowInvocationHidesFailureDetails(invokeFrom) {
		return data
	}

	switch eventType {
	case workflowpause.EventError:
		data = failureprojection.ProjectPublicPayload(data, workflowPublicFailureMessage, true)
		data["message"] = workflowPublicFailureMessage
		data["error"] = workflowPublicFailurePayload()
	case workflowpause.EventWorkflowFinished:
		if workflowEventHasFailureStatus(data) {
			data = failureprojection.ProjectPublicPayload(data, workflowPublicFailureMessage, true)
			data["error"] = workflowPublicFailurePayload()
		}
	case "workflow_failed":
		data = failureprojection.ProjectPublicPayload(data, workflowPublicFailureMessage, true)
		data["message"] = workflowPublicFailureMessage
		data["error"] = workflowPublicFailurePayload()
	case workflowpause.EventNodeFinished,
		"iteration_completed", "loop_completed":
		data = projectWorkflowExecutionEventError(data)
		if workflowEventHasFailureStatus(data) {
			data = withWorkflowFailureMetadata(data)
		}
	case "iteration_failed", "loop_failed":
		data = failureprojection.ProjectPublicPayload(data, workflowPublicFailureMessage, true)
		data = withWorkflowFailureMetadata(data)
	case "workflow_snapshot":
		redactWorkflowSnapshotErrors(data)
	}
	return data
}

func withWorkflowFailureMetadata(data map[string]interface{}) map[string]interface{} {
	if data == nil {
		data = map[string]interface{}{}
	}
	data["error_code"] = "llm.invocation.failed"
	data["params"] = map[string]interface{}{}
	data["error_id"] = uuid.NewString()
	return data
}

func workflowInvocationHidesFailureDetails(invokeFrom string) bool {
	switch strings.ToLower(strings.TrimSpace(invokeFrom)) {
	case string(InvokeFromWebApp), string(InvokeFromExternalAPI), "app-run":
		return true
	default:
		return false
	}
}

func workflowEventProjectionInvokeFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	invokeFrom, _ := ctx.Value("invoke_from").(string)
	return invokeFrom
}

func workflowRunEventProjectionInvokeFrom(run *WorkflowRunLog) string {
	if run == nil {
		return ""
	}
	if run.CreatedByRole == CreatedByRoleEndUser || workflowInvocationHidesFailureDetails(run.TriggeredFrom) {
		return string(InvokeFromWebApp)
	}
	return run.TriggeredFrom
}

func workflowEventHasFailureStatus(data map[string]interface{}) bool {
	return failureprojection.IsFailureStatus(workflowEventString(data["status"]))
}

func workflowPublicFailurePayload() map[string]interface{} {
	return map[string]interface{}{
		"message":    workflowPublicFailureMessage,
		"code":       "llm.invocation.failed",
		"error_code": "llm.invocation.failed",
		"params":     map[string]interface{}{},
		"retryable":  true,
		"error_id":   uuid.NewString(),
	}
}

func projectWorkflowExecutionEventError(data map[string]interface{}) map[string]interface{} {
	if !workflowEventHasFailureStatus(data) {
		return data
	}
	return failureprojection.ProjectPublicPayload(data, workflowPublicFailureMessage, true)
}

func redactWorkflowSnapshotErrors(data map[string]interface{}) {
	if run, ok := data["workflow_run"].(map[string]interface{}); ok && workflowEventHasFailureStatus(run) {
		data["workflow_run"] = failureprojection.ProjectPublicPayload(run, workflowPublicFailureMessage, true)
	}

	switch nodes := data["nodes"].(type) {
	case []map[string]interface{}:
		for index, node := range nodes {
			nodes[index] = projectWorkflowExecutionEventError(node)
		}
	case []interface{}:
		for index, item := range nodes {
			if node, ok := item.(map[string]interface{}); ok {
				nodes[index] = projectWorkflowExecutionEventError(node)
			}
		}
	}
}
