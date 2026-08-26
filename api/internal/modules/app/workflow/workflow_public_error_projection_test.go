package workflow

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
)

func TestPublicWorkflowSSEFailurePayloadsAreRedacted(t *testing.T) {
	const privateDetail = "node llm-1 failed: upstream provider secret"
	tests := []struct {
		name      string
		eventType string
		data      map[string]interface{}
	}{
		{
			name:      "error event",
			eventType: workflowpause.EventError,
			data: map[string]interface{}{
				"message": privateDetail,
				"error": map[string]interface{}{
					"message": privateDetail,
					"code":    "private_provider_failure",
					"params":  map[string]interface{}{"route": "private-route"},
				},
			},
		},
		{
			name:      "workflow finished event",
			eventType: workflowpause.EventWorkflowFinished,
			data: map[string]interface{}{
				"status": "failed",
				"error":  map[string]interface{}{"message": privateDetail},
			},
		},
		{
			name:      "node finished event",
			eventType: workflowpause.EventNodeFinished,
			data: map[string]interface{}{
				"status": "failed",
				"error":  privateDetail,
			},
		},
		{
			name:      "failed iteration completion",
			eventType: "iteration_completed",
			data: map[string]interface{}{
				"status": "failed",
				"error":  privateDetail,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			sendWorkflowSSEStoredEventForInvocation(t.Context(), recorder, string(InvokeFromWebApp), workflowpause.RunEventPayload{
				Sequence: 1,
				Event:    tt.eventType,
				Data:     tt.data,
			})

			body := recorder.Body.String()
			if strings.Contains(body, privateDetail) || strings.Contains(body, "private_provider_failure") || strings.Contains(body, "private-route") {
				t.Fatalf("public SSE payload leaked private failure detail: %s", body)
			}
			if !strings.Contains(body, workflowPublicFailureMessage) {
				t.Fatalf("public SSE payload = %s, want generic failure message %q", body, workflowPublicFailureMessage)
			}
		})
	}
}

func TestDebugWorkflowSSEFailurePayloadKeepsDiagnosticDetail(t *testing.T) {
	const privateDetail = "debug-only provider failure"
	recorder := httptest.NewRecorder()

	sendWorkflowSSEStoredEventForInvocation(t.Context(), recorder, "debugging", workflowpause.RunEventPayload{
		Sequence: 1,
		Event:    workflowpause.EventError,
		Data: map[string]interface{}{
			"message": privateDetail,
			"error":   map[string]interface{}{"message": privateDetail},
		},
	})

	if body := recorder.Body.String(); !strings.Contains(body, privateDetail) {
		t.Fatalf("debug SSE payload = %s, want diagnostic detail %q", body, privateDetail)
	}
}

func TestPublicWorkflowSnapshotRedactsErrorsWithoutMutatingStoredData(t *testing.T) {
	const privateDetail = "persisted internal workflow error"
	input := map[string]interface{}{
		"workflow_run": map[string]interface{}{
			"status": "failed",
			"error":  privateDetail,
		},
		"nodes": []map[string]interface{}{
			{"status": "failed", "error": privateDetail},
		},
	}

	projected := projectWorkflowEventDataForInvocation(string(InvokeFromWebApp), "workflow_snapshot", input)
	projectedRun := projected["workflow_run"].(map[string]interface{})
	if projectedRun["error"] != workflowPublicFailureMessage {
		t.Fatalf("projected workflow error = %#v, want %q", projectedRun["error"], workflowPublicFailureMessage)
	}
	projectedNodes := projected["nodes"].([]map[string]interface{})
	if projectedNodes[0]["error"] != workflowPublicFailureMessage {
		t.Fatalf("projected node error = %#v, want %q", projectedNodes[0]["error"], workflowPublicFailureMessage)
	}

	storedRun := input["workflow_run"].(map[string]interface{})
	storedNodes := input["nodes"].([]map[string]interface{})
	if storedRun["error"] != privateDetail || storedNodes[0]["error"] != privateDetail {
		t.Fatalf("stored diagnostic detail was mutated: run=%#v nodes=%#v", storedRun, storedNodes)
	}
}

func TestWebAppDirectSSEErrorUsesGenericMessage(t *testing.T) {
	const privateDetail = "failed to create durable workflow run: database secret"
	recorder := httptest.NewRecorder()
	ctx := context.WithValue(t.Context(), "invoke_from", string(InvokeFromWebApp))

	(&WorkflowHandler{}).sendSSEError(ctx, recorder, privateDetail)

	body := recorder.Body.String()
	if strings.Contains(body, privateDetail) {
		t.Fatalf("public direct SSE error leaked private failure detail: %s", body)
	}
	if !strings.Contains(body, workflowPublicFailureMessage) {
		t.Fatalf("public direct SSE error = %s, want generic failure message %q", body, workflowPublicFailureMessage)
	}
}
