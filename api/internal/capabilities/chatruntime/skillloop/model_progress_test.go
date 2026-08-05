package skillloop

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestNativeModelProgressNonStreamingEmitsAllStages(t *testing.T) {
	client := &runnerTestLLMClient{
		appChatDelays: []time.Duration{80 * time.Millisecond},
		appChatResponses: []*adapter.ChatResponse{{Choices: []adapter.Choice{{
			Message:      adapter.Message{Role: "assistant", Content: "done"},
			FinishReason: "stop",
		}}}},
	}
	events, onEvent := modelProgressEventCollector()
	runner := &Runner{
		LLMClient:        client,
		SkillRuntime:     skills.NewRuntime(nil, nil),
		AppContext:       &llmclient.AppContext{},
		ModelIdleTimeout: time.Second,
		ModelProgressSchedule: ModelProgressSchedule{
			Initial:     5 * time.Millisecond,
			Extended:    15 * time.Millisecond,
			LongRunning: 30 * time.Millisecond,
		},
		OnEvent: onEvent,
	}
	prepared := NewPreparedChat("progress-conversation", "progress-message", "", "auto", &adapter.ChatRequest{
		Model:    "non-stream-model",
		Messages: []adapter.Message{{Role: "user", Content: "work"}},
	})

	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:                   prepared,
		Resolved:                   &skills.ResolvedSkills{},
		ProtocolToolsOnly:          true,
		NativeAgentLoop:            true,
		NativeModelProgressEnabled: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "done" {
		t.Fatalf("Run() answer = %q, want done", answer)
	}
	assertModelProgressStages(t, events(), []string{
		modelProgressStageInitial,
		modelProgressStageExtended,
		modelProgressStageLongRunning,
	})
	assertModelProgressActivity(t, events(), modelProgressActivityAwaitingResponse, modelProgressSourceRuntime)
}

func TestNativeModelProgressShortCallDoesNotFlicker(t *testing.T) {
	client := &runnerTestLLMClient{appChatResponses: []*adapter.ChatResponse{{Choices: []adapter.Choice{{
		Message:      adapter.Message{Role: "assistant", Content: "done"},
		FinishReason: "stop",
	}}}}}
	events, onEvent := modelProgressEventCollector()
	runner := &Runner{
		LLMClient:    client,
		SkillRuntime: skills.NewRuntime(nil, nil),
		AppContext:   &llmclient.AppContext{},
		ModelProgressSchedule: ModelProgressSchedule{
			Initial:     50 * time.Millisecond,
			Extended:    100 * time.Millisecond,
			LongRunning: 150 * time.Millisecond,
		},
		OnEvent: onEvent,
	}
	prepared := NewPreparedChat("short-conversation", "short-message", "", "auto", &adapter.ChatRequest{
		Model:    "non-stream-model",
		Messages: []adapter.Message{{Role: "user", Content: "work"}},
	})

	_, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:                   prepared,
		Resolved:                   &skills.ResolvedSkills{},
		ProtocolToolsOnly:          true,
		NativeAgentLoop:            true,
		NativeModelProgressEnabled: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertModelProgressStages(t, events(), nil)
}

func TestNativeModelProgressCanBeDisabled(t *testing.T) {
	client := &runnerTestLLMClient{
		appChatDelays: []time.Duration{40 * time.Millisecond},
		appChatResponses: []*adapter.ChatResponse{{Choices: []adapter.Choice{{
			Message:      adapter.Message{Role: "assistant", Content: "done"},
			FinishReason: "stop",
		}}}},
	}
	events, onEvent := modelProgressEventCollector()
	runner := &Runner{
		LLMClient:    client,
		SkillRuntime: skills.NewRuntime(nil, nil),
		AppContext:   &llmclient.AppContext{},
		ModelProgressSchedule: ModelProgressSchedule{
			Initial:     5 * time.Millisecond,
			Extended:    10 * time.Millisecond,
			LongRunning: 20 * time.Millisecond,
		},
		OnEvent: onEvent,
	}
	prepared := NewPreparedChat("disabled-conversation", "disabled-message", "", "auto", &adapter.ChatRequest{
		Model:    "non-stream-model",
		Messages: []adapter.Message{{Role: "user", Content: "work"}},
	})

	_, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:          prepared,
		Resolved:          &skills.ResolvedSkills{},
		ProtocolToolsOnly: true,
		NativeAgentLoop:   true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertModelProgressStages(t, events(), nil)
}

