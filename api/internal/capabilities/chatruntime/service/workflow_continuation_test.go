package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

func TestWorkflowSummaryLLMRequestUsesOriginalMessageModelAndWorkflowOutputs(t *testing.T) {
	provider := "openai"
	message := &runtimemodel.Message{
		ModelProvider:   &provider,
		ModelName:       "gpt-4.1-mini",
		ModelParameters: map[string]interface{}{"temperature": 0.2, "max_tokens": 256},
	}
	continuation := &WorkflowApprovalContinuation{
		WorkflowRunID: "run-1",
		OriginalQuery: "请处理这个任务",
	}
	req := workflowSummaryLLMRequest(message, continuation, WorkflowContinuationSummaryRequest{
		Status:  "succeeded",
		Outputs: map[string]interface{}{"answer": "任务结果"},
	})

	if req.Provider != "openai" || req.Model != "gpt-4.1-mini" {
		t.Fatalf("request model = %s/%s, want openai/gpt-4.1-mini", req.Provider, req.Model)
	}
	if req.Temperature == nil || *req.Temperature != 0.2 {
		t.Fatalf("temperature = %#v, want 0.2", req.Temperature)
	}
	if req.MaxTokens == nil || *req.MaxTokens != 256 {
		t.Fatalf("max_tokens = %#v, want 256", req.MaxTokens)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(req.Messages))
	}
	systemPrompt, ok := req.Messages[0].Content.(string)
	if !ok {
		t.Fatalf("system content type = %T, want string", req.Messages[0].Content)
	}
	for _, want := range []string{"Use only the workflow outputs", "Do not answer the original user request yourself", "workflow outputs do not contain the requested business result"} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("summary system prompt missing %q:\n%s", want, systemPrompt)
		}
	}
	content, ok := req.Messages[1].Content.(string)
	if !ok {
		t.Fatalf("user content type = %T, want string", req.Messages[1].Content)
	}
	for _, want := range []string{"请处理这个任务", "run-1", "succeeded", "任务结果"} {
		if !strings.Contains(content, want) {
			t.Fatalf("summary prompt missing %q:\n%s", want, content)
		}
	}
}

func TestWorkflowContinuationMetadataWithStatusPreservesExistingFields(t *testing.T) {
	runID := uuid.NewString()
	metadata := workflowContinuationMetadataWithStatus(map[string]interface{}{
		"agent_workflow_continuation": map[string]interface{}{
			"workflow_run_id": runID,
			"binding_id":      "binding-1",
			"status":          "waiting_approval",
		},
	}, workflowContinuationStatusSummarizing)

	state := workflowRecordFromAny(metadata["agent_workflow_continuation"])
	if got := firstNonEmptyString(state["workflow_run_id"]); got != runID {
		t.Fatalf("workflow_run_id = %q, want %q", got, runID)
	}
	if got := firstNonEmptyString(state["binding_id"]); got != "binding-1" {
		t.Fatalf("binding_id = %q, want binding-1", got)
	}
	if got := firstNonEmptyString(state["status"]); got != workflowContinuationStatusSummarizing {
		t.Fatalf("status = %q, want %q", got, workflowContinuationStatusSummarizing)
	}
}

func TestValidateWorkflowContinuationBindingMatchesBindingAndAgent(t *testing.T) {
	continuation := &WorkflowApprovalContinuation{BindingID: "binding-1", AgentID: "agent-1"}
	bindings := []AgentWorkflowBinding{{BindingID: "binding-1", AgentID: "agent-other"}}
	if err := validateWorkflowContinuationBinding(continuation, bindings); !errors.Is(err, ErrWorkflowBindingUnavailable) {
		t.Fatalf("validateWorkflowContinuationBinding() agent mismatch error = %v, want ErrWorkflowBindingUnavailable", err)
	}
	bindings = append(bindings, AgentWorkflowBinding{BindingID: "binding-1", AgentID: "agent-1"})
	if err := validateWorkflowContinuationBinding(continuation, bindings); err != nil {
		t.Fatalf("validateWorkflowContinuationBinding() error = %v, want active match", err)
	}
}

func TestWorkflowContinuationAppendStreamEventAddsRuntimeIDs(t *testing.T) {
	conversationID := uuid.New()
	messageID := uuid.New()
	continuation := &WorkflowApprovalContinuation{
		ConversationID: conversationID,
		MessageID:      messageID,
		WorkflowRunID:  "workflow-run-1",
	}

	event, err := (&service{}).AppendWorkflowApprovalContinuationStreamEvent(context.Background(), continuation, "workflow_finished", map[string]interface{}{
		"status": "succeeded",
	})
	if err != nil {
		t.Fatalf("AppendWorkflowApprovalContinuationStreamEvent() error = %v", err)
	}
	if event == nil {
		t.Fatal("AppendWorkflowApprovalContinuationStreamEvent() returned nil event")
	}
	if event.EventType != "workflow_finished" {
		t.Fatalf("event type = %q, want workflow_finished", event.EventType)
	}
	if got := firstNonEmptyString(event.Payload["conversation_id"]); got != conversationID.String() {
		t.Fatalf("conversation_id = %q, want %q", got, conversationID.String())
	}
	if got := firstNonEmptyString(event.Payload["message_id"]); got != messageID.String() {
		t.Fatalf("message_id = %q, want %q", got, messageID.String())
	}
	if got := firstNonEmptyString(event.Payload["workflow_run_id"]); got != "workflow-run-1" {
		t.Fatalf("workflow_run_id = %q, want workflow-run-1", got)
	}
}

func TestTerminalStreamEventIgnoresWaitingMessageEnd(t *testing.T) {
	for _, status := range []string{
		runtimemodel.MessageStatusWaitingApproval,
		runtimemodel.MessageStatusWaitingQuestion,
		"",
	} {
		if isTerminalStreamEvent(StreamEvent{
			EventType: streamEventMessageEnd,
			Payload:   map[string]interface{}{"status": status},
		}) {
			t.Fatalf("message_end status %q should not be terminal", status)
		}
	}
	for _, status := range []string{
		runtimemodel.MessageStatusCompleted,
		runtimemodel.MessageStatusStopped,
		runtimemodel.MessageStatusError,
		"failed",
	} {
		if !isTerminalStreamEvent(StreamEvent{
			EventType: streamEventMessageEnd,
			Payload:   map[string]interface{}{"status": status},
		}) {
			t.Fatalf("message_end status %q should be terminal", status)
		}
	}
}

func TestWorkflowNoDisplayableOutputAnswer(t *testing.T) {
	got := workflowNoDisplayableOutputAnswer("run-empty")
	if !strings.Contains(got, "工作流已运行，但未返回可展示输出") || !strings.Contains(got, "run-empty") {
		t.Fatalf("workflowNoDisplayableOutputAnswer() = %q", got)
	}
}
