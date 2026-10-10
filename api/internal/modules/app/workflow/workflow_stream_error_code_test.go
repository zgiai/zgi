package workflow

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSendSSEErrorDataResolvesTheContractCode pins the SSE error frame: the
// product `code` stays the word list clients parse, while `error_code` is the
// catalog field and must never contradict it. The frame used to force
// llm.invocation.failed onto every payload that did not already carry one.
func TestSendSSEErrorDataResolvesTheContractCode(t *testing.T) {
	cases := []struct {
		name     string
		data     map[string]interface{}
		wantCode string
	}{
		{
			name:     "explicit catalog code is kept",
			data:     map[string]interface{}{"code": "workflow_conversation_busy", "error_code": "llm.rate_limit.exceeded"},
			wantCode: "llm.rate_limit.exceeded",
		},
		{
			name:     "nested catalog code is used",
			data:     map[string]interface{}{"code": "workflow_failed", "error": map[string]interface{}{"code": "billing.organization.balance_insufficient"}},
			wantCode: "billing.organization.balance_insufficient",
		},
		{
			name:     "product word is never published as the contract code",
			data:     map[string]interface{}{"code": "workflow_conversation_busy"},
			wantCode: "llm.invocation.failed",
		},
		{
			name:     "empty payload falls back",
			data:     map[string]interface{}{},
			wantCode: "llm.invocation.failed",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			handler := &WorkflowHandler{}
			recorder := httptest.NewRecorder()
			recorder.Header().Set("X-Request-ID", "req-workflow-sse")
			handler.sendSSEErrorData(context.Background(), recorder, testCase.data)

			body := strings.TrimSpace(recorder.Body.String())
			body = strings.TrimPrefix(body, "data: ")
			var event struct {
				Data map[string]interface{} `json:"data"`
			}
			if err := json.Unmarshal([]byte(body), &event); err != nil {
				t.Fatalf("decode sse frame: %v; raw = %s", err, body)
			}
			if event.Data["error_code"] != testCase.wantCode {
				t.Fatalf("error_code = %v, want %q", event.Data["error_code"], testCase.wantCode)
			}
			if _, ok := event.Data["error_id"]; !ok {
				t.Fatalf("frame has no error_id: %v", event.Data)
			}
			if event.Data["request_id"] != "req-workflow-sse" {
				t.Fatalf("request_id = %v, want req-workflow-sse", event.Data["request_id"])
			}
		})
	}
}

// TestWorkflowBusyFrameDoesNotContradictItself pins the recoverable
// conversation-busy outcome: a retryable catalog identity accompanies the
// product code instead of the generic invocation failure.
func TestWorkflowBusyFrameDoesNotContradictItself(t *testing.T) {
	if workflowBusyContractCode != "llm.rate_limit.exceeded" {
		t.Fatalf("workflowBusyContractCode = %q", workflowBusyContractCode)
	}
	retryable, known := workflowStreamErrorRetryable(workflowBusyContractCode)
	if !known || !retryable {
		t.Fatalf("busy contract code retryable = (%v, %v), want (true, true)", retryable, known)
	}
}
