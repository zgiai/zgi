package agents

import (
	"testing"

	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	approvalruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/approval"
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

func TestNormalizeAgentWorkflowQuestionInputsPreservesQuestionIdentity(t *testing.T) {
	inputs := normalizeAgentWorkflowQuestionInputs(map[string]interface{}{
		"query":                     "answer",
		"question_answer_option_id": "option-2",
		"question_answer_node_id":   "question-node-2",
		"question_answer_round":     3,
	})

	if inputs["query"] != "answer" || inputs["sys.query"] != "answer" {
		t.Fatalf("normalized query = %#v, want query and sys.query", inputs)
	}
	if inputs["question_answer_option_id"] != "option-2" {
		t.Fatalf("normalized option = %#v, want option-2", inputs["question_answer_option_id"])
	}
	if inputs["question_answer_node_id"] != "question-node-2" {
		t.Fatalf("normalized node = %#v, want question-node-2", inputs["question_answer_node_id"])
	}
	if inputs["question_answer_round"] != 3 {
		t.Fatalf("normalized round = %#v, want 3", inputs["question_answer_round"])
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

func TestEmitAgentWorkflowInteractionEventsRelaysApprovalBeforeQuestionProjection(t *testing.T) {
	generation := int64(2)
	var received []struct {
		event   string
		payload map[string]interface{}
	}
	err := emitAgentWorkflowInteractionEvents(&workflowpause.RunEventPayload{
		EventID: "approval-event", Sequence: 10, Event: workflowpause.EventApprovalResultFilled,
		PauseID: "pause-1", PauseGeneration: &generation,
		Data: map[string]interface{}{"node_id": "approval-node", "action": "approve"},
	}, []*workflowpause.RunEventPayload{
		{
			EventID: "question-event", Sequence: 11, Event: workflowpause.EventQuestionAnswerRequested,
			PauseID: "pause-1", PauseGeneration: &generation,
			Data: map[string]interface{}{"node_id": "question-node", "question": "Continue?"},
		},
		{
			EventID: "paused-event", Sequence: 12, Event: workflowpause.EventWorkflowPaused,
			PauseID: "pause-1", PauseGeneration: &generation,
			Data: map[string]interface{}{"status": "paused"},
		},
	}, func(event string, payload map[string]interface{}) error {
		received = append(received, struct {
			event   string
			payload map[string]interface{}
		}{event: event, payload: payload})
		return nil
	})
	if err != nil {
		t.Fatalf("emit interaction events: %v", err)
	}
	if len(received) != 3 {
		t.Fatalf("received event count = %d, want 3", len(received))
	}
	if received[0].event != workflowpause.EventApprovalResultFilled {
		t.Fatalf("first event = %q, want approval_result_filled", received[0].event)
	}
	if received[0].payload["sequence"] != 10 || received[0].payload["event_id"] != "approval-event" {
		t.Fatalf("approval event envelope = %#v", received[0].payload)
	}
	if received[1].event != workflowpause.EventQuestionAnswerRequested {
		t.Fatalf("second event = %q, want question_answer_requested", received[1].event)
	}
	if received[1].payload["pause_generation"] != generation {
		t.Fatalf("pause generation = %#v, want %d", received[1].payload["pause_generation"], generation)
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

func TestApprovalSubmissionObservesRunningExecution(t *testing.T) {
	tests := []struct {
		name       string
		submission *approvalruntime.SubmitResult
		want       bool
	}{
		{name: "nil submission"},
		{name: "waiting", submission: &approvalruntime.SubmitResult{ResumeState: "waiting"}},
		{name: "queued", submission: &approvalruntime.SubmitResult{ResumeState: "queued"}},
		{name: "running", submission: &approvalruntime.SubmitResult{ResumeState: "running"}, want: true},
		{name: "normalized running", submission: &approvalruntime.SubmitResult{ResumeState: " RUNNING "}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := approvalSubmissionObservesRunningExecution(tt.submission); got != tt.want {
				t.Fatalf("approvalSubmissionObservesRunningExecution() = %v, want %v", got, tt.want)
			}
		})
	}
}
