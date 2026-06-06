package service

import (
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

func TestWorkflowNoDisplayableOutputAnswer(t *testing.T) {
	got := workflowNoDisplayableOutputAnswer("run-empty")
	if !strings.Contains(got, "工作流已运行，但未返回可展示输出") || !strings.Contains(got, "run-empty") {
		t.Fatalf("workflowNoDisplayableOutputAnswer() = %q", got)
	}
}
