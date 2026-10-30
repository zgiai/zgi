package skillloop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

// barrierTool returns only once two calls have started, or after a timeout,
// and reports whether it saw both: true only when the calls ran together.
type barrierTool struct{ started atomic.Int32 }

func (t *barrierTool) GetEntity() tools.ToolEntity {
	return tools.ToolEntity{
		Identity:    tools.ToolIdentity{Name: "wait_read", Provider: "barrier_test", Label: tools.I18nText{"en_US": "Wait Read"}},
		Description: tools.ToolDescription{Human: tools.I18nText{"en_US": "Read after a barrier"}, LLM: "Read after a barrier."},
	}
}
func (t *barrierTool) GetProviderType() tools.ToolProviderType { return tools.ToolProviderTypeBuiltin }
func (t *barrierTool) GetTenantID() string                     { return "" }
func (t *barrierTool) Invoke(ctx context.Context, _ string, _ map[string]interface{}, _ *string, _ *string, _ *string) ([]tools.ToolInvokeMessage, error) {
	t.started.Add(1)
	deadline := time.Now().Add(2 * time.Second)
	for t.started.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	return []tools.ToolInvokeMessage{{Type: tools.ToolInvokeMessageTypeJSON, Data: map[string]interface{}{"saw_both": t.started.Load() >= 2}}}, nil
}
func (t *barrierTool) GetRuntimeParameters(context.Context, *string, *string, *string) ([]tools.ToolParameter, error) {
	return nil, nil
}
func (t *barrierTool) ForkToolRuntime(*tools.ToolRuntime) tools.Tool                     { return t }
func (t *barrierTool) ValidateCredentials(context.Context, map[string]interface{}) error { return nil }

type barrierProvider struct{ tool *barrierTool }

func (p *barrierProvider) GetEntity() tools.ToolProviderEntity {
	return tools.ToolProviderEntity{Identity: tools.ToolProviderIdentity{Name: "barrier_test", Label: tools.I18nText{"en_US": "Barrier"}, Description: tools.I18nText{"en_US": "Barrier"}}, ProviderType: tools.ToolProviderTypeBuiltin}
}
func (p *barrierProvider) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeBuiltin
}
func (p *barrierProvider) GetTool(name string) (tools.Tool, error) {
	if name != "wait_read" {
		return nil, tools.ErrToolNotFound
	}
	return p.tool, nil
}
func (p *barrierProvider) GetTools() []tools.Tool { return []tools.Tool{p.tool} }
func (p *barrierProvider) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

func barrierSkillRuntime(t *testing.T, sideEffects string) (*skills.Runtime, *skills.ResolvedSkills) {
	t.Helper()
	catalogDir := t.TempDir()
	skillDir := filepath.Join(catalogDir, "barrier-reader")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	frontmatter := "---\nname: barrier-reader\ndescription: Reads after a barrier.\nwhen_to_use: Use it in tests.\nprovider_type: builtin\nprovider_id: barrier_test\nruntime_type: tool\ntools:\n  - wait_read\nsupported_callers:\n  - agent\n"
	if sideEffects != "" {
		frontmatter += "side_effects: " + sideEffects + "\n"
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(frontmatter+"---\nRead after a barrier.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(&barrierProvider{tool: &barrierTool{}}); err != nil {
		t.Fatal(err)
	}
	runtime := skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir)
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{"barrier-reader"})
	if err != nil {
		t.Fatal(err)
	}
	return runtime, resolved
}

func barrierCalls(ids ...string) []adapter.ToolCall {
	calls := make([]adapter.ToolCall, 0, len(ids))
	for _, id := range ids {
		arguments, _ := json.Marshal(map[string]interface{}{"skill_id": "barrier-reader", "tool_name": "wait_read", "arguments": map[string]interface{}{}})
		calls = append(calls, adapter.ToolCall{ID: id, Type: "function", Function: adapter.FunctionCall{Name: skills.MetaToolCallSkillTool, Arguments: string(arguments)}})
	}
	return calls
}

func sawBoth(t *testing.T, invocation *skills.ToolInvocationResult, err error) bool {
	t.Helper()
	if err != nil || invocation == nil || len(invocation.Messages) == 0 {
		t.Fatalf("invocation = %#v, err = %v", invocation, err)
	}
	saw, _ := invocation.Messages[0].Data["saw_both"].(bool)
	return saw
}

