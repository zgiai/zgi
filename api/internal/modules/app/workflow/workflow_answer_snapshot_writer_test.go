package workflow

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

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
