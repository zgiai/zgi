package memoryplanner

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

type fakePlannerLLM struct {
	responses []string
	usage     []*adapter.Usage
	err       error
	calls     int
	waitCtx   bool
}

func (f *fakePlannerLLM) AppChat(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	_ = appCtx
	_ = req
	index := f.calls
	f.calls++
	if f.waitCtx {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.err != nil {
		return nil, f.err
	}
	if index >= len(f.responses) {
		index = len(f.responses) - 1
	}
	resp := &adapter.ChatResponse{
		Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: f.responses[index]}}},
	}
	if index >= 0 && index < len(f.usage) {
		resp.Usage = f.usage[index]
	}
	return resp, nil
}

func (f *fakePlannerLLM) Chat(context.Context, string, *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return nil, nil
}
func (f *fakePlannerLLM) ChatStream(context.Context, string, *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, nil
}
func (f *fakePlannerLLM) CreateResponse(context.Context, string, *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, nil
}
func (f *fakePlannerLLM) Embed(context.Context, string, *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, nil
}
func (f *fakePlannerLLM) CreateImage(context.Context, string, *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, nil
}
func (f *fakePlannerLLM) Rerank(context.Context, string, *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, nil
}
func (f *fakePlannerLLM) AppChatStream(context.Context, *llmclient.AppContext, *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, nil
}
func (f *fakePlannerLLM) AppCreateResponse(context.Context, *llmclient.AppContext, *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, nil
}
func (f *fakePlannerLLM) AppEmbed(context.Context, *llmclient.AppContext, *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, nil
}
func (f *fakePlannerLLM) AppCreateImage(context.Context, *llmclient.AppContext, *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, nil
}
func (f *fakePlannerLLM) AppRerank(context.Context, *llmclient.AppContext, *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, nil
}

type plannerDecision struct {
	action string
}

type fakePlannerDomain struct {
	applied int
}

func (d *fakePlannerDomain) PlannerMessages(baseMessages []adapter.Message) []adapter.Message {
	return append(baseMessages, adapter.Message{Role: "system", Content: "plan"})
}

func (d *fakePlannerDomain) ParseDecision(raw string) (interface{}, error) {
	switch strings.TrimSpace(raw) {
	case "bad":
		return nil, errors.New("bad json")
	case "none":
		return plannerDecision{action: "none"}, nil
	default:
		return plannerDecision{action: "update"}, nil
	}
}

func (d *fakePlannerDomain) IsNoop(decision interface{}) bool {
	typed, _ := decision.(plannerDecision)
	return typed.action == "none"
}

func (d *fakePlannerDomain) NoopStatus(interface{}) string {
	return "success_none"
}

func (d *fakePlannerDomain) PlannerSuccessStatus(interface{}) string {
	return "success_update"
}

func (d *fakePlannerDomain) PlannerTrace(_ interface{}, status string, err error) skills.SkillTrace {
	trace := skills.SkillTrace{Kind: "memory_planner", Status: status}
	if err != nil {
		trace.Error = err.Error()
	}
	return trace
}

func (d *fakePlannerDomain) ApplyDecision(context.Context, interface{}) (map[string]interface{}, skills.SkillTrace, error) {
	d.applied++
	return map[string]interface{}{"entry_id": "m1"}, skills.SkillTrace{Kind: "memory", Status: "success"}, nil
}

func (d *fakePlannerDomain) SuccessNote(interface{}, map[string]interface{}) adapter.Message {
	return adapter.Message{Role: "system", Content: "success note"}
}

func (d *fakePlannerDomain) GuardNote(status string) adapter.Message {
	return adapter.Message{Role: "system", Content: "guard " + status}
}

func (d *fakePlannerDomain) MetadataUpdates(_ interface{}, plannerStatus string, _ map[string]interface{}, mutationStatus string) map[string]interface{} {
	return map[string]interface{}{"planner_status": plannerStatus, "mutation_status": mutationStatus}
}

