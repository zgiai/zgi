package digitalhuman

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type pendingChangeSourceFunc func(context.Context, Scope, uuid.UUID, []uuid.UUID) (map[uuid.UUID]TaskPendingChange, error)

func (f pendingChangeSourceFunc) PendingTaskChanges(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, taskIDs []uuid.UUID) (map[uuid.UUID]TaskPendingChange, error) {
	return f(ctx, scope, digitalHumanID, taskIDs)
}

func TestWithPendingChangesMarksOnlyTasksWaitingForTheReader(t *testing.T) {
	waiting, idle := &Task{ID: uuid.New()}, &Task{ID: uuid.New()}
	change := TaskPendingChange{ConversationID: uuid.New(), MessageID: uuid.New()}
	var asked []uuid.UUID
	source := pendingChangeSourceFunc(func(_ context.Context, _ Scope, _ uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]TaskPendingChange, error) {
		asked = ids
		return map[uuid.UUID]TaskPendingChange{waiting.ID: change}, nil
	})

	withPendingChanges(context.Background(), source, Scope{ActorID: uuid.New()}, uuid.New(), []*Task{waiting, idle, nil})

	if len(asked) != 2 {
		t.Fatalf("asked for %d tasks, want 2", len(asked))
	}
	if waiting.PendingChange == nil || *waiting.PendingChange != change {
		t.Fatalf("waiting task change = %#v, want %#v", waiting.PendingChange, change)
	}
	if idle.PendingChange != nil {
		t.Fatalf("idle task marked: %#v", idle.PendingChange)
	}
}

func TestWithPendingChangesKeepsTheReadWhenLookupFails(t *testing.T) {
	task := &Task{ID: uuid.New()}
	source := pendingChangeSourceFunc(func(context.Context, Scope, uuid.UUID, []uuid.UUID) (map[uuid.UUID]TaskPendingChange, error) {
		return nil, errors.New("database unavailable")
	})
	withPendingChanges(context.Background(), source, Scope{}, uuid.New(), []*Task{task})
	withPendingChanges(context.Background(), nil, Scope{}, uuid.New(), []*Task{task})
	if task.PendingChange != nil {
		t.Fatalf("task marked without a lookup result: %#v", task.PendingChange)
	}
}
