package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/agentmemoryruntime"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestAgentMemoryToolsPolicyRequiresVerifiedFunctionCalling(t *testing.T) {
	parts := &chatRequestParts{
		AgentMemoryEnabled:   true,
		AgentMemorySlots:     []AgentMemorySlotConfig{{Key: "profile", Enabled: true}},
		FunctionCallingKnown: true, ModelSupportsFunctionCalling: true,
	}
	applyAgentMemoryToolsPolicy(parts)
	if !parts.AgentMemoryToolsEnabled {
		t.Fatal("AgentMemoryToolsEnabled = false, want true")
	}
	parts.FunctionCallingAssumed = true
	applyAgentMemoryToolsPolicy(parts)
	if parts.AgentMemoryToolsEnabled {
		t.Fatal("AgentMemoryToolsEnabled = true for assumed capability")
	}
}

func TestAgentMemoryUnavailableMessagePreventsFalseConfirmation(t *testing.T) {
	message := agentMemoryUnavailableSystemMessage(&chatRequestParts{
		AgentMemoryEnabled: true,
		AgentMemorySlots:   []AgentMemorySlotConfig{{Key: "profile", Enabled: true}},
	})
	if message == nil || !strings.Contains(stringFromAny(message.Content), "do not claim") {
		t.Fatalf("unavailable message = %#v", message)
	}
}

func TestAgentMemoryRuntimeToolMutatesOneAtomicBatch(t *testing.T) {
	workspaceID, accountID, agentID := uuid.New(), uuid.New(), uuid.New()
	conversationID, messageID := uuid.New(), uuid.New()
	memory := &fakeAgentMemoryContextService{}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: conversationID},
		Message:      &runtimemodel.Message{ID: messageID, Metadata: map[string]interface{}{}},
		Scope:        Scope{WorkspaceID: &workspaceID, AccountID: accountID},
		parts: &chatRequestParts{
			Query: "Please remember that I prefer concise replies.", AgentMemoryEnabled: true,
			AgentMemoryToolsEnabled: true, AgentMemoryAgentID: agentID.String(), AgentMemoryUserScope: agentmemory.UserScopeAccount,
			AgentMemorySlots: []AgentMemorySlotConfig{{Key: "preferences", Name: "回答偏好", Enabled: true, MaxChars: 500}},
		},
	}
	svc := &service{agentMemoryService: memory}
	timeline := newProcessTimelineRecorder(context.Background(), context.Background(), svc, prepared, nil)
	tools := svc.agentMemoryRuntimeTools(context.Background(), prepared, timeline)
	if len(tools) != 1 || tools[0].Definition.Function.Name != agentmemoryruntime.ToolMutate {
		t.Fatalf("runtime tools = %#v", tools)
	}
	result := tools[0].Handler(context.Background(), adapter.ToolCall{Function: adapter.FunctionCall{
		Name:      agentmemoryruntime.ToolMutate,
		Arguments: `{"operations":[{"action":"upsert","key":"preferences","content":"Prefers concise replies.","evidence":"I prefer concise replies","mode":"explicit"}]}`,
	}})
	if result.Error != nil || memory.mutateCalls != 1 || len(memory.lastMutation.Operations) != 1 {
		t.Fatalf("result=%#v mutateCalls=%d mutation=%#v", result, memory.mutateCalls, memory.lastMutation)
	}
	if _, leaked := result.Arguments["content"]; leaked {
		t.Fatalf("runtime trace arguments leaked content: %#v", result.Arguments)
	}
	invocations := skillInvocationsFromMetadata(prepared.Message.Metadata["skill_invocations"])
	if len(invocations) != 1 {
		t.Fatalf("memory invocations = %#v, want one persisted mutation", invocations)
	}
	invocation := invocations[0]
	if invocation["kind"] != "memory_mutation" || invocation["key"] != "preferences" || invocation["display_name"] != "回答偏好" || invocation["mutation_status"] != agentmemory.MutationStatusUpdated {
		t.Fatalf("memory invocation = %#v, want sanitized preferences update", invocation)
	}
	serialized := fmt.Sprintf("%#v", invocation)
	if strings.Contains(serialized, "Prefers concise replies") || strings.Contains(serialized, "I prefer concise replies") {
		t.Fatalf("memory invocation leaked content or evidence: %s", serialized)
	}
	if prepared.Message.Metadata["memory_operation_count"] != 1 || prepared.Message.Metadata["tool_call_count"] != 0 {
		t.Fatalf("memory/tool counts = %#v/%#v, want 1/0", prepared.Message.Metadata["memory_operation_count"], prepared.Message.Metadata["tool_call_count"])
	}
}

func TestAgentMemoryRuntimeToolAllowsOneValidationCorrection(t *testing.T) {
	workspaceID, accountID, agentID := uuid.New(), uuid.New(), uuid.New()
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message:      &runtimemodel.Message{ID: uuid.New(), Metadata: map[string]interface{}{}},
		Scope:        Scope{WorkspaceID: &workspaceID, AccountID: accountID},
		parts: &chatRequestParts{
			Query: "remember concise replies", AgentMemoryEnabled: true, AgentMemoryToolsEnabled: true,
			AgentMemoryAgentID: agentID.String(), AgentMemoryUserScope: agentmemory.UserScopeAccount,
			AgentMemorySlots: []AgentMemorySlotConfig{{Key: "preferences", Enabled: true, MaxChars: 500}},
		},
	}
	memory := &fakeAgentMemoryContextService{}
	tool := (&service{agentMemoryService: memory}).agentMemoryRuntimeTools(context.Background(), prepared, nil)[0]
	invalid := tool.Handler(context.Background(), adapter.ToolCall{Function: adapter.FunctionCall{Name: agentmemoryruntime.ToolMutate, Arguments: `{"operations":[]}`}})
	if !invalid.Recoverable || invalid.Error == nil {
		t.Fatalf("invalid result = %#v", invalid)
	}
	retry := tool.Handler(context.Background(), adapter.ToolCall{Function: adapter.FunctionCall{Name: agentmemoryruntime.ToolMutate, Arguments: `{"operations":[{"action":"upsert","key":"preferences","content":"Concise replies.","evidence":"concise replies","mode":"explicit"}]}`}})
	if retry.Error != nil || memory.mutateCalls != 1 {
		t.Fatalf("retry result=%#v calls=%d", retry, memory.mutateCalls)
	}
}