func TestReadOnlyCallsOfARoundRunTogether(t *testing.T) {
	runtime, resolved := barrierSkillRuntime(t, "none")
	runner := &Runner{SkillRuntime: runtime}
	loaded := map[string]struct{}{"barrier-reader": {}}
	runner.prefetchedCalls = runner.prefetchReadOnlyCalls(context.Background(), RunRequest{}, resolved, nil, loaded, nil, barrierCalls("first", "second"))
	if len(runner.prefetchedCalls) != 2 {
		t.Fatalf("started %d calls ahead, want 2", len(runner.prefetchedCalls))
	}
	for _, id := range []string{"first", "second"} {
		invocation, err := runner.callSkillTool(context.Background(), resolved, "barrier-reader", "wait_read", map[string]interface{}{}, skills.ExecutionContext{}, id)
		if !sawBoth(t, invocation, err) {
			t.Fatalf("call %s ran alone, want both calls running together", id)
		}
	}
	if len(runner.prefetchedCalls) != 0 {
		t.Fatalf("unconsumed calls = %d, want none", len(runner.prefetchedCalls))
	}
}

func TestCallsThatMayChangeSomethingStayInOrder(t *testing.T) {
	runtime, resolved := barrierSkillRuntime(t, "")
	runner := &Runner{SkillRuntime: runtime}
	loaded := map[string]struct{}{"barrier-reader": {}}
	if started := runner.prefetchReadOnlyCalls(context.Background(), RunRequest{}, resolved, nil, loaded, nil, barrierCalls("first", "second")); started != nil {
		t.Fatalf("started %d calls of a skill that declares no effects, want none", len(started))
	}
	if started := (&Runner{SkillRuntime: runtime}).prefetchReadOnlyCalls(context.Background(), RunRequest{
		AuthorizeSkillStep: func(context.Context, string) (bool, error) { return false, nil },
	}, resolved, nil, loaded, nil, barrierCalls("first", "second")); started != nil {
		t.Fatalf("started calls the organization's policy refuses")
	}
}

func TestASingleReadOnlyCallIsNotStartedAhead(t *testing.T) {
	runtime, resolved := barrierSkillRuntime(t, "none")
	runner := &Runner{SkillRuntime: runtime}
	if started := runner.prefetchReadOnlyCalls(context.Background(), RunRequest{}, resolved, nil, map[string]struct{}{"barrier-reader": {}}, nil, barrierCalls("only")); started != nil {
		t.Fatalf("started a lone call ahead")
	}
}

// Read-only runtime tools, such as a DigitalHuman's memory and task reads, run
// together like read-only skill tools; other runtime tools stay in order.
func TestReadOnlyRuntimeToolCallsRunTogether(t *testing.T) {
	var started atomic.Int32
	barrier := func(_ context.Context, _ adapter.ToolCall) RuntimeToolResult {
		started.Add(1)
		deadline := time.Now().Add(2 * time.Second)
		for started.Load() < 2 && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		return RuntimeToolResult{Status: "success", Result: map[string]interface{}{"saw_both": started.Load() >= 2}}
	}
	tool := func(name string, readOnly bool) RuntimeTool {
		return RuntimeTool{
			Definition: adapter.Tool{Type: "function", Function: adapter.Function{Name: name}},
			SkillID:    "digital-human-memory",
			Handler:    barrier,
			ReadOnly:   readOnly,
		}
	}
	call := func(id, name string) adapter.ToolCall {
		return adapter.ToolCall{ID: id, Type: "function", Function: adapter.FunctionCall{Name: name, Arguments: "{}"}}
	}
	runtimeTools := []RuntimeTool{tool("memory_view", true), tool("memory_search", true), tool("memory_edit", false)}
	runner := &Runner{}
	calls := []adapter.ToolCall{call("a", "memory_view"), call("b", "memory_search"), call("c", "memory_edit")}
	runner.prefetchedCalls = runner.prefetchReadOnlyCalls(context.Background(), RunRequest{RuntimeTools: runtimeTools}, nil, nil, nil, nil, calls)
	if len(runner.prefetchedCalls) != 2 || runner.prefetchedCalls["c"] != nil {
		t.Fatalf("prefetched = %v, want the two reads only", runner.prefetchedCalls)
	}
	for _, item := range calls[:2] {
		tool, _ := runtimeToolForCall(runtimeTools, item)
		result := runner.callRuntimeTool(context.Background(), item, tool)
		if result.toolResult["saw_both"] != true {
			t.Fatalf("%s: result = %#v, want it run alongside the other read", item.ID, result.toolResult)
		}
	}
	if len(runner.prefetchedCalls) != 0 {
		t.Fatalf("prefetched calls left after use: %v", runner.prefetchedCalls)
	}
}