func TestNativeModelProgressStreamIncludesOpenWaitAndTracksReasoningDelta(t *testing.T) {
	client := &runnerTestLLMClient{
		appChatStreams: [][]adapter.StreamResponse{{
			{Choices: []adapter.StreamChoice{{Delta: adapter.Message{ReasoningContent: "hidden reasoning"}}}},
			{Choices: []adapter.StreamChoice{{Delta: adapter.Message{Content: "done"}}}},
			{Choices: []adapter.StreamChoice{{FinishReason: "stop"}}, Done: true},
		}},
		appChatStreamOpenDelays: []time.Duration{12 * time.Millisecond},
		appChatStreamResponseDelays: [][]time.Duration{{
			5 * time.Millisecond,
			35 * time.Millisecond,
			0,
		}},
	}
	events, onEvent := modelProgressEventCollector()
	runner := &Runner{
		LLMClient:        client,
		SkillRuntime:     skills.NewRuntime(nil, nil),
		AppContext:       &llmclient.AppContext{},
		ModelIdleTimeout: time.Second,
		ModelProgressSchedule: ModelProgressSchedule{
			Initial:     5 * time.Millisecond,
			Extended:    20 * time.Millisecond,
			LongRunning: 40 * time.Millisecond,
		},
		OnEvent: onEvent,
	}
	prepared := NewPreparedChat("stream-conversation", "stream-message", "qwen", "auto", &adapter.ChatRequest{
		Model:    "qwen-plus",
		Messages: []adapter.Message{{Role: "user", Content: "work"}},
	})

	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:                   prepared,
		Resolved:                   &skills.ResolvedSkills{},
		ProtocolToolsOnly:          true,
		NativeAgentLoop:            true,
		NativeModelProgressEnabled: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "done" {
		t.Fatalf("Run() answer = %q, want done", answer)
	}
	assertModelProgressIncludesStages(t, events(), []string{
		modelProgressStageInitial,
		modelProgressStageExtended,
		modelProgressStageLongRunning,
	})
	assertModelProgressActivity(t, events(), modelProgressActivityReasoning, modelProgressSourceProviderSignal)
}

