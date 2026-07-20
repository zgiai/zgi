package agents

import (
	"testing"

	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
)

func TestShouldSummarizeAgentWorkflowContinuation(t *testing.T) {
	tests := []struct {
		name      string
		agentType string
		status    string
		outputs   map[string]interface{}
		want      bool
	}{
		{
			name:      "task workflow with outputs",
			agentType: "WORKFLOW",
			status:    "succeeded",
			outputs:   map[string]interface{}{"answer": "done"},
			want:      true,
		},
		{
			name:      "conversational workflow direct output",
			agentType: "CONVERSATIONAL_WORKFLOW",
			status:    "succeeded",
			outputs:   map[string]interface{}{"answer": "done"},
			want:      false,
		},
		{
			name:      "task workflow without outputs",
			agentType: "WORKFLOW",
			status:    "succeeded",
			outputs:   map[string]interface{}{},
			want:      false,
		},
		{
			name:      "failed task workflow direct failure answer",
			agentType: "WORKFLOW",
			status:    "failed",
			outputs:   map[string]interface{}{"answer": "partial"},
			want:      false,
		},
		{
			name:      "stopped task workflow direct failure answer",
			agentType: "WORKFLOW",
			status:    "stopped",
			outputs:   map[string]interface{}{"answer": "partial"},
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSummarizeAgentWorkflowContinuation(&runtimeservice.WorkflowApprovalContinuation{AgentType: tt.agentType}, tt.status, tt.outputs)
			if got != tt.want {
				t.Fatalf("shouldSummarizeAgentWorkflowContinuation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompletionContinuationStatus(t *testing.T) {
	if got := completionContinuationStatus("failed"); got != "failed" {
		t.Fatalf("completionContinuationStatus(failed) = %q, want failed", got)
	}
	if got := completionContinuationStatus("stopped"); got != "failed" {
		t.Fatalf("completionContinuationStatus(stopped) = %q, want failed", got)
	}
	if got := completionContinuationStatus("succeeded"); got != "completed" {
		t.Fatalf("completionContinuationStatus(succeeded) = %q, want completed", got)
	}
}

func TestAgentWorkflowRunLogTerminal(t *testing.T) {
	for _, status := range []string{"succeeded", "failed", "stopped", "partial-succeeded"} {
		if !agentWorkflowRunLogTerminal(status) {
			t.Fatalf("agentWorkflowRunLogTerminal(%q) = false, want true", status)
		}
	}
	for _, status := range []string{"", "running", "paused"} {
		if agentWorkflowRunLogTerminal(status) {
			t.Fatalf("agentWorkflowRunLogTerminal(%q) = true, want false", status)
		}
	}
}

func TestAgentWorkflowContinuationMessageChunkPreservesWhitespace(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		payload map[string]interface{}
		want    string
	}{
		{name: "newline chunk", payload: map[string]interface{}{"answer": "\n"}, want: "\n"},
		{name: "surrounding whitespace", payload: map[string]interface{}{"text": "  hello\n"}, want: "  hello\n"},
		{name: "nested text", payload: map[string]interface{}{"data": map[string]interface{}{"text": "\nnext"}}, want: "\nnext"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := agentWorkflowContinuationMessageChunk(testCase.payload); got != testCase.want {
				t.Fatalf("agentWorkflowContinuationMessageChunk() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestAgentWorkflowContinuationFinalPassthroughAnswerPrefersWorkflowOutput(t *testing.T) {
	got, ok := agentWorkflowContinuationFinalPassthroughAnswer(
		map[string]interface{}{"answer": "first line\n\nsecond line\n"},
		"first linesecond line",
		true,
	)
	if !ok || got != "first line\n\nsecond line\n" {
		t.Fatalf("final passthrough answer = %q ok=%v, want authoritative workflow output", got, ok)
	}

	got, ok = agentWorkflowContinuationFinalPassthroughAnswer(nil, "fallback\nanswer", true)
	if !ok || got != "fallback\nanswer" {
		t.Fatalf("fallback passthrough answer = %q ok=%v, want streamed fallback", got, ok)
	}
}

func TestAgentWorkflowContinuationModePrefersPersistedInvocationMode(t *testing.T) {
	continuation := &runtimeservice.WorkflowApprovalContinuation{
		AgentType:      "CONVERSATIONAL_WORKFLOW",
		InvocationMode: "agent_task_tool",
	}
	if got := agentWorkflowContinuationMode(continuation); got != "agent_task_tool" {
		t.Fatalf("agentWorkflowContinuationMode() = %q, want persisted task mode", got)
	}
	if isAgentWorkflowPassthroughMessageEvent("message", continuation) {
		t.Fatal("task workflow answer transport must not be projected as the agent answer")
	}
}

func TestAgentWorkflowContinuationRelaysWorkflowResumed(t *testing.T) {
	if got := agentWorkflowContinuationEventType(workflowpause.EventWorkflowResumed); got != workflowpause.EventWorkflowResumed {
		t.Fatalf("agentWorkflowContinuationEventType(workflow_resumed) = %q", got)
	}
}

func TestAgentWorkflowContinuationStreamStateAppliesReplacement(t *testing.T) {
	state := &agentWorkflowContinuationStreamState{}
	state.apply(agentWorkflowContinuationDrainResult{
		HasWorkflowMessage:  true,
		WorkflowMessageText: "old answer",
	})
	state.apply(agentWorkflowContinuationDrainResult{
		HasWorkflowMessage:     true,
		WorkflowMessageReplace: true,
		WorkflowMessageText:    "authoritative answer\n",
	})

	if state.WorkflowMessageText != "authoritative answer\n" {
		t.Fatalf("replacement answer = %q", state.WorkflowMessageText)
	}
}
