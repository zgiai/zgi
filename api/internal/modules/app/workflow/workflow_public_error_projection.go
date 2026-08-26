package workflow

import (
	"context"
	"net/http"
	"strings"

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
		data["message"] = workflowPublicFailureMessage
		data["error"] = workflowPublicFailurePayload()
	case workflowpause.EventWorkflowFinished:
		if workflowEventHasFailureStatus(data) {
			data["error"] = workflowPublicFailurePayload()
		}
	case "workflow_failed":
		data["message"] = workflowPublicFailureMessage
		data["error"] = workflowPublicFailurePayload()
	case workflowpause.EventNodeFinished,
		"iteration_completed", "iteration_failed",
		"loop_completed", "loop_failed":
		redactWorkflowExecutionEventError(data)
	case "workflow_snapshot":
		redactWorkflowSnapshotErrors(data)
	}
	return data
}

func workflowInvocationHidesFailureDetails(invokeFrom string) bool {
	switch strings.ToLower(strings.TrimSpace(invokeFrom)) {
	case string(InvokeFromWebApp), "app-run":
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
	status := strings.ToLower(strings.TrimSpace(workflowEventString(data["status"])))
	return status == "failed" || status == "error"
}

func workflowPublicFailurePayload() map[string]interface{} {
	return map[string]interface{}{"message": workflowPublicFailureMessage}
}

func redactWorkflowExecutionEventError(data map[string]interface{}) {
	if !workflowEventHasFailureStatus(data) {
		return
	}
	if _, exists := data["error"]; exists {
		data["error"] = workflowPublicFailureMessage
	}
}

func redactWorkflowSnapshotErrors(data map[string]interface{}) {
	if run, ok := data["workflow_run"].(map[string]interface{}); ok && workflowEventHasFailureStatus(run) {
		if _, exists := run["error"]; exists {
			run["error"] = workflowPublicFailureMessage
		}
	}

	switch nodes := data["nodes"].(type) {
	case []map[string]interface{}:
		for _, node := range nodes {
			redactWorkflowExecutionEventError(node)
		}
	case []interface{}:
		for _, item := range nodes {
			if node, ok := item.(map[string]interface{}); ok {
				redactWorkflowExecutionEventError(node)
			}
		}
	}
}
