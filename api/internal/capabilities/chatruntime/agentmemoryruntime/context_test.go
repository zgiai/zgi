package agentmemoryruntime

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
)

func TestBuildContextCapturesEpochBeforeReadingValues(t *testing.T) {
	memory := &contextMemoryService{epoch: 9}
	result, err := BuildContext(context.Background(), ContextRequest{
		Enabled: true, MemoryService: memory, WorkspaceID: uuid.New(), AgentID: uuid.New(), UserID: uuid.New(),
		UserScope: agentmemory.UserScopeAccount, ConfigScope: agentmemory.ConfigScopePublished, ConfigRevision: "revision-1", Budget: 1024,
		Slots: []Slot{{Key: "profile", Enabled: true, MaxChars: 500}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State == nil || result.State.MemoryEpoch == nil || *result.State.MemoryEpoch != memory.epoch {
		t.Fatalf("state epoch = %#v, want %d", result.State, memory.epoch)
	}
	if !reflect.DeepEqual(memory.calls, []string{"epoch", "values"}) {
		t.Fatalf("memory calls = %#v, want epoch before values", memory.calls)
	}
}

type contextMemoryService struct {
	epoch int64
	calls []string
}

func (f *contextMemoryService) ReadRuntimeFence(context.Context, uuid.UUID, uuid.UUID, string, uuid.UUID, string, string) (int64, error) {
	f.calls = append(f.calls, "epoch")
	return f.epoch, nil
}

func (f *contextMemoryService) ReadUserMemory(context.Context, uuid.UUID, uuid.UUID, []agentmemory.RuntimeSlot, string, uuid.UUID) ([]agentmemory.SlotValueResponse, error) {
	f.calls = append(f.calls, "values")
	return []agentmemory.SlotValueResponse{}, nil
}

func (f *contextMemoryService) MutateValues(context.Context, uuid.UUID, uuid.UUID, []agentmemory.RuntimeSlot, string, uuid.UUID, agentmemory.MutateValuesRequest, agentmemory.MutationMetadata) (*agentmemory.MutateValuesResponse, error) {
	return nil, nil
}