func TestAgentMemorySuppressesModelInvocationBodies(t *testing.T) {
	t.Setenv("ZGI_AICHAT_MODEL_INVOCATION_DEBUG", "true")
	prepared := &PreparedChat{
		Message: &runtimemodel.Message{ID: uuid.New(), Metadata: map[string]interface{}{}},
		parts:   &chatRequestParts{AgentMemoryEnabled: true},
	}
	(&service{}).persistModelInvocationBestEffort(context.Background(), prepared, skillloop.ModelInvocationTrace{
		Request:  &adapter.ChatRequest{Provider: "test", Model: "model", Messages: []adapter.Message{{Role: "user", Content: "secret-memory-body"}}},
		Response: &adapter.Message{Role: "assistant", Content: "secret-evidence-body"},
	})
	stored := fmt.Sprintf("%#v", prepared.Message.Metadata["model_invocations"])
	if strings.Contains(stored, "secret-memory-body") || strings.Contains(stored, "secret-evidence-body") {
		t.Fatalf("model invocation metadata leaked memory content: %s", stored)
	}
}

type fakeAgentMemoryContextService struct {
	values       []agentmemory.SlotValueResponse
	mutateCalls  int
	lastMutation agentmemory.MutateValuesRequest
}

func (f *fakeAgentMemoryContextService) ReadUserMemory(context.Context, uuid.UUID, uuid.UUID, []agentmemory.RuntimeSlot, string, uuid.UUID) ([]agentmemory.SlotValueResponse, error) {
	return f.values, nil
}

func (f *fakeAgentMemoryContextService) MutateValues(_ context.Context, _, _ uuid.UUID, _ []agentmemory.RuntimeSlot, _ string, _ uuid.UUID, req agentmemory.MutateValuesRequest, _ agentmemory.MutationMetadata) (*agentmemory.MutateValuesResponse, error) {
	f.mutateCalls++
	f.lastMutation = req
	results := make([]agentmemory.ValueMutationResult, 0, len(req.Operations))
	for _, operation := range req.Operations {
		sourceKind := agentmemory.SourceKindExplicit
		if operation.Mode == agentmemory.MutationModeProactive {
			sourceKind = agentmemory.SourceKindAutomatic
		}
		results = append(results, agentmemory.ValueMutationResult{Action: operation.Action, Status: agentmemory.MutationStatusUpdated, Key: operation.Key, Revision: operation.ExpectedRevision + 1, SourceKind: sourceKind, OperationID: operation.OperationID.String()})
	}
	return &agentmemory.MutateValuesResponse{Status: "success", Operations: results}, nil
}

// This general LLM fake is shared by chat-runtime tests that assert prepare
// does not introduce an extra model invocation.
type fakeAgentMemoryPlannerLLM struct {
	response *adapter.ChatResponse
	err      error
	requests []*adapter.ChatRequest
}

func (f *fakeAgentMemoryPlannerLLM) Chat(context.Context, string, *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return nil, errors.New("unexpected Chat call")
}
func (f *fakeAgentMemoryPlannerLLM) ChatStream(context.Context, string, *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, errors.New("unexpected ChatStream call")
}
func (f *fakeAgentMemoryPlannerLLM) CreateResponse(context.Context, string, *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, errors.New("unexpected CreateResponse call")
}
func (f *fakeAgentMemoryPlannerLLM) Embed(context.Context, string, *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, errors.New("unexpected Embed call")
}
func (f *fakeAgentMemoryPlannerLLM) CreateImage(context.Context, string, *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, errors.New("unexpected CreateImage call")
}
func (f *fakeAgentMemoryPlannerLLM) Rerank(context.Context, string, *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, errors.New("unexpected Rerank call")
}
func (f *fakeAgentMemoryPlannerLLM) AppChat(_ context.Context, _ *llmclient.AppContext, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	f.requests = append(f.requests, cloneChatRequest(req))
	if f.err != nil {
		return nil, f.err
	}
	return f.response, nil
}
func (f *fakeAgentMemoryPlannerLLM) AppChatStream(context.Context, *llmclient.AppContext, *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, errors.New("unexpected AppChatStream call")
}
func (f *fakeAgentMemoryPlannerLLM) AppCreateResponse(context.Context, *llmclient.AppContext, *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, errors.New("unexpected AppCreateResponse call")
}
func (f *fakeAgentMemoryPlannerLLM) AppEmbed(context.Context, *llmclient.AppContext, *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, errors.New("unexpected AppEmbed call")
}
func (f *fakeAgentMemoryPlannerLLM) AppCreateImage(context.Context, *llmclient.AppContext, *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, errors.New("unexpected AppCreateImage call")
}
func (f *fakeAgentMemoryPlannerLLM) AppRerank(context.Context, *llmclient.AppContext, *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, errors.New("unexpected AppRerank call")
}
