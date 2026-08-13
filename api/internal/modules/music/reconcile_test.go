package music

import (
	"testing"
	"time"
)

func TestReconcilerRepairsDurableTasksMissingFromQueue(t *testing.T) {
	generation := queuedTask()
	generation.UpdatedAt = time.Now().Add(-time.Minute)
	compensation := queuedTask()
	compensation.Status = StatusCompensationPending
	compensation.UpdatedAt = time.Now().Add(-time.Minute)
	staleGeneration := queuedTask()
	staleGeneration.Status = StatusGenerating
	staleGeneration.UpdatedAt = time.Now().Add(-20 * time.Minute)
	repo := newMemoryRepository(generation, compensation, staleGeneration)
	dispatcher := &dispatcherStub{}
	handler := NewReconcileHandler(repo, dispatcher)

	if err := handler.Handle(t.Context(), nil); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if dispatcher.generated != generation.ID {
		t.Fatalf("generation task = %s, want %s", dispatcher.generated, generation.ID)
	}
	if !dispatcher.hasCompensation(compensation.ID) || !dispatcher.hasCompensation(staleGeneration.ID) {
		t.Fatalf("compensation tasks = %v, want %s and %s", dispatcher.compensatedIDs, compensation.ID, staleGeneration.ID)
	}
	if !repo.tasks[generation.ID].UpdatedAt.After(generation.UpdatedAt) {
		t.Fatalf("generation updated_at was not rotated")
	}
	if !repo.tasks[compensation.ID].UpdatedAt.After(compensation.UpdatedAt) {
		t.Fatalf("compensation updated_at was not rotated")
	}
	if got := repo.tasks[staleGeneration.ID]; got.Status != StatusCompensationPending || got.ErrorCode != ErrorCodeDeliveryUnknown {
		t.Fatalf("stale generation = %#v, want compensation pending", got)
	}
}

func TestReconcilerLeavesActiveGenerationAlone(t *testing.T) {
	task := queuedTask()
	task.Status = StatusGenerating
	task.UpdatedAt = time.Now()
	repo := newMemoryRepository(task)
	dispatcher := &dispatcherStub{}

	if err := NewReconcileHandler(repo, dispatcher).Handle(t.Context(), nil); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if got := repo.tasks[task.ID].Status; got != StatusGenerating {
		t.Fatalf("task status = %s, want %s", got, StatusGenerating)
	}
	if dispatcher.hasCompensation(task.ID) {
		t.Fatal("active generation was queued for compensation")
	}
}

func TestReconcilerRetriesStaleLyricsWithoutCompensation(t *testing.T) {
	task := queuedTask()
	task.Status = StatusGeneratingLyrics
	task.UpdatedAt = time.Now().Add(-20 * time.Minute)
	repo := newMemoryRepository(task)
	dispatcher := &dispatcherStub{}

	if err := NewReconcileHandler(repo, dispatcher).Handle(t.Context(), nil); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if dispatcher.generated != task.ID || dispatcher.generateCalls != 1 {
		t.Fatalf("lyrics task was not requeued: %#v", dispatcher)
	}
	if dispatcher.hasCompensation(task.ID) {
		t.Fatal("lyrics task was incorrectly queued for billing compensation")
	}
	if got := repo.tasks[task.ID].Status; got != StatusGeneratingLyrics {
		t.Fatalf("task status = %s, want %s", got, StatusGeneratingLyrics)
	}
}
