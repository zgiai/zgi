package workflow

import (
	"testing"
	"time"

	"github.com/google/uuid"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

func TestAgentRuntimeRunProjectionsExposeErrorCode(t *testing.T) {
	errText := "The request completed but billing settlement failed."
	message := &runtimemodel.Message{
		ID:             uuid.New(),
		ConversationID: uuid.New(),
		Status:         runtimemodel.MessageStatusError,
		Error:          &errText,
		Metadata:       map[string]interface{}{"error_code": "billing.settle.failed"},
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	item := buildAgentRuntimeRunItem(message, nil, runtimemodel.ConversationSourceWebApp)
	detail := buildAgentRuntimeRunDetail(message, nil)
	if item.ErrorCode != "billing.settle.failed" || detail.ErrorCode != "billing.settle.failed" {
		t.Fatalf("error codes = item %q / detail %q, want billing.settle.failed", item.ErrorCode, detail.ErrorCode)
	}
	if item.Error != errText || detail.Error != errText {
		t.Fatalf("error text changed: item %q / detail %q", item.Error, detail.Error)
	}

	// Legacy numeric product codes and successful runs expose no catalog code.
	message.Metadata = map[string]interface{}{"error_code": 207011}
	if got := buildAgentRuntimeRunItem(message, nil, "").ErrorCode; got != "" {
		t.Fatalf("numeric legacy code exposed as %q", got)
	}
	message.Error = nil
	message.Metadata = map[string]interface{}{"error_code": "llm.invocation.failed"}
	if got := buildAgentRuntimeRunDetail(message, nil).ErrorCode; got != "" {
		t.Fatalf("successful run exposed error code %q", got)
	}
}
