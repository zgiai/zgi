package agents

import (
	"context"
	"errors"
	"strings"
	"testing"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
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
			name:      "failed task workflow resumes agent loop",
			agentType: "WORKFLOW",
			status:    "failed",
			outputs:   map[string]interface{}{"answer": "partial"},
			want:      true,
		},
		{
			name:      "failed delegated workflow resumes agent loop",
			agentType: "CONVERSATIONAL_WORKFLOW",
			status:    "failed",
			outputs:   map[string]interface{}{"answer": "partial"},
			want:      true,
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

func TestAgentWorkflowContinuationAnswerHidesPublishedFailureReason(t *testing.T) {
	detail := "node failed: private provider route"
	got := agentWorkflowContinuationAnswer(&runtimeservice.WorkflowApprovalContinuation{
		Caller: runtimeservice.Caller{Source: runtimemodel.ConversationSourceWebApp},
	}, "run-secret", "exception", nil, &detail)
	if got != "Workflow run failed." || strings.Contains(got, detail) || strings.Contains(got, "run-secret") {
		t.Fatalf("published failure answer = %q, want generic failure only", got)
	}

	debug := agentWorkflowContinuationAnswer(&runtimeservice.WorkflowApprovalContinuation{
		Caller: runtimeservice.Caller{Source: runtimemodel.ConversationSourceConsole},
	}, "run-debug", "failed", nil, &detail)
	if !strings.Contains(debug, detail) || !strings.Contains(debug, "run-debug") {
		t.Fatalf("debug failure answer = %q, want diagnostic detail", debug)
	}
}

func TestEmitAgentWorkflowContinuationEventProjectsPublishedFailure(t *testing.T) {
	detail := "append failed: storage secret"
	continuation := &runtimeservice.WorkflowApprovalContinuation{
		Caller: runtimeservice.Caller{Source: runtimemodel.ConversationSourceWebApp},
	}
	var received runtimeservice.StreamEvent
	emitAgentWorkflowContinuationEvent(continuation, func(event runtimeservice.StreamEvent) error {
		received = event
		return nil
	}, &runtimeservice.StreamEvent{EventType: "error", Payload: map[string]interface{}{"message": detail}})
	if strings.Contains(received.Payload["message"].(string), detail) {
		t.Fatalf("published continuation event exposed detail: %#v", received.Payload)
	}
}

func TestFailAgentWorkflowContinuationPersistsAndEmitsGenericPublishedFailure(t *testing.T) {
	detail := "workflow continuation stopped: database secret"
	runtimeService := &recordingWorkflowContinuationFailureService{}
	handler := &AgentsHandler{chatRuntimeService: runtimeService}
	continuation := &runtimeservice.WorkflowApprovalContinuation{
		WorkflowRunID: "run-1",
		Caller:        runtimeservice.Caller{Source: runtimemodel.ConversationSourceWebApp},
	}
	var events []runtimeservice.StreamEvent
	handler.failAgentWorkflowContinuation(t.Context(), continuation, errors.New(detail), func(event runtimeservice.StreamEvent) error {
		events = append(events, event)
		return nil
	})

	if runtimeService.failureMessage != "workflow run failed" {
		t.Fatalf("persisted failure message = %q, want generic", runtimeService.failureMessage)
	}
	if len(events) != 2 || strings.Contains(events[0].Payload["message"].(string), detail) {
		t.Fatalf("published failure events = %#v, want generic error and message_end", events)
	}
}

type recordingWorkflowContinuationFailureService struct {
	runtimeservice.Service
	failureMessage string
}

func (s *recordingWorkflowContinuationFailureService) FailWorkflowApprovalContinuation(_ context.Context, _ *runtimeservice.WorkflowApprovalContinuation, message string) (map[string]interface{}, error) {
	s.failureMessage = message
	return map[string]interface{}{"status": "failed"}, nil
}

func (s *recordingWorkflowContinuationFailureService) AppendWorkflowApprovalContinuationStreamEvent(_ context.Context, _ *runtimeservice.WorkflowApprovalContinuation, eventType string, payload map[string]interface{}) (*runtimeservice.StreamEvent, error) {
	return &runtimeservice.StreamEvent{EventType: eventType, Payload: payload}, nil
}

func TestCompletionContinuationStatus(t *testing.T) {
	if got := completionContinuationStatus("failed"); got != "failed" {
		t.Fatalf("completionContinuationStatus(failed) = %q, want failed", got)
	}
	if got := completionContinuationStatus("stopped"); got != "failed" {
		t.Fatalf("completionContinuationStatus(stopped) = %q, want failed", got)
	}
	if got := completionContinuationStatus("exception"); got != "failed" {
		t.Fatalf("completionContinuationStatus(exception) = %q, want failed", got)
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
	for _, status := range []string{"succeeded", "failed", "exception", "stopped", "partial-succeeded"} {
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

func TestApprovalSubmissionObservesExistingExecution(t *testing.T) {
	tests := []struct {
		name       string
		submission *approvalruntime.SubmitResult
		want       bool
	}{
		{name: "nil submission"},
		{name: "waiting", submission: &approvalruntime.SubmitResult{ResumeState: "waiting"}},
		{name: "queued", submission: &approvalruntime.SubmitResult{ResumeState: "queued"}},
		{name: "observe queued execution", submission: &approvalruntime.SubmitResult{ResumeState: "queued", ObserveExistingExecution: true}, want: true},
		{name: "running", submission: &approvalruntime.SubmitResult{ResumeState: "running"}, want: true},
		{name: "normalized running", submission: &approvalruntime.SubmitResult{ResumeState: " RUNNING "}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := approvalSubmissionObservesExistingExecution(tt.submission); got != tt.want {
				t.Fatalf("approvalSubmissionObservesExistingExecution() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResumeAgentWorkflowApprovalSubmissionObservesWithBaseRunner(t *testing.T) {
	runner := &fakeWorkflowContinuationRunner{}
	submission := &approvalruntime.SubmitResult{
		ResumeState:              "queued",
		ObserveExistingExecution: true,
	}

	if err := resumeAgentWorkflowApprovalSubmission(context.Background(), runner, submission); err != nil {
		t.Fatalf("observe existing execution: %v", err)
	}
	if runner.resumeApprovalCalled {
		t.Fatal("base workflow runner resumed an execution that should only be observed")
	}
}
