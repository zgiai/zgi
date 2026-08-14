package agentmemoryruntime

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
)

func TestExecuteValidatesWholeBatchBeforeMutation(t *testing.T) {
	service := &fakeMemoryService{}
	request := mutationTestRequest(service)
	result := Execute(context.Background(), request, `{"operations":[
		{"action":"upsert","key":"profile","content":"Name is Ada.","evidence":"My name is Ada","mode":"explicit"},
		{"action":"upsert","key":"preferences","content":"Prefers concise replies.","evidence":"not in the message","mode":"explicit"}
	]}`)
	if !ValidationError(result.Error) || service.calls != 0 {
		t.Fatalf("result=%#v calls=%d", result, service.calls)
	}
}

func TestExecuteRejectsProactiveClear(t *testing.T) {
	service := &fakeMemoryService{}
	result := Execute(context.Background(), mutationTestRequest(service), `{"operations":[{"action":"clear","key":"profile","evidence":"I prefer concise replies","mode":"proactive"}]}`)
	if !ValidationError(result.Error) || service.calls != 0 {
		t.Fatalf("result=%#v calls=%d", result, service.calls)
	}
}

func TestExecuteAllowsProactiveWriteToAnyEnabledSlot(t *testing.T) {
	service := &fakeMemoryService{}
	result := Execute(context.Background(), mutationTestRequest(service), `{"operations":[{"action":"upsert","key":"standing_instructions","content":"Prefers concise replies.","evidence":"I prefer concise replies","mode":"proactive"}]}`)
	if result.Error != nil || service.calls != 1 {
		t.Fatalf("result=%#v calls=%d", result, service.calls)
	}
}

func TestExecuteUsesLegacyExplicitOperationIDAndSafeResult(t *testing.T) {
	service := &fakeMemoryService{}
	request := mutationTestRequest(service)
	result := Execute(context.Background(), request, `{"operations":[{"action":"upsert","key":"profile","content":"Name is Ada.","evidence":"My name is Ada","mode":"explicit"}]}`)
	if result.Error != nil || service.calls != 1 {
		t.Fatalf("result=%#v calls=%d", result, service.calls)
	}
	want := uuid.NewSHA1(uuid.NameSpaceOID, []byte(request.MutationMetadata.SourceMessageID.String()+":profile:"+legacyToolUpdate))
	if got := service.request.Operations[0].OperationID; got != want {
		t.Fatalf("operation id = %s, want %s", got, want)
	}
	if _, exists := result.Result["content"]; exists {
		t.Fatalf("safe result leaked content: %#v", result.Result)
	}
}

func TestToolsExposeOnlyAtomicMutationFunction(t *testing.T) {
	tools := Tools([]Slot{{Key: "profile", Enabled: true, MaxChars: 500}}, true)
	if len(tools) != 1 || tools[0].Function.Name != ToolMutate {
		t.Fatalf("tools = %#v", tools)
	}
}

func mutationTestRequest(service MemoryService) MutationRequest {
	messageID := uuid.New()
	return MutationRequest{
		MemoryService: service, WorkspaceID: uuid.New(), AgentID: uuid.New(), UserID: uuid.New(), UserScope: agentmemory.UserScopeAccount,
		LatestUserMessage: "My name is Ada and I prefer concise replies.", ProactiveAllowed: true,
		Slots: []Slot{
			{Key: "profile", Enabled: true, MaxChars: 500},
			{Key: "preferences", Enabled: true, MaxChars: 500},
			{Key: "standing_instructions", Enabled: true, MaxChars: 500},
		},
		MutationMetadata: agentmemory.MutationMetadata{SourceMessageID: &messageID},
	}
}

type fakeMemoryService struct {
	calls   int
	request agentmemory.MutateValuesRequest
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
