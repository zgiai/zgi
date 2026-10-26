package agentmemoryruntime

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
)

type fakeMemoryService struct {
	calls   int
	request agentmemory.MutateValuesRequest
}

func (f *fakeMemoryService) ReadRuntimeFence(context.Context, uuid.UUID, uuid.UUID, string, uuid.UUID, string, string) (int64, error) {
	return 0, errors.New("unexpected epoch read")
}

func (f *fakeMemoryService) ReadUserMemory(context.Context, uuid.UUID, uuid.UUID, []agentmemory.RuntimeSlot, string, uuid.UUID) ([]agentmemory.SlotValueResponse, error) {
	return nil, errors.New("unexpected read")
}

func (f *fakeMemoryService) MutateValues(_ context.Context, _, _ uuid.UUID, _ []agentmemory.RuntimeSlot, _ string, _ uuid.UUID, request agentmemory.MutateValuesRequest, _ agentmemory.MutationMetadata) (*agentmemory.MutateValuesResponse, error) {
	f.calls++
	f.request = request
	results := make([]agentmemory.ValueMutationResult, 0, len(request.Operations))
	for _, operation := range request.Operations {
		results = append(results, agentmemory.ValueMutationResult{Action: operation.Action, Status: agentmemory.MutationStatusUpdated, Key: operation.Key, Revision: operation.ExpectedRevision + 1, OperationID: operation.OperationID.String()})
	}
	return &agentmemory.MutateValuesResponse{Status: "success", Operations: results}, nil
}