func TestRunAppliesMutationAndMergesUsage(t *testing.T) {
	llm := &fakePlannerLLM{
		responses: []string{"update"},
		usage:     []*adapter.Usage{{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5}},
	}
	domain := &fakePlannerDomain{}
	result := Run(context.Background(), Request{
		LatestUserMessage: "remember this",
		LLMRequest:        &adapter.ChatRequest{Messages: []adapter.Message{{Role: "user", Content: "remember this"}}},
		LLMClient:         llm,
	}, domain)

	if domain.applied != 1 {
		t.Fatalf("applied = %d, want 1", domain.applied)
	}
	if result.Usage == nil || result.Usage.TotalTokens != 5 {
		t.Fatalf("usage = %#v, want total 5", result.Usage)
	}
	if got := result.MetadataUpdates["mutation_status"]; got != "success" {
		t.Fatalf("mutation_status = %#v, want success", got)
	}
	if len(result.Messages) == 0 || result.Messages[len(result.Messages)-1].Content != "success note" {
		t.Fatalf("messages = %#v, want success note appended", result.Messages)
	}
}

func TestRunNoopAddsGuardNoteWithoutMutation(t *testing.T) {
	llm := &fakePlannerLLM{responses: []string{"none"}}
	domain := &fakePlannerDomain{}
	result := Run(context.Background(), Request{
		LatestUserMessage: "hello",
		LLMRequest:        &adapter.ChatRequest{Messages: []adapter.Message{{Role: "user", Content: "hello"}}},
		LLMClient:         llm,
	}, domain)

	if domain.applied != 0 {
		t.Fatalf("applied = %d, want 0", domain.applied)
	}
	if got := result.MetadataUpdates["planner_status"]; got != "success_none" {
		t.Fatalf("planner_status = %#v, want success_none", got)
	}
	if len(result.Messages) == 0 || !strings.Contains(result.Messages[len(result.Messages)-1].Content.(string), "guard") {
		t.Fatalf("messages = %#v, want guard note", result.Messages)
	}
}

func TestRunRetriesInvalidJSON(t *testing.T) {
	llm := &fakePlannerLLM{responses: []string{"bad", "update"}}
	domain := &fakePlannerDomain{}
	result := Run(context.Background(), Request{
		LatestUserMessage: "remember this",
		LLMRequest:        &adapter.ChatRequest{Messages: []adapter.Message{{Role: "user", Content: "remember this"}}},
		LLMClient:         llm,
	}, domain)

	if llm.calls != 2 {
		t.Fatalf("calls = %d, want 2", llm.calls)
	}
	if domain.applied != 1 {
		t.Fatalf("applied = %d, want 1", domain.applied)
	}
	if got := result.MetadataUpdates["planner_status"]; got != "success_update" {
		t.Fatalf("planner_status = %#v, want success_update", got)
	}
}

func TestRunTimesOutPlannerLLMAndAddsGuardNote(t *testing.T) {
	llm := &fakePlannerLLM{waitCtx: true}
	domain := &fakePlannerDomain{}
	start := time.Now()
	result := Run(context.Background(), Request{
		LatestUserMessage: "remember this",
		LLMRequest:        &adapter.ChatRequest{Messages: []adapter.Message{{Role: "user", Content: "remember this"}}},
		LLMClient:         llm,
		PlanningTimeout:   10 * time.Millisecond,
	}, domain)

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("planner timeout took %s, want under 1s", elapsed)
	}
	if domain.applied != 0 {
		t.Fatalf("applied = %d, want 0", domain.applied)
	}
	if got := result.MetadataUpdates["planner_status"]; got != "error_llm" {
		t.Fatalf("planner_status = %#v, want error_llm", got)
	}
	if len(result.Messages) == 0 || !strings.Contains(result.Messages[len(result.Messages)-1].Content.(string), "guard error_llm") {
		t.Fatalf("messages = %#v, want error guard note", result.Messages)
	}
}
