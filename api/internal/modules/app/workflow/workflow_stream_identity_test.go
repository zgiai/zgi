package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
)

func TestRequiredWorkflowDataIDRejectsFallbackIdentities(t *testing.T) {
	want := uuid.New()
	got, err := requiredWorkflowDataID(map[string]any{"id": "  " + want.String() + "  "})
	if err != nil || got != want.String() {
		t.Fatalf("requiredWorkflowDataID() = %q, %v; want %q", got, err, want)
	}
	for _, workflowData := range []map[string]any{
		{},
		{"id": ""},
		{"id": "agent-id-fallback"},
		{"id": 42},
	} {
		if _, err := requiredWorkflowDataID(workflowData); err == nil {
			t.Fatalf("requiredWorkflowDataID(%#v) error = nil", workflowData)
		}
	}
}

func TestExecuteWorkflowStreamRejectsGraphAndRuntimeIdentityMismatch(t *testing.T) {
	selectedID := uuid.NewString()
	runtimeID := uuid.NewString()
	resultChan := make(chan *WorkflowStreamEvent, 1)
	errorChan := make(chan error, 1)
	doneChan := make(chan map[string]interface{}, 1)

	handler := &WorkflowHandler{}
	handler.executeWorkflowStream(
		context.Background(), uuid.NewString(), uuid.NewString(),
		&dto.DraftWorkflowRunRequest{Inputs: map[string]interface{}{}},
		uuid.NewString(), "task", uuid.NewString(), runtimeID,
		map[string]interface{}{}, 1, resultChan, errorChan, doneChan,
		false, "WORKFLOW", "web-app", map[string]any{"id": selectedID},
	)

	err := <-errorChan
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("executeWorkflowStream() error = %v, want identity mismatch", err)
	}
}
