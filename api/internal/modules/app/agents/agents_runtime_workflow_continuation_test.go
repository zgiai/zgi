package agents

import "testing"

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
			got := shouldSummarizeAgentWorkflowContinuation(tt.agentType, tt.status, tt.outputs)
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
