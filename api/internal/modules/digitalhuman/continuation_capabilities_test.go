package digitalhuman

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type fakeTaskRuntimeProvider struct{ preparation *TaskRuntimePreparation }

func (p *fakeTaskRuntimeProvider) Prepare(context.Context, *Run, invocation.Event) (*TaskRuntimePreparation, error) {
	copied := *p.preparation
	copied.Tools = append([]skillloop.RuntimeTool(nil), p.preparation.Tools...)
	return &copied, nil
}

func capabilityTestTool(name string) skillloop.RuntimeTool {
	return skillloop.RuntimeTool{Definition: adapter.Tool{Type: "function", Function: adapter.Function{Name: name}}}
}

func runtimeToolNames(config chatruntime.RunConfig) []string {
	names := make([]string, 0, len(config.RuntimeTools))
	for _, tool := range config.RuntimeTools {
		names = append(names, tool.Definition.Function.Name)
	}
	return names
}

// A continuation must send the system prompt and tools of the run it
// continues, or it cannot reuse the provider's cached prefix (ADR 0032).
func TestContinuationsPrepareWhatTheFirstRunHad(t *testing.T) {
	store, dispatcher, chat := newFakeRunStore(), &fakeRunDispatcher{}, &fakeDigitalHumanChat{}
	now := time.Date(2026, 8, 18, 6, 0, 0, 0, time.UTC)
	memory := &fakeMemoryRuntimeProvider{preparation: &MemoryRuntimePreparation{
		StableGuidance:       "Memory rules.",
		SystemPromptAddendum: "Memory files.",
		Tools:                []skillloop.RuntimeTool{capabilityTestTool(memoryEditToolName)},
	}}
	tasks := &fakeTaskRuntimeProvider{preparation: &TaskRuntimePreparation{
		StableGuidance: "Task rules.",
		Tools:          []skillloop.RuntimeTool{capabilityTestTool("list_digital_human_tasks"), capabilityTestTool(taskManageToolName)},
	}}
	service := NewRuntimeService(store, fakeExecutionResolver{now: now}, fakeBrainAdapter{}, chat, dispatcher, WithMemoryRuntime(memory), WithTaskRuntime(tasks))
	service.now = func() time.Time { return now }
	run, _, err := service.SubmitManual(t.Context(), testRunScope(), uuid.New(), ManualInvocationRequest{RequestID: uuid.New(), Query: "remind me tomorrow"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Execute(t.Context(), run.ID); err != nil {
		t.Fatal(err)
	}
	first := chat.config
	if first.SystemPrompt != "Researcher\n\nMemory rules.\n\nTask rules." {
		t.Fatalf("first run system prompt = %q", first.SystemPrompt)
	}
	stored := store.runs[run.ID]
	resolved, event, err := decodeAndVerifyRun(stored)
	if err != nil {
		t.Fatal(err)
	}

	userInput, _, err := service.userInputContinuationConfig(t.Context(), stored, resolved, event, testRunScope())
	if err != nil {
		t.Fatal(err)
	}
	approved, err := service.taskApprovalContinuationConfigForRun(t.Context(), stored, true)
	if err != nil {
		t.Fatal(err)
	}
	for name, config := range map[string]chatruntime.RunConfig{"user input": userInput, "task approval": approved} {
		if config.SystemPrompt != first.SystemPrompt || !reflect.DeepEqual(runtimeToolNames(config), runtimeToolNames(first)) {
			t.Fatalf("%s continuation: system prompt %q, tools %v; first run: %q, %v",
				name, config.SystemPrompt, runtimeToolNames(config), first.SystemPrompt, runtimeToolNames(first))
		}
		if config.TurnContext != first.TurnContext {
			t.Fatalf("%s continuation turn context = %q, want the first run's %q", name, config.TurnContext, first.TurnContext)
		}
	}

	readOnly, err := service.taskApprovalContinuationConfigForRun(t.Context(), stored, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := runtimeToolNames(readOnly); !reflect.DeepEqual(got, runtimeToolNames(first)) || readOnly.SystemPrompt != first.SystemPrompt {
		t.Fatalf("read-only task approval tools = %v, want the first run's %v", got, runtimeToolNames(first))
	}
	for _, tool := range readOnly.RuntimeTools {
		if tool.Definition.Function.Name != taskManageToolName {
			continue
		}
		result := tool.Handler(t.Context(), adapter.ToolCall{ID: "again", Function: adapter.FunctionCall{Name: taskManageToolName, Arguments: `{"action":"create"}`}})
		if result.Status != "failed" || result.Result["error_code"] != taskApprovalAppliedCode {
			t.Fatalf("task change after an applied approval = %#v, want it refused", result)
		}
	}
}

func TestAContinuationSoonAfterItsRunKeepsTheRunsTime(t *testing.T) {
	started := time.Date(2026, 8, 18, 6, 0, 30, 0, time.UTC)
	service := &RuntimeService{}
	run := &Run{StartedAt: &started}
	for _, tt := range []struct {
		now  time.Time
		want time.Time
	}{
		{started.Add(4 * time.Minute), started},
		{started.Add(20 * time.Minute), started.Add(20 * time.Minute)},
	} {
		service.now = func() time.Time { return tt.now }
		if got := service.runTurnTime(run); !got.Equal(tt.want) {
			t.Fatalf("runTurnTime at %s = %s, want %s", tt.now, got, tt.want)
		}
	}
	service.now = func() time.Time { return started }
	if got := service.runTurnTime(&Run{}); !got.Equal(started) {
		t.Fatalf("runTurnTime without a start = %s, want now", got)
	}
}
