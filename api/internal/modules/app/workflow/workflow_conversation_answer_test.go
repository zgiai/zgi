package workflow

import "testing"

func TestWorkflowConversationAnswerAccumulatorPreservesStreamedResumeText(t *testing.T) {
	answer := newWorkflowConversationAnswerAccumulator("before approval ")
	answer.Append("after approval")
	answer.Merge("after approval and final")

	if got, want := answer.String(), "before approval after approval and final"; got != want {
		t.Fatalf("answer = %q, want %q", got, want)
	}
	outputs := workflowOutputsWithConversationAnswer(map[string]interface{}{"result": 1}, answer.String())
	if got := extractWorkflowAnswer(outputs); got != answer.String() {
		t.Fatalf("persisted answer = %q, want %q", got, answer.String())
	}
}

func TestWorkflowConversationAnswerAccumulatorReplacesCumulativeSnapshot(t *testing.T) {
	answer := newWorkflowConversationAnswerAccumulator("")
	answer.Append("first")
	answer.Merge("first")
	answer.Append(" second")
	answer.Merge("first second")
	answer.Merge("first second")

	if got, want := answer.String(), "first second"; got != want {
		t.Fatalf("answer = %q, want %q", got, want)
	}
}

func TestWorkflowConversationAnswerAccumulatorMergesContinuationOnlySnapshot(t *testing.T) {
	answer := newWorkflowConversationAnswerAccumulator("before approval ")
	answer.Append("after approval")
	answer.Merge("after approval and final")

	if got, want := answer.String(), "before approval after approval and final"; got != want {
		t.Fatalf("answer = %q, want %q", got, want)
	}
}

func TestWorkflowEventDataWithConversationAnswerDoesNotOverwriteExplicitAnswer(t *testing.T) {
	data := workflowEventDataWithConversationAnswer(map[string]interface{}{
		"status":  "succeeded",
		"outputs": map[string]interface{}{"answer": "explicit"},
	}, "streamed")
	outputs, _ := data["outputs"].(map[string]interface{})
	if got := extractWorkflowAnswer(outputs); got != "explicit" {
		t.Fatalf("answer = %q, want explicit", got)
	}
}

func TestApprovalResumeTerminalOutputOverridesLiveProjection(t *testing.T) {
	handler := &WorkflowHandler{}
	run := &WorkflowRunLog{RuntimeProtocolVersion: workflowRuntimeProtocolVersionV2}
	_, answer, _ := handler.persistApprovalResumeConversationEvents(
		t.Context(),
		run,
		map[string]interface{}{"answer": "first\nsecond\nthird"},
		map[string]interface{}{"sys.conversation_id": "conversation-1"},
		true,
		"",
		"firstfirst\nsecondfirst\nsecond\nthird",
	)

	if got, want := answer, "first\nsecond\nthird"; got != want {
		t.Fatalf("answer = %q, want %q", got, want)
	}
}
