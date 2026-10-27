package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
)

type dualAgentMemoryBackend struct{}

func (*dualAgentMemoryBackend) ReadRuntimeFence(context.Context, uuid.UUID, uuid.UUID, string, uuid.UUID, string, string) (int64, error) {
	return 0, nil
}

func (*dualAgentMemoryBackend) ReadUserMemory(context.Context, uuid.UUID, uuid.UUID, []agentmemory.RuntimeSlot, string, uuid.UUID) ([]agentmemory.SlotValueResponse, error) {
	return nil, nil
}

func (*dualAgentMemoryBackend) MutateValues(context.Context, uuid.UUID, uuid.UUID, []agentmemory.RuntimeSlot, string, uuid.UUID, agentmemory.MutateValuesRequest, agentmemory.MutationMetadata) (*agentmemory.MutateValuesResponse, error) {
	return nil, nil
}

func TestNewServiceRegistersEveryContractImplementedByOptionalBackend(t *testing.T) {
	backend := &dualAgentMemoryBackend{}
	constructed := NewServiceWithSkillRuntime(nil, nil, nil, nil, nil, nil, nil, nil, nil, backend).(*service)

	if constructed.agentMemoryService != backend {
		t.Fatal("dual backend was not registered as Agent memory context service")
	}
}
