package digitalhuman

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestApplyMemoryUndoStatusProjectsConsumedRecordAfterUndo(t *testing.T) {
	memoryID := uuid.New()
	createOperationID := uuid.New()
	undoOperationID := uuid.New()
	consumedAt := time.Now().UTC()
	expiresAt := consumedAt.Add(time.Hour)
	item := &DigitalHumanMemory{
		ID:              memoryID,
		LastOperationID: undoOperationID,
	}

	applyMemoryUndoStatus([]*DigitalHumanMemory{item}, []MemoryUndoRecord{{
		OperationID: createOperationID,
		MemoryID:    memoryID,
		ExpiresAt:   expiresAt,
		ConsumedAt:  &consumedAt,
	}})

	if !item.UndoConsumed {
		t.Fatal("UndoConsumed = false, want true after the mutation undo record was consumed")
	}
	if item.UndoableUntil == nil || !item.UndoableUntil.Equal(expiresAt) {
		t.Fatalf("UndoableUntil = %v, want %v", item.UndoableUntil, expiresAt)
	}
}

func TestApplyMemoryUndoStatusPrefersCurrentUnconsumedMutation(t *testing.T) {
	memoryID := uuid.New()
	oldOperationID := uuid.New()
	currentOperationID := uuid.New()
	now := time.Now().UTC()
	oldConsumedAt := now.Add(-time.Hour)
	currentExpiresAt := now.Add(time.Hour)
	item := &DigitalHumanMemory{
		ID:              memoryID,
		LastOperationID: currentOperationID,
	}

	applyMemoryUndoStatus([]*DigitalHumanMemory{item}, []MemoryUndoRecord{
		{
			OperationID: oldOperationID,
			MemoryID:    memoryID,
			ExpiresAt:   now,
			ConsumedAt:  &oldConsumedAt,
		},
		{
			OperationID: currentOperationID,
			MemoryID:    memoryID,
			ExpiresAt:   currentExpiresAt,
		},
	})

	if item.UndoConsumed {
		t.Fatal("UndoConsumed = true, want false for the current unconsumed mutation")
	}
	if item.UndoableUntil == nil || !item.UndoableUntil.Equal(currentExpiresAt) {
		t.Fatalf("UndoableUntil = %v, want %v", item.UndoableUntil, currentExpiresAt)
	}
}
