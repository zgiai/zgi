package workflow

import (
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/config"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	workflow_shared "github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

func TestAnswerOutputCoordinatorDoesNotEmitBeforeAnswerActive(t *testing.T) {
	restoreAnswerCoordinatorConfig(t, 20, nil)
	coordinator, resultChan := newTestAnswerOutputCoordinator(t, []testAnswerNode{
		{id: "llm", nodeType: "llm"},
		{id: "answer", nodeType: "answer", answer: "prefix {{#llm.text#}} suffix"},
	}, nil)

	if !coordinator.HandleStreamChunk("llm", testRunChunk("llm", "text", "hello")) {
		t.Fatal("HandleStreamChunk() = false, want true for watched answer selector")
	}
	assertNoAnswerMessages(t, resultChan)

	coordinator.MarkAnswerActive("answer")
	assertAnswerMessages(t, resultChan, []string{"prefix "})

	coordinator.MarkNodeFinished("llm", "llm", string(workflow_shared.SUCCEEDED), map[string]any{"text": "hello"}, nil)
	coordinator.MarkNodeFinished("answer", "answer", string(workflow_shared.SUCCEEDED), nil, nil)
	assertAnswerMessages(t, resultChan, []string{"hello", " suffix"})
	if got := coordinator.FullAnswer(); got != "prefix hello suffix" {
		t.Fatalf("FullAnswer() = %q, want %q", got, "prefix hello suffix")
	}
}

func TestAnswerOutputCoordinatorEligibleAnswerStreamsBeforeAnswerStarts(t *testing.T) {
	restoreAnswerCoordinatorConfig(t, 20, nil)
	coordinator, resultChan := newTestAnswerOutputCoordinator(t, []testAnswerNode{
		{id: "start", nodeType: "start"},
		{id: "llm", nodeType: "llm"},
		{id: "answer", nodeType: "answer", answer: "prefix {{#llm.text#}}"},
	}, []testAnswerEdge{
		{source: "start", target: "llm"},
		{source: "llm", target: "answer"},
	})

	if !coordinator.HandleStreamChunk("llm", testRunChunk("llm", "text", "hello")) {
		t.Fatal("HandleStreamChunk() = false, want true for watched answer selector")
	}
	assertAnswerMessages(t, resultChan, []string{"prefix ", "hello"})
}

func TestAnswerOutputCoordinatorEmitsSnapshotAfterReleasedMessages(t *testing.T) {
	restoreAnswerCoordinatorConfig(t, 20, nil)
	coordinator, resultChan := newTestAnswerOutputCoordinator(t, []testAnswerNode{
		{id: "start", nodeType: "start"},
		{id: "llm", nodeType: "llm"},
		{id: "answer", nodeType: "answer", answer: "prefix {{#llm.text#}}"},
	}, []testAnswerEdge{
		{source: "start", target: "llm"},
		{source: "llm", target: "answer"},
	})

	if !coordinator.HandleStreamChunk("llm", testRunChunk("llm", "text", "hello")) {
		t.Fatal("HandleStreamChunk() = false, want true for watched answer selector")
	}

	events := drainAnswerWorkflowStreamEvents(resultChan)
	if len(events) != 3 {
		t.Fatalf("events = %d, want 3", len(events))
	}
	if events[0].EventType != "message" || workflowMessageEventText(events[0].Data) != "prefix " {
		t.Fatalf("first event = %#v, want prefix message", events[0])
	}
	if events[1].EventType != "message" || workflowMessageEventText(events[1].Data) != "hello" {
		t.Fatalf("second event = %#v, want chunk message", events[1])
	}
	if events[2].EventType != workflowEventAnswerSnapshotReady || workflowAnswerSnapshotText(events[2].Data) != "prefix hello" {
		t.Fatalf("third event = %#v, want answer snapshot", events[2])
	}
}

func TestAnswerOutputCoordinatorEligibleFixedAnswerWaitsForAnswerStart(t *testing.T) {
	restoreAnswerCoordinatorConfig(t, 20, nil)
	coordinator, resultChan := newTestAnswerOutputCoordinator(t, []testAnswerNode{
		{id: "start", nodeType: "start"},
		{id: "answer", nodeType: "answer", answer: "fixed"},
	}, []testAnswerEdge{
		{source: "start", target: "answer"},
	})

	assertNoAnswerMessages(t, resultChan)

	coordinator.MarkAnswerActive("answer")
	coordinator.MarkNodeFinished("answer", "answer", string(workflow_shared.SUCCEEDED), nil, nil)
	assertAnswerMessages(t, resultChan, []string{"fixed"})
}

func TestAnswerOutputCoordinatorUnknownBranchVariableUsesFinalOnlyUntilSelected(t *testing.T) {
	restoreAnswerCoordinatorConfig(t, 20, nil)
	coordinator, resultChan := newTestAnswerOutputCoordinator(t, []testAnswerNode{
		{id: "start", nodeType: "start"},
		{id: "llm", nodeType: "llm"},
		{id: "if", nodeType: "if-else"},
		{id: "answer", nodeType: "answer", answer: "{{#llm.text#}}"},
	}, []testAnswerEdge{
		{source: "start", target: "llm"},
		{source: "llm", target: "if"},
		{source: "if", sourceHandle: "case1", target: "answer"},
	})

	coordinator.HandleStreamChunk("llm", testRunChunk("llm", "text", "streamed"))
	assertNoAnswerMessages(t, resultChan)

	coordinator.MarkNodeFinished("llm", "llm", string(workflow_shared.SUCCEEDED), map[string]any{"text": "final"}, nil)
	assertNoAnswerMessages(t, resultChan)

	coordinator.MarkSelectedHandleReachable("if", string(workflow_shared.SUCCEEDED), "case1")
	assertAnswerMessages(t, resultChan, []string{"final"})
}

func TestAnswerOutputCoordinatorBlocksLaterVariableUntilEarlierFinalizes(t *testing.T) {
	restoreAnswerCoordinatorConfig(t, 20, nil)
	coordinator, resultChan := newTestAnswerOutputCoordinator(t, []testAnswerNode{
		{id: "code", nodeType: "code"},
		{id: "llm", nodeType: "llm"},
		{id: "answer", nodeType: "answer", answer: "before {{#code.result#}} middle {{#llm.text#}} tail"},
	}, nil)

	coordinator.MarkAnswerActive("answer")
	assertAnswerMessages(t, resultChan, []string{"before "})

	coordinator.HandleStreamChunk("llm", testRunChunk("llm", "text", "streaming"))
	assertNoAnswerMessages(t, resultChan)

	coordinator.MarkNodeFinished("code", "code", string(workflow_shared.SUCCEEDED), map[string]any{"result": "A"}, nil)
	assertAnswerMessages(t, resultChan, []string{"A", " middle "})

	coordinator.HandleStreamChunk("llm", testRunChunk("llm", "text", "-now"))
	assertNoAnswerMessages(t, resultChan)

	coordinator.MarkNodeFinished("llm", "llm", string(workflow_shared.SUCCEEDED), map[string]any{"text": "streaming-now"}, nil)
	coordinator.MarkNodeFinished("answer", "answer", string(workflow_shared.SUCCEEDED), nil, nil)
	assertAnswerMessages(t, resultChan, []string{"streaming-now", " tail"})
	if got := coordinator.FullAnswer(); got != "before A middle streaming-now tail" {
		t.Fatalf("FullAnswer() = %q, want %q", got, "before A middle streaming-now tail")
	}
}

func TestAnswerOutputCoordinatorUnknownUnrelatedAnswerDoesNotBlockActiveAnswer(t *testing.T) {
	restoreAnswerCoordinatorConfig(t, 20, nil)
	coordinator, resultChan := newTestAnswerOutputCoordinator(t, []testAnswerNode{
		{id: "answer-a", nodeType: "answer", answer: "A"},
		{id: "answer-b", nodeType: "answer", answer: "B"},
	}, nil)

	coordinator.MarkAnswerActive("answer-b")
	coordinator.MarkNodeFinished("answer-b", "answer", string(workflow_shared.SUCCEEDED), nil, nil)

	assertAnswerMessages(t, resultChan, []string{"B"})
	if got := coordinator.FullAnswer(); got != "B" {
		t.Fatalf("FullAnswer() = %q, want %q", got, "B")
	}
}

func TestAnswerOutputCoordinatorReadyBatchStabilizesParallelAnswers(t *testing.T) {
	restoreAnswerCoordinatorConfig(t, 20, nil)
	coordinator, resultChan := newTestAnswerOutputCoordinator(t, []testAnswerNode{
		{id: "answer-a", nodeType: "answer", answer: "6"},
		{id: "answer-b", nodeType: "answer", answer: "5"},
	}, nil)

	coordinator.RegisterReadyBatch(answerTopScope(), []string{"answer-b", "answer-a"})
	coordinator.MarkAnswerActive("answer-b")
	assertNoAnswerMessages(t, resultChan)

	coordinator.MarkAnswerActive("answer-a")
	coordinator.MarkNodeFinished("answer-b", "answer", string(workflow_shared.SUCCEEDED), nil, nil)
	coordinator.MarkNodeFinished("answer-a", "answer", string(workflow_shared.SUCCEEDED), nil, nil)

	assertAnswerMessages(t, resultChan, []string{"6", "5"})
	if got := coordinator.FullAnswer(); got != "65" {
		t.Fatalf("FullAnswer() = %q, want %q", got, "65")
	}
}

func TestAnswerOutputCoordinatorIterationScopeOrdersIndexes(t *testing.T) {
	restoreAnswerCoordinatorConfig(t, 20, nil)
	coordinator, resultChan := newTestAnswerOutputCoordinator(t, []testAnswerNode{
		{id: "iteration", nodeType: "iteration"},
		{id: "inneranswer", nodeType: "answer", answer: "{{#inneranswer.text#}}", parentID: "iteration"},
	}, nil)

	indexTwo := answerOutputScope{kind: answerScopeIteration, parentNodeID: "iteration", index: 2}
	indexOne := answerOutputScope{kind: answerScopeIteration, parentNodeID: "iteration", index: 1}
	indexZero := answerOutputScope{kind: answerScopeIteration, parentNodeID: "iteration", index: 0}

	coordinator.RegisterReadyBatch(indexTwo, []string{"inneranswer"})
	coordinator.MarkAnswerActiveScoped(indexTwo, "inneranswer")
	coordinator.MarkNodeFinishedScoped(indexTwo, "inneranswer", "answer", string(workflow_shared.SUCCEEDED), map[string]any{"text": "2"}, nil)
	assertNoAnswerMessages(t, resultChan)

	coordinator.RegisterReadyBatch(indexZero, []string{"inneranswer"})
	coordinator.MarkAnswerActiveScoped(indexZero, "inneranswer")
	coordinator.MarkNodeFinishedScoped(indexZero, "inneranswer", "answer", string(workflow_shared.SUCCEEDED), map[string]any{"text": "0"}, nil)
	assertAnswerMessages(t, resultChan, []string{"0"})

	coordinator.RegisterReadyBatch(indexOne, []string{"inneranswer"})
	coordinator.MarkAnswerActiveScoped(indexOne, "inneranswer")
	coordinator.MarkNodeFinishedScoped(indexOne, "inneranswer", "answer", string(workflow_shared.SUCCEEDED), map[string]any{"text": "1"}, nil)
	assertAnswerMessages(t, resultChan, []string{"1", "2"})
	if got := coordinator.FullAnswer(); got != "012" {
		t.Fatalf("FullAnswer() = %q, want %q", got, "012")
	}
}

func TestAnswerOutputCoordinatorIterationItemVariableReleasesInIndexOrder(t *testing.T) {
	restoreAnswerCoordinatorConfig(t, 20, nil)
	coordinator, resultChan := newTestAnswerOutputCoordinator(t, []testAnswerNode{
		{id: "iteration", nodeType: "iteration"},
		{id: "inneranswer", nodeType: "answer", answer: "{{#iteration.item#}}", parentID: "iteration"},
	}, nil)

	indexTwo := answerOutputScope{kind: answerScopeIteration, parentNodeID: "iteration", index: 2}
	indexOne := answerOutputScope{kind: answerScopeIteration, parentNodeID: "iteration", index: 1}
	indexZero := answerOutputScope{kind: answerScopeIteration, parentNodeID: "iteration", index: 0}

	coordinator.RegisterReadyBatch(indexTwo, []string{"inneranswer"})
	coordinator.MarkScopedSourceAvailable(indexTwo, "iteration", map[string]any{"item": "3", "index": 2})
	coordinator.MarkAnswerActiveScoped(indexTwo, "inneranswer")
	coordinator.MarkNodeFinishedScoped(indexTwo, "inneranswer", "answer", string(workflow_shared.SUCCEEDED), map[string]any{"answer": "3"}, nil)
	assertNoAnswerMessages(t, resultChan)

	coordinator.RegisterReadyBatch(indexZero, []string{"inneranswer"})
	coordinator.MarkScopedSourceAvailable(indexZero, "iteration", map[string]any{"item": "1", "index": 0})
	coordinator.MarkAnswerActiveScoped(indexZero, "inneranswer")
	coordinator.MarkNodeFinishedScoped(indexZero, "inneranswer", "answer", string(workflow_shared.SUCCEEDED), map[string]any{"answer": "1"}, nil)
	assertAnswerMessages(t, resultChan, []string{"1"})
	if got := coordinator.iterationNext["iteration"]; got != 1 {
		t.Fatalf("iterationNext = %d, want 1", got)
	}

	coordinator.RegisterReadyBatch(indexOne, []string{"inneranswer"})
	coordinator.MarkScopedSourceAvailable(indexOne, "iteration", map[string]any{"item": "2", "index": 1})
	coordinator.MarkAnswerActiveScoped(indexOne, "inneranswer")
	coordinator.MarkNodeFinishedScoped(indexOne, "inneranswer", "answer", string(workflow_shared.SUCCEEDED), map[string]any{"answer": "2"}, nil)
	assertAnswerMessages(t, resultChan, []string{"2", "3"})
	if got := coordinator.FullAnswer(); got != "123" {
		t.Fatalf("FullAnswer() = %q, want %q", got, "123")
	}
}

func TestAnswerOutputCoordinatorBlockedActiveAnswerDoesNotBlockReadyUnrelatedAnswer(t *testing.T) {
	restoreAnswerCoordinatorConfig(t, 20, nil)
	coordinator, resultChan := newTestAnswerOutputCoordinator(t, []testAnswerNode{
		{id: "code", nodeType: "code"},
		{id: "answer-a", nodeType: "answer", answer: "A{{#code.result#}}"},
		{id: "answer-b", nodeType: "answer", answer: "B"},
	}, nil)

	coordinator.MarkAnswerActive("answer-a")
	assertAnswerMessages(t, resultChan, []string{"A"})

	coordinator.MarkAnswerActive("answer-b")
	coordinator.MarkNodeFinished("answer-b", "answer", string(workflow_shared.SUCCEEDED), nil, nil)
	assertAnswerMessages(t, resultChan, []string{"B"})

	coordinator.MarkNodeFinished("code", "code", string(workflow_shared.SUCCEEDED), map[string]any{"result": "done"}, nil)
	coordinator.MarkNodeFinished("answer-a", "answer", string(workflow_shared.SUCCEEDED), nil, nil)
	assertAnswerMessages(t, resultChan, []string{"done"})
	if got := coordinator.FullAnswer(); got != "ABdone" {
		t.Fatalf("FullAnswer() = %q, want %q", got, "ABdone")
	}
}

func TestAnswerOutputCoordinatorSkippedAnswerReleasesTopologicalBlock(t *testing.T) {
	restoreAnswerCoordinatorConfig(t, 20, nil)
	coordinator, resultChan := newTestAnswerOutputCoordinator(t, []testAnswerNode{
		{id: "answer-a", nodeType: "answer", answer: "A"},
		{id: "answer-b", nodeType: "answer", answer: "B"},
	}, []testAnswerEdge{
		{source: "answer-a", target: "answer-b"},
	})

	coordinator.MarkAnswerActive("answer-b")
	assertNoAnswerMessages(t, resultChan)

	coordinator.MarkNodeSkipped("answer-a")
	coordinator.MarkNodeFinished("answer-b", "answer", string(workflow_shared.SUCCEEDED), nil, nil)
	assertAnswerMessages(t, resultChan, []string{"B"})
}

func TestAnswerOutputCoordinatorFailedAnswerBlocksTopologicalSuccessor(t *testing.T) {
	restoreAnswerCoordinatorConfig(t, 20, nil)
	coordinator, resultChan := newTestAnswerOutputCoordinator(t, []testAnswerNode{
		{id: "answer-a", nodeType: "answer", answer: "A"},
		{id: "answer-b", nodeType: "answer", answer: "B"},
	}, []testAnswerEdge{
		{source: "answer-a", target: "answer-b"},
	})

	coordinator.MarkAnswerActive("answer-b")
	coordinator.MarkNodeFinished("answer-a", "answer", string(workflow_shared.FAILED), nil, assertErr("failed answer"))
	assertNoAnswerMessages(t, resultChan)
}

func TestAnswerOutputCoordinatorBlockedVariableUsesFinalOnly(t *testing.T) {
	restoreAnswerCoordinatorConfig(t, 4, nil)
	coordinator, resultChan := newTestAnswerOutputCoordinator(t, []testAnswerNode{
		{id: "code", nodeType: "code"},
		{id: "llm", nodeType: "llm"},
		{id: "answer", nodeType: "answer", answer: "{{#code.result#}}{{#llm.text#}}"},
	}, nil)

	coordinator.MarkAnswerActive("answer")
	coordinator.HandleStreamChunk("llm", testRunChunk("llm", "text", "discarded"))
	assertNoAnswerMessages(t, resultChan)

	coordinator.MarkNodeFinished("code", "code", string(workflow_shared.SUCCEEDED), map[string]any{"result": "A"}, nil)
	coordinator.MarkNodeFinished("llm", "llm", string(workflow_shared.SUCCEEDED), map[string]any{"text": "final-only"}, nil)
	coordinator.MarkNodeFinished("answer", "answer", string(workflow_shared.SUCCEEDED), nil, nil)

	assertAnswerMessages(t, resultChan, []string{"A", "fina", "l-on", "ly"})
	if got := coordinator.FullAnswer(); got != "Afinal-only" {
		t.Fatalf("FullAnswer() = %q, want %q", got, "Afinal-only")
	}
}

func TestAnswerOutputCoordinatorFinalOnlyEmitsOnlySuffixAfterPartialRelease(t *testing.T) {
	restoreAnswerCoordinatorConfig(t, 4, nil)
	coordinator, resultChan := newTestAnswerOutputCoordinator(t, []testAnswerNode{
		{id: "llm", nodeType: "llm"},
		{id: "answer", nodeType: "answer", answer: "{{#llm.text#}}"},
	}, nil)

	coordinator.MarkAnswerActive("answer")
	coordinator.HandleStreamChunk("llm", testRunChunk("llm", "text", "he"))
	assertAnswerMessages(t, resultChan, []string{"he"})

	variable := firstTestAnswerVariableBySource(t, coordinator, "llm")
	variable.finalOnly = true

	coordinator.MarkNodeFinished("llm", "llm", string(workflow_shared.SUCCEEDED), map[string]any{"text": "hello"}, nil)
	coordinator.MarkNodeFinished("answer", "answer", string(workflow_shared.SUCCEEDED), nil, nil)

	assertAnswerMessages(t, resultChan, []string{"llo"})
	if got := coordinator.FullAnswer(); got != "hello" {
		t.Fatalf("FullAnswer() = %q, want %q", got, "hello")
	}
}

func TestAnswerOutputCoordinatorPreparePauseSnapshotConvertsBlockedChunksToFinalOnly(t *testing.T) {
	restoreAnswerCoordinatorConfig(t, 20, nil)
	coordinator, resultChan := newTestAnswerOutputCoordinator(t, []testAnswerNode{
		{id: "code", nodeType: "code"},
		{id: "llm", nodeType: "llm"},
		{id: "answer", nodeType: "answer", answer: "before {{#code.result#}} middle {{#llm.text#}}"},
	}, nil)

	coordinator.MarkAnswerActive("answer")
	assertAnswerMessages(t, resultChan, []string{"before "})

	coordinator.HandleStreamChunk("llm", testRunChunk("llm", "text", "streaming"))
	assertNoAnswerMessages(t, resultChan)

	snapshot, messages := coordinator.PreparePauseSnapshot()
	if len(messages) != 0 {
		t.Fatalf("PreparePauseSnapshot() messages = %#v, want none", messages)
	}
	if snapshot == nil {
		t.Fatal("PreparePauseSnapshot() snapshot = nil, want snapshot")
	}
	if snapshot.FullAnswer != "before " {
		t.Fatalf("snapshot.FullAnswer = %q, want %q", snapshot.FullAnswer, "before ")
	}

	variable := firstTestAnswerVariableBySource(t, coordinator, "llm")
	if !variable.finalOnly {
		t.Fatal("variable.finalOnly = false, want true")
	}
	if len(variable.chunks) != 0 {
		t.Fatalf("variable.chunks = %#v, want empty", variable.chunks)
	}

	state := snapshotVariableState(t, snapshot, variable.stateKey)
	if !state.FinalOnly {
		t.Fatal("snapshot variable final_only = false, want true")
	}
}

func TestAnswerOutputCoordinatorRestorePauseSnapshotResumesWithFinalOnlyRemainder(t *testing.T) {
	restoreAnswerCoordinatorConfig(t, 20, nil)
	original, originalChan := newTestAnswerOutputCoordinator(t, []testAnswerNode{
		{id: "code", nodeType: "code"},
		{id: "llm", nodeType: "llm"},
		{id: "answer", nodeType: "answer", answer: "before {{#code.result#}} middle {{#llm.text#}}"},
	}, nil)

	original.MarkAnswerActive("answer")
	assertAnswerMessages(t, originalChan, []string{"before "})
	original.HandleStreamChunk("llm", testRunChunk("llm", "text", "streaming"))
	assertNoAnswerMessages(t, originalChan)

	snapshot, _ := original.PreparePauseSnapshot()
	if snapshot == nil {
		t.Fatal("PreparePauseSnapshot() snapshot = nil, want snapshot")
	}

	restored, restoredChan := newTestAnswerOutputCoordinator(t, []testAnswerNode{
		{id: "code", nodeType: "code"},
		{id: "llm", nodeType: "llm"},
		{id: "answer", nodeType: "answer", answer: "before {{#code.result#}} middle {{#llm.text#}}"},
	}, nil)
	if err := restored.RestorePauseSnapshot(snapshot); err != nil {
		t.Fatalf("RestorePauseSnapshot() error = %v, want nil", err)
	}
	assertNoAnswerMessages(t, restoredChan)

	restored.MarkNodeFinished("code", "code", string(workflow_shared.SUCCEEDED), map[string]any{"result": "A"}, nil)
	assertAnswerMessages(t, restoredChan, []string{"A", " middle "})

	restored.MarkNodeFinished("llm", "llm", string(workflow_shared.SUCCEEDED), map[string]any{"text": "streaming"}, nil)
	restored.MarkNodeFinished("answer", "answer", string(workflow_shared.SUCCEEDED), nil, nil)
	assertAnswerMessages(t, restoredChan, []string{"streaming"})

	if got := restored.FullAnswer(); got != "before A middle streaming" {
		t.Fatalf("FullAnswer() = %q, want %q", got, "before A middle streaming")
	}
}

func TestAnswerOutputCoordinatorPausedQuestionAnswerDoesNotFailDirectReply(t *testing.T) {
	restoreAnswerCoordinatorConfig(t, 20, nil)
	original, originalChan := newQuestionAnswerDirectReplyCoordinator(t)

	original.MarkNodeFinished("qa1", "question-answer", string(workflow_shared.SUCCEEDED), map[string]any{"answer": "test1"}, nil)
	assertAnswerMessages(t, originalChan, []string{"test1"})

	original.MarkNodeFinished("qa2", "question-answer", string(workflow_shared.PAUSED), map[string]any{"question": "second question"}, nil)
	assertNoAnswerMessages(t, originalChan)

	snapshot, _ := original.PreparePauseSnapshot()
	if snapshot == nil {
		t.Fatal("PreparePauseSnapshot() snapshot = nil, want snapshot")
	}
	if snapshot.FullAnswer != "test1" {
		t.Fatalf("snapshot.FullAnswer = %q, want %q", snapshot.FullAnswer, "test1")
	}

	restored, restoredChan := newQuestionAnswerDirectReplyCoordinator(t)
	if err := restored.RestorePauseSnapshot(snapshot); err != nil {
		t.Fatalf("RestorePauseSnapshot() error = %v, want nil", err)
	}

	restored.MarkNodeFinished("qa2", "question-answer", string(workflow_shared.SUCCEEDED), map[string]any{"answer": "test2"}, nil)
	restored.MarkNodeFinished("qa3", "question-answer", string(workflow_shared.SUCCEEDED), map[string]any{"answer": "222"}, nil)
	restored.MarkAnswerActive("answer")
	restored.MarkNodeFinished("answer", "answer", string(workflow_shared.SUCCEEDED), map[string]any{"answer": "test1test2222"}, nil)

	assertAnswerMessages(t, restoredChan, []string{"test2", "222"})
	if got := restored.FullAnswer(); got != "test1test2222" {
		t.Fatalf("FullAnswer() = %q, want %q", got, "test1test2222")
	}
	if !restored.HasCompleteOutput() {
		t.Fatal("HasCompleteOutput() = false, want true")
	}
}

func TestFinalizeWorkflowStreamExecutionKeepsFinalAnswerWhenCoordinatorIncomplete(t *testing.T) {
	restoreAnswerCoordinatorConfig(t, 20, nil)
	coordinator, resultChan := newQuestionAnswerDirectReplyCoordinator(t)
	coordinator.MarkNodeFinished("qa1", "question-answer", string(workflow_shared.SUCCEEDED), map[string]any{"answer": "test1"}, nil)
	assertAnswerMessages(t, resultChan, []string{"test1"})
	if coordinator.HasCompleteOutput() {
		t.Fatal("HasCompleteOutput() = true, want false for partial answer")
	}

	doneChan := make(chan map[string]interface{}, 1)
	finalizeWorkflowStreamExecution(workflowStreamFinalizeParams{
		Ctx:                    t.Context(),
		WorkflowElapsedTracker: newWorkflowElapsedTrackerFromNodeLogs(nil),
		WorkflowStartTime:      time.Now(),
		FailedNodes:            map[string]string{},
		AllNodeOutputs:         map[string]interface{}{"answer": "test1test2222"},
		DoneChan:               doneChan,
		AnswerCoordinator:      coordinator,
	})

	outputs := <-doneChan
	if got := outputs["answer"]; got != "test1test2222" {
		t.Fatalf("outputs[answer] = %#v, want %#v", got, "test1test2222")
	}
}

func TestFinalizeWorkflowStreamExecutionUsesCoordinatorWhenComplete(t *testing.T) {
	restoreAnswerCoordinatorConfig(t, 20, nil)
	coordinator, resultChan := newQuestionAnswerDirectReplyCoordinator(t)
	coordinator.MarkNodeFinished("qa1", "question-answer", string(workflow_shared.SUCCEEDED), map[string]any{"answer": "test1"}, nil)
	coordinator.MarkNodeFinished("qa2", "question-answer", string(workflow_shared.SUCCEEDED), map[string]any{"answer": "test2"}, nil)
	coordinator.MarkNodeFinished("qa3", "question-answer", string(workflow_shared.SUCCEEDED), map[string]any{"answer": "222"}, nil)
	coordinator.MarkAnswerActive("answer")
	coordinator.MarkNodeFinished("answer", "answer", string(workflow_shared.SUCCEEDED), map[string]any{"answer": "test1test2222"}, nil)
	assertAnswerMessages(t, resultChan, []string{"test1", "test2", "222"})
	if !coordinator.HasCompleteOutput() {
		t.Fatal("HasCompleteOutput() = false, want true for fully drained answer")
	}

	doneChan := make(chan map[string]interface{}, 1)
	finalizeWorkflowStreamExecution(workflowStreamFinalizeParams{
		Ctx:                    t.Context(),
		WorkflowElapsedTracker: newWorkflowElapsedTrackerFromNodeLogs(nil),
		WorkflowStartTime:      time.Now(),
		FailedNodes:            map[string]string{},
		AllNodeOutputs:         map[string]interface{}{"answer": "stale"},
		DoneChan:               doneChan,
		AnswerCoordinator:      coordinator,
	})

	outputs := <-doneChan
	if got := outputs["answer"]; got != "test1test2222" {
		t.Fatalf("outputs[answer] = %#v, want %#v", got, "test1test2222")
	}
}

func TestAnswerOutputCoordinatorRestorePauseSnapshotRejectsTemplateMismatch(t *testing.T) {
	restoreAnswerCoordinatorConfig(t, 20, nil)
	original, _ := newTestAnswerOutputCoordinator(t, []testAnswerNode{
		{id: "llm", nodeType: "llm"},
		{id: "answer", nodeType: "answer", answer: "prefix {{#llm.text#}}"},
	}, nil)
	original.MarkAnswerActive("answer")
	snapshot, _ := original.PreparePauseSnapshot()

	restored, _ := newTestAnswerOutputCoordinator(t, []testAnswerNode{
		{id: "llm", nodeType: "llm"},
		{id: "answer", nodeType: "answer", answer: "prefix {{#llm.text#}} suffix"},
	}, nil)
	if err := restored.RestorePauseSnapshot(snapshot); err == nil {
		t.Fatal("RestorePauseSnapshot() error = nil, want mismatch error")
	}
	if got := restored.FullAnswer(); got != "" {
		t.Fatalf("FullAnswer() = %q, want empty after failed restore", got)
	}
}

type testAnswerNode struct {
	id       string
	nodeType string
	answer   string
	parentID string
}

type testAnswerEdge struct {
	source       string
	sourceHandle string
	target       string
}

type assertErr string

func (e assertErr) Error() string {
	return string(e)
}

func newTestAnswerOutputCoordinator(t *testing.T, nodes []testAnswerNode, edges []testAnswerEdge) (*answerOutputCoordinator, chan *WorkflowStreamEvent) {
	t.Helper()

	graphNodes := make([]interface{}, 0, len(nodes))
	nodeMap := make(map[string]map[string]interface{}, len(nodes))
	for _, node := range nodes {
		data := map[string]interface{}{
			"type": node.nodeType,
		}
		if node.answer != "" {
			data["answer"] = node.answer
		}
		graphNode := map[string]interface{}{
			"id":   node.id,
			"data": data,
		}
		if node.parentID != "" {
			graphNode["parentId"] = node.parentID
		}
		graphNodes = append(graphNodes, graphNode)
		nodeMap[node.id] = graphNode
	}

	edgeMap := make(map[string]map[string][]string)
	graphEdges := make([]interface{}, 0, len(edges))
	for _, edge := range edges {
		if edgeMap[edge.source] == nil {
			edgeMap[edge.source] = make(map[string][]string)
		}
		sourceHandle := edge.sourceHandle
		if sourceHandle == "" {
			sourceHandle = "source"
		}
		edgeMap[edge.source][sourceHandle] = append(edgeMap[edge.source][sourceHandle], edge.target)
		graphEdges = append(graphEdges, map[string]interface{}{
			"source":       edge.source,
			"sourceHandle": sourceHandle,
			"target":       edge.target,
		})
	}

	startNodeID := ""
	for _, node := range nodes {
		if node.nodeType == "start" {
			startNodeID = node.id
			break
		}
	}

	resultChan := make(chan *WorkflowStreamEvent, 32)
	coordinator := newAnswerOutputCoordinator(
		"CONVERSATION_WORKFLOW",
		"run-1",
		map[string]interface{}{"sys.conversation_id": "conversation-1"},
		&workflowStreamGraph{
			GraphData: map[string]any{
				"nodes": graphNodes,
				"edges": graphEdges,
			},
			NodeMap:     nodeMap,
			EdgeMap:     edgeMap,
			StartNodeID: startNodeID,
		},
		resultChan,
	)
	if coordinator == nil {
		t.Fatal("newAnswerOutputCoordinator() = nil, want coordinator")
	}
	return coordinator, resultChan
}

func newQuestionAnswerDirectReplyCoordinator(t *testing.T) (*answerOutputCoordinator, chan *WorkflowStreamEvent) {
	t.Helper()

	return newTestAnswerOutputCoordinator(t, []testAnswerNode{
		{id: "start", nodeType: "start"},
		{id: "qa1", nodeType: "question-answer"},
		{id: "qa2", nodeType: "question-answer"},
		{id: "qa3", nodeType: "question-answer"},
		{id: "answer", nodeType: "answer", answer: "{{#qa1.answer#}}{{#qa2.answer#}}{{#qa3.answer#}}"},
	}, []testAnswerEdge{
		{source: "start", target: "qa1"},
		{source: "qa1", target: "qa2"},
		{source: "qa2", target: "qa3"},
		{source: "qa3", target: "answer"},
	})
}

func restoreAnswerCoordinatorConfig(t *testing.T, chunkSize int, mutate func(*config.Config)) {
	t.Helper()

	previousConfig := config.GlobalConfig
	cfg := &config.Config{
		AnswerNodeStreaming: config.AnswerNodeStreamingConfig{
			ChunkSize: chunkSize,
		},
	}
	if mutate != nil {
		mutate(cfg)
	}
	config.GlobalConfig = cfg
	t.Cleanup(func() {
		config.GlobalConfig = previousConfig
	})
}

func testRunChunk(nodeID string, outputKey string, content string) *workflow_shared.RunStreamChunkEvent {
	return &workflow_shared.RunStreamChunkEvent{
		ChunkContent:         content,
		FromVariableSelector: []string{nodeID, outputKey},
	}
}

func assertAnswerMessages(t *testing.T, resultChan <-chan *WorkflowStreamEvent, want []string) {
	t.Helper()

	got := drainAnswerMessages(resultChan)
	if strings.Join(got, "") != strings.Join(want, "") || len(got) != len(want) {
		t.Fatalf("messages = %#v, want %#v", got, want)
	}
}

func assertNoAnswerMessages(t *testing.T, resultChan <-chan *WorkflowStreamEvent) {
	t.Helper()

	if got := drainAnswerMessages(resultChan); len(got) != 0 {
		t.Fatalf("messages = %#v, want none", got)
	}
}

func drainAnswerMessages(resultChan <-chan *WorkflowStreamEvent) []string {
	var messages []string
	for {
		select {
		case event := <-resultChan:
			if event != nil && event.EventType == "message" {
				if answer, _ := event.Data["answer"].(string); answer != "" {
					messages = append(messages, answer)
				}
			}
		default:
			return messages
		}
	}
}

func drainAnswerWorkflowStreamEvents(resultChan <-chan *WorkflowStreamEvent) []*WorkflowStreamEvent {
	var events []*WorkflowStreamEvent
	for {
		select {
		case event := <-resultChan:
			events = append(events, event)
		default:
			return events
		}
	}
}

func firstTestAnswerVariableBySource(t *testing.T, coordinator *answerOutputCoordinator, sourceNode string) *answerVariableState {
	t.Helper()

	for _, variable := range coordinator.variables {
		if variable != nil && variable.sourceNode == sourceNode {
			return variable
		}
	}
	t.Fatalf("variable for source %s not found", sourceNode)
	return nil
}

func snapshotVariableState(t *testing.T, snapshot *workflowpause.AnswerOutputState, stateKey string) workflowpause.AnswerOutputVariableState {
	t.Helper()

	for _, variable := range snapshot.Variables {
		if variable.StateKey == stateKey {
			return variable
		}
	}
	t.Fatalf("snapshot variable %s not found", stateKey)
	return workflowpause.AnswerOutputVariableState{}
}