func TestInitialModelProgressActivityUsesCompletedBusinessToolAfterLatestUser(t *testing.T) {
	businessCall := adapter.ToolCall{
		ID: "business-1",
		Function: adapter.FunctionCall{
			Name: "generate_report",
		},
	}
	activateCall := adapter.ToolCall{
		ID: "activate-1",
		Function: adapter.FunctionCall{
			Name: skills.MetaToolActivateSkills,
		},
	}
	tests := []struct {
		name     string
		messages []adapter.Message
		want     string
	}{
		{
			name: "business result",
			messages: []adapter.Message{
				{Role: "user", Content: "create a report"},
				{Role: "assistant", ToolCalls: []adapter.ToolCall{businessCall}},
				{Role: "tool", ToolCallID: businessCall.ID, Content: `{"status":"success"}`},
			},
			want: modelProgressActivityReviewingToolResult,
		},
		{
			name: "hidden control result",
			messages: []adapter.Message{
				{Role: "user", Content: "create a report"},
				{Role: "assistant", ToolCalls: []adapter.ToolCall{activateCall}},
				{Role: "tool", ToolCallID: activateCall.ID, Content: `{"status":"success"}`},
			},
			want: modelProgressActivityAwaitingResponse,
		},
		{
			name: "business result belongs to previous user turn",
			messages: []adapter.Message{
				{Role: "user", Content: "create a report"},
				{Role: "assistant", ToolCalls: []adapter.ToolCall{businessCall}},
				{Role: "tool", ToolCallID: businessCall.ID, Content: `{"status":"success"}`},
				{Role: "user", Content: "now summarize it"},
			},
			want: modelProgressActivityAwaitingResponse,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := initialModelProgressActivity(test.messages); got != test.want {
				t.Fatalf("initialModelProgressActivity() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestNativeModelProgressStopsOnTimeout(t *testing.T) {
	client := &runnerTestLLMClient{
		appChatDelays:    []time.Duration{time.Second},
		appChatResponses: []*adapter.ChatResponse{{}},
	}
	events, onEvent := modelProgressEventCollector()
	runner := &Runner{
		LLMClient:        client,
		SkillRuntime:     skills.NewRuntime(nil, nil),
		AppContext:       &llmclient.AppContext{},
		ModelIdleTimeout: 35 * time.Millisecond,
		ModelProgressSchedule: ModelProgressSchedule{
			Initial:     5 * time.Millisecond,
			Extended:    15 * time.Millisecond,
			LongRunning: 25 * time.Millisecond,
		},
		OnEvent: onEvent,
	}
	prepared := NewPreparedChat("timeout-conversation", "timeout-message", "", "auto", &adapter.ChatRequest{
		Model:    "non-stream-model",
		Messages: []adapter.Message{{Role: "user", Content: "work"}},
	})

	_, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:                   prepared,
		Resolved:                   &skills.ResolvedSkills{},
		ProtocolToolsOnly:          true,
		NativeAgentLoop:            true,
		NativeModelProgressEnabled: true,
	})
	if !errors.Is(err, ErrModelIdleTimeout) {
		t.Fatalf("Run() error = %v, want model idle timeout", err)
	}
	before := len(events())
	time.Sleep(35 * time.Millisecond)
	if after := len(events()); after != before {
		t.Fatalf("model progress events after timeout = %d, want stable %d", after, before)
	}
}

func TestNativeModelProgressStopsOnCancellation(t *testing.T) {
	client := &runnerTestLLMClient{
		appChatDelays:    []time.Duration{time.Second},
		appChatResponses: []*adapter.ChatResponse{{}},
	}
	events, onEvent := modelProgressEventCollector()
	runner := &Runner{
		LLMClient:        client,
		SkillRuntime:     skills.NewRuntime(nil, nil),
		AppContext:       &llmclient.AppContext{},
		ModelIdleTimeout: time.Second,
		ModelProgressSchedule: ModelProgressSchedule{
			Initial:     5 * time.Millisecond,
			Extended:    50 * time.Millisecond,
			LongRunning: 100 * time.Millisecond,
		},
		OnEvent: onEvent,
	}
	prepared := NewPreparedChat("cancel-conversation", "cancel-message", "", "auto", &adapter.ChatRequest{
		Model:    "non-stream-model",
		Messages: []adapter.Message{{Role: "user", Content: "work"}},
	})
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)

	_, _, err := runner.Run(ctx, RunRequest{
		Prepared:                   prepared,
		Resolved:                   &skills.ResolvedSkills{},
		ProtocolToolsOnly:          true,
		NativeAgentLoop:            true,
		NativeModelProgressEnabled: true,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context canceled", err)
	}
	before := len(events())
	time.Sleep(100 * time.Millisecond)
	if after := len(events()); after != before {
		t.Fatalf("model progress events after cancellation = %d, want stable %d", after, before)
	}
}

func modelProgressEventCollector() (func() []Event, func(Event) error) {
	var mu sync.Mutex
	events := make([]Event, 0)
	return func() []Event {
			mu.Lock()
			defer mu.Unlock()
			return append([]Event(nil), events...)
		}, func(event Event) error {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, event)
			return nil
		}
}

func assertModelProgressStages(t *testing.T, events []Event, want []string) {
	t.Helper()
	stages := make([]string, 0)
	for _, event := range events {
		if event.Type != EventAgentProgress || event.Payload["phase"] != modelProgressPhase {
			continue
		}
		stage, _ := event.Payload["stage"].(string)
		stages = append(stages, stage)
		if event.Payload["progress_id"] == "" || event.Payload["status"] != "running" {
			t.Fatalf("model progress payload = %#v, want progress_id and running status", event.Payload)
		}
	}
	if len(stages) != len(want) {
		t.Fatalf("model progress stages = %#v, want %#v", stages, want)
	}
	for index := range want {
		if stages[index] != want[index] {
			t.Fatalf("model progress stages = %#v, want %#v", stages, want)
		}
	}
}

func assertModelProgressIncludesStages(t *testing.T, events []Event, want []string) {
	t.Helper()
	seen := make(map[string]bool)
	for _, event := range events {
		if event.Type == EventAgentProgress && event.Payload["phase"] == modelProgressPhase {
			stage, _ := event.Payload["stage"].(string)
			seen[stage] = true
		}
	}
	for _, stage := range want {
		if !seen[stage] {
			t.Fatalf("model progress stages = %#v, missing %q", seen, stage)
		}
	}
}

func assertModelProgressActivity(t *testing.T, events []Event, activity string, source string) {
	t.Helper()
	for _, event := range events {
		if event.Type != EventAgentProgress || event.Payload["phase"] != modelProgressPhase {
			continue
		}
		if event.Payload["activity"] == activity && event.Payload["source"] == source {
			return
		}
	}
	t.Fatalf("model progress events = %#v, missing activity %q from %q", events, activity, source)
}
