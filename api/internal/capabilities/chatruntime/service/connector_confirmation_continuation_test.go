package service

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	connectorconfirmation "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeconfirmation"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestReplacePendingConnectorToolResultSwapsOnlyTheDecidedCall(t *testing.T) {
	receipt := uuid.NewString()
	request := &adapter.ChatRequest{Messages: []adapter.Message{
		{Role: "user", Content: "check nginx " + receipt},
		{Role: "tool", ToolCallID: "call_status", Content: `[{"type":"json","data":{"confirmation":{"route_receipt":"` + receipt + `"}}}]`},
		{Role: "tool", ToolCallID: "call_memory", Content: `{"status":"cancelled"}`},
	}}
	if !replacePendingConnectorToolResult(request, receipt, `[{"type":"json","data":{"active_state":"active"}}]`) {
		t.Fatal("pending tool result was not found")
	}
	if got := request.Messages[1].Content; got != `[{"type":"json","data":{"active_state":"active"}}]` {
		t.Fatalf("tool result = %v", got)
	}
	if request.Messages[0].Content != "check nginx "+receipt || request.Messages[2].Content != `{"status":"cancelled"}` {
		t.Fatalf("other messages changed: %#v", request.Messages)
	}
	if replacePendingConnectorToolResult(request, uuid.NewString(), "x") {
		t.Fatal("an unknown receipt replaced a tool result")
	}
}

func TestConnectorConfirmationModelToolResultMatchesAnUnconfirmedCall(t *testing.T) {
	completed := connectorConfirmationModelToolResult(connectorconfirmation.Result{
		State: connectorconfirmation.StateCompleted,
		ExecutionResult: &connectorexecution.ExecutionResult{
			State:   connectorexecution.ExecutionStateCompleted,
			Value:   map[string]any{"active_state": "active", "sub_state": "running"},
			Outcome: connectorcore.OperationOutcome{ID: uuid.New(), Status: connectorcore.OutcomeStatusSuccessful},
		},
		RequestIdentity: connectorconfirmation.ConfirmationRequestIdentity{
			ConnectorID: "serverops_ssh", OperationID: "server.service.status", LogicalOperationID: "op-1",
		},
	})
	for _, want := range []string{`"active_state":"active"`, `"sub_state":"running"`, `"confirmation_decision":"completed"`} {
		if !strings.Contains(completed, want) {
			t.Fatalf("completed result %s lacks %s", completed, want)
		}
	}

	cancelled := connectorConfirmationModelToolResult(connectorconfirmation.Result{State: connectorconfirmation.StateCancelled})
	if !strings.Contains(cancelled, `"state":"cancelled"`) || strings.Contains(cancelled, "active_state") {
		t.Fatalf("cancelled result = %s", cancelled)
	}
	failed := connectorConfirmationModelToolResult(connectorconfirmation.Result{
		State: connectorconfirmation.StateFailed, ErrorCode: connectorcore.ErrorCodeProviderUnavailable,
	})
	if !strings.Contains(failed, `"error_code":"connector_provider_unavailable"`) {
		t.Fatalf("failed result = %s", failed)
	}
}

func TestConnectorConfirmationContinuationMessageNamesTheOutcome(t *testing.T) {
	claim := &ConnectorConfirmationContinuation{
		State: connectorconfirmation.StateCancelled, Title: "Restart nginx",
		ToolResult: `[{"type":"json","data":{"state":"cancelled"}}]`,
	}
	message := connectorConfirmationContinuationMessage(nil, claim)
	content, _ := message.Content.(string)
	if message.Role != "system" || !strings.Contains(content, "cancelled Restart nginx") ||
		!strings.Contains(content, "Do not repeat the same connector call") ||
		!strings.Contains(content, "state: cancelled") {
		t.Fatalf("continuation message = %#v", message)
	}
	if got := boundedConnectorToolResult(strings.Repeat("x", maxConnectorContinuationResultRunes+5)); !strings.HasSuffix(got, "(truncated)") {
		t.Fatalf("unbounded result: %d runes", len([]rune(got)))
	}
}

func TestMarkDecidedConnectorToolResultsMarksEveryEarlierDecision(t *testing.T) {
	earlier, current := uuid.NewString(), uuid.NewString()
	request := &adapter.ChatRequest{Messages: []adapter.Message{
		{Role: "tool", ToolCallID: "call_snapshot", Content: `[{"type":"json","data":{"confirmation":{"route_receipt":"` + earlier + `"}}}]`},
		{Role: "tool", ToolCallID: "call_command", Content: `[{"type":"json","data":{"confirmation":{"route_receipt":"` + current + `"}}}]`},
		{Role: "tool", ToolCallID: "call_other", Content: `{"status":"success"}`},
	}}
	metadata := map[string]interface{}{connectorConfirmationEventsMetadataKey: []interface{}{
		map[string]interface{}{"event_type": streamEventConnectorConfirmationResolved, "route_receipt": earlier, "state": "completed"},
		map[string]interface{}{"event_type": streamEventConnectorConfirmationResolved, "route_receipt": current, "state": "completed"},
	}}
	markDecidedConnectorToolResults(request, metadata, current)
	replacePendingConnectorToolResult(request, current, `[{"type":"json","data":{"exit_code":0}}]`)

	first, _ := request.Messages[0].Content.(string)
	if strings.Contains(first, earlier) || !strings.Contains(first, `"state":"completed"`) || !strings.Contains(first, "continuation note") {
		t.Fatalf("earlier decided call = %s", first)
	}
	if request.Messages[1].Content != `[{"type":"json","data":{"exit_code":0}}]` || request.Messages[2].Content != `{"status":"success"}` {
		t.Fatalf("messages = %#v", request.Messages)
	}
}

func TestConnectorConfirmationNoteResultPutsTheOutcomeFirst(t *testing.T) {
	toolResult := `[{"type":"json","data":{"command":"curl -sS http://127.0.0.1:2670/health","confirmation_decision":"completed","exit_code":0,"operation":"server.command.run","provider":"serverops_ssh","stderr":"","stdout":"unhealthy\n","truncated":false}}]`
	note := connectorConfirmationNoteResult(toolResult)
	stdout, command := strings.Index(note, "stdout: unhealthy"), strings.Index(note, "command: curl")
	if stdout < 0 || command < 0 || stdout > command || strings.Contains(note, "provider") || strings.Contains(note, `"meta"`) {
		t.Fatalf("note = %q", note)
	}
	if got := connectorConfirmationNoteResult("not json"); got != "not json" {
		t.Fatalf("fallback = %q", got)
	}
}
