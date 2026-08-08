package workflow

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
)

func TestTaskWorkflowDoesNotCreateAnswerSnapshotWriter(t *testing.T) {
	writer := newWorkflowAnswerSnapshotWriter("WORKFLOW", &WorkflowHandler{}, "run-1", "agent-1", "account-1", nil, nil, "web-app")
	if writer != nil {
		writer.closeWithoutFlush()
		t.Fatal("task workflow created a conversation answer snapshot writer")
	}
}

func TestAnswerSnapshotWriterCoalescesTokenChunksAndFlushesTerminalAnswer(t *testing.T) {
	writer := &answerSnapshotWriter{
		workflowRunID: "run-1",
		wake:          make(chan struct{}, 1),
		stop:          make(chan struct{}),
		done:          make(chan struct{}),
	}
	var mu sync.Mutex
	writes := make([]string, 0, 2)
	writer.persistCheckpoint = func(_ context.Context, previous, answer, _ string) (int64, error) {
		mu.Lock()
		defer mu.Unlock()
		if len(writes) > 0 && previous != writes[len(writes)-1] {
			return 0, fmt.Errorf("previous answer %q does not match last checkpoint", previous)
		}
		writes = append(writes, answer)
		return int64(len(writes)), nil
	}
	go writer.run()

	answer := ""
	for index := 0; index < 1000; index++ {
		answer += "x"
		writer.PersistAsync(context.Background(), answer, "running", false)
	}
	if err := writer.PersistFinal(context.Background(), answer, "succeeded"); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(writes) == 0 || len(writes) > 2 {
		t.Fatalf("checkpoint writes = %d, want 1..2", len(writes))
	}
	if got := writes[len(writes)-1]; got != answer {
		t.Fatalf("final checkpoint length = %d, want %d", len(got), len(answer))
	}
}

func TestWorkflowAnswerDeltaFallsBackToReplaceWhenAnswerDiverges(t *testing.T) {
	delta, replace := workflowAnswerDelta("prefix old", "prefix new")
	if !replace || delta != "prefix new" {
		t.Fatalf("delta = %q replace = %v", delta, replace)
	}
}

func TestAnswerSnapshotWriterSeedKeepsContinuationDeltaIncremental(t *testing.T) {
	writer := &answerSnapshotWriter{
		workflowRunID: "run-resume",
		wake:          make(chan struct{}, 1),
		stop:          make(chan struct{}),
		done:          make(chan struct{}),
	}
	writer.SeedPersistedAnswer("before approval ")

	var previous, answer string
	writer.persistCheckpoint = func(_ context.Context, gotPrevious, gotAnswer, _ string) (int64, error) {
		previous = gotPrevious
		answer = gotAnswer
		return 2, nil
	}
	go writer.run()

	if err := writer.PersistFinal(context.Background(), "before approval after approval", "succeeded"); err != nil {
		t.Fatal(err)
	}
	if previous != "before approval " {
		t.Fatalf("previous answer = %q, want pre-pause baseline", previous)
	}
	if answer != "before approval after approval" {
		t.Fatalf("answer = %q, want complete resumed answer", answer)
	}
}

func TestAnswerSnapshotWriterPersistsStoppedTailWithRevokedOwner(t *testing.T) {
	writer := &answerSnapshotWriter{
		workflowRunID: "00000000-0000-0000-0000-000000000101",
		wake:          make(chan struct{}, 1),
		stop:          make(chan struct{}),
		done:          make(chan struct{}),
	}
	owner := workflowExecutionOwner{
		WorkflowRunID: writer.workflowRunID,
		ExecutionID:   "00000000-0000-0000-0000-000000000201",
		Generation:    4,
	}
	ctx := withWorkflowExecutionOwner(context.Background(), owner)
	var gotOwner workflowExecutionOwner
	var gotAnswer string
	writer.persistCheckpoint = func(context.Context, string, string, string) (int64, error) {
		return 0, workflowpause.ErrExecutionOwnershipLost
	}
	writer.persistStoppedFinal = func(_ context.Context, owner workflowExecutionOwner, answer string) error {
		gotOwner = owner
		gotAnswer = answer
		return nil
	}
	go writer.run()

	writer.PersistAsync(ctx, "七零八落\n一心一意\n", conversation.AgentMessageStatusRunning, false)
	if err := writer.PersistStoppedFinal(ctx, "七零八落\n一心"); err != nil {
		t.Fatal(err)
	}
	if gotOwner != owner {
		t.Fatalf("stopped owner = %#v, want %#v", gotOwner, owner)
	}
	if got, want := gotAnswer, "七零八落\n一心一意\n"; got != want {
		t.Fatalf("stopped answer = %q, want %q", got, want)
	}
}
