package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	aichatmodel "github.com/zgiai/zgi/api/internal/modules/aichat/model"
	"github.com/zgiai/zgi/api/internal/modules/aichat/repository"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin/calculator"
)

func TestFinalizePreparedErrorSetsFailedMessageAsCurrentLeaf(t *testing.T) {
	conversationID := uuid.New()
	messageID := uuid.New()
	conversationRepo := &recordingConversationRepository{}
	messageRepo := &recordingMessageRepository{}
	svc := &service{
		repos: &repository.Repositories{
			Conversation: conversationRepo,
			Message:      messageRepo,
		},
		events: newStreamEventStore(nil),
	}

	events := make([]StreamEvent, 0)

	svc.finalizePreparedError(context.Background(), &PreparedChat{
		Conversation: &aichatmodel.Conversation{ID: conversationID},
		Message:      &aichatmodel.Message{ID: messageID},
	}, errors.New("tool failed"), func(event StreamEvent) error {
		events = append(events, event)
		return nil
	})

	if messageRepo.updateErrorID != messageID || messageRepo.updateErrorMessage != "tool failed" {
		t.Fatalf("UpdateError(%s, %q), want (%s, %q)", messageRepo.updateErrorID, messageRepo.updateErrorMessage, messageID, "tool failed")
	}
	if conversationRepo.updateAfterConversationID != conversationID || conversationRepo.updateAfterLeafID != messageID {
		t.Fatalf("UpdateAfterMessage(%s, %s), want (%s, %s)", conversationRepo.updateAfterConversationID, conversationRepo.updateAfterLeafID, conversationID, messageID)
	}
	if conversationRepo.finishActiveCalls != 0 {
		t.Fatalf("FinishActiveMessage calls = %d, want 0", conversationRepo.finishActiveCalls)
	}
	if len(events) != 1 || events[0].EventType != streamEventError {
		t.Fatalf("events = %#v, want one stream error event", events)
	}
	if got, _ := events[0].Payload["message"].(string); got != "tool failed" {
		t.Fatalf("stream error message = %q, want %q", got, "tool failed")
	}
}

func TestFinalizePreparedErrorCompletesRootReplacement(t *testing.T) {
	conversationID := uuid.New()
	messageID := uuid.New()
	conversationRepo := &recordingConversationRepository{}
	messageRepo := &recordingMessageRepository{}
	svc := &service{
		repos: &repository.Repositories{
			Conversation: conversationRepo,
			Message:      messageRepo,
		},
		events: newStreamEventStore(nil),
	}

	svc.finalizePreparedError(context.Background(), &PreparedChat{
		Conversation: &aichatmodel.Conversation{ID: conversationID},
		Message:      &aichatmodel.Message{ID: messageID},
		ReplaceRoot:  true,
	}, errors.New("model failed"))

	if conversationRepo.completeRootConversationID != conversationID || conversationRepo.completeRootMessageID != messageID {
		t.Fatalf("CompleteRootReplacement(%s, %s), want (%s, %s)", conversationRepo.completeRootConversationID, conversationRepo.completeRootMessageID, conversationID, messageID)
	}
	if conversationRepo.updateAfterConversationID != uuid.Nil {
		t.Fatalf("UpdateAfterMessage was called for root replacement")
	}
}

func TestFinalizedStreamErrorWrapsCause(t *testing.T) {
	cause := errors.New("provider failed")
	err := newFinalizedStreamError(cause)

	if !IsFinalizedStreamError(err) {
		t.Fatalf("IsFinalizedStreamError(%v) = false, want true", err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is(%v, cause) = false, want true", err)
	}
}

func TestHandleProgressiveSkillCallReturnsRecoverableArgumentError(t *testing.T) {
	svc := &service{}
	call := adapter.ToolCall{
		ID: "call_1",
		Function: adapter.FunctionCall{
			Name:      skills.MetaToolLoadSkill,
			Arguments: "{",
		},
	}

	result := svc.handleProgressiveSkillCall(context.Background(), &PreparedChat{}, nil, call, skills.ExecutionContext{}, 0, nil, map[string]struct{}{}, nil)

	if !result.recoverable || result.fatalErr != nil {
		t.Fatalf("result recoverable=%v fatalErr=%v, want recoverable without fatal", result.recoverable, result.fatalErr)
	}
	if result.trace.Status != "error" || !strings.Contains(result.trace.Error, "invalid tool arguments") {
		t.Fatalf("trace = %#v, want invalid arguments error", result.trace)
	}
	content, _ := result.toolMessage.Content.(string)
	if !strings.Contains(content, `"recoverable":true`) {
		t.Fatalf("tool message content = %q, want recoverable payload", content)
	}
}

func TestRunPreparedSkillStreamEmitsPlanningTextAsAnswer(t *testing.T) {
	catalogDir := t.TempDir()
	writeTestSkill(t, catalogDir, "brief-writer", `---
name: brief-writer
description: Help draft short writing briefs.
when_to_use: Use when writing a concise brief.
---

# Brief Writer

Write concise briefs.
`)
	conversationID := uuid.New()
	messageID := uuid.New()
	svc := &service{
		repos: &repository.Repositories{
			Message:     &recordingMessageRepository{},
			CustomSkill: &fakeCustomSkillRepository{items: map[string]*aichatmodel.CustomSkill{}},
		},
		llmClient: &fakeAgenticLLMClient{
			appChatResponses: []*adapter.ChatResponse{
				{
					Choices: []adapter.Choice{{
						Message: adapter.Message{Role: "assistant", Content: "我会直接给出结果。"},
					}},
					Usage: &adapter.Usage{PromptTokens: 3, CompletionTokens: 4, TotalTokens: 7},
				},
			},
		},
		events:       newStreamEventStore(nil),
		skillRuntime: skills.NewRuntimeWithCatalog(tools.NewToolEngine(tools.NewToolManager(nil)), tools.NewToolManager(nil), catalogDir),
	}
	prepared := &PreparedChat{
		Conversation: &aichatmodel.Conversation{
			ID:             conversationID,
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
		},
		Message: &aichatmodel.Message{
			ID:       messageID,
			Metadata: map[string]interface{}{},
		},
		LLMRequest: &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "写一个 brief"}},
		},
		Scope: Scope{
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
		},
		parts: &chatRequestParts{
			SkillMode: skillModeAuto,
			SkillIDs:  []string{"brief-writer"},
		},
	}
	chunks := make([]string, 0)

	answer, usage, err := svc.runPreparedSkillStream(context.Background(), context.Background(), prepared, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	}, nil)
	if err != nil {
		t.Fatalf("runPreparedSkillStream() error = %v", err)
	}
	if answer != "我会直接给出结果。" {
		t.Fatalf("answer = %q, want planning text answer", answer)
	}
	if len(chunks) != 1 || chunks[0] != answer {
		t.Fatalf("chunks = %#v, want single answer chunk", chunks)
	}
	if usage == nil || usage.TotalTokens != 7 {
		t.Fatalf("usage = %#v, want aggregated usage", usage)
	}
}

func TestRunPreparedSkillStreamContinuesAfterRecoverableToolArgumentError(t *testing.T) {
	catalogDir := t.TempDir()
	writeTestSkill(t, catalogDir, "brief-writer", `---
name: brief-writer
description: Help draft short writing briefs.
when_to_use: Use when writing a concise brief.
---

# Brief Writer

Write concise briefs.
`)
	fakeLLM := &fakeAgenticLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{{
							ID:   "call_1",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolLoadSkill,
								Arguments: "{",
							},
						}},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "参数已修正，结果完成。"},
				}},
			},
		},
	}
	svc := &service{
		repos: &repository.Repositories{
			Message:     &recordingMessageRepository{},
			CustomSkill: &fakeCustomSkillRepository{items: map[string]*aichatmodel.CustomSkill{}},
		},
		llmClient:    fakeLLM,
		events:       newStreamEventStore(nil),
		skillRuntime: skills.NewRuntimeWithCatalog(tools.NewToolEngine(tools.NewToolManager(nil)), tools.NewToolManager(nil), catalogDir),
	}
	prepared := &PreparedChat{
		Conversation: &aichatmodel.Conversation{
			ID:             uuid.New(),
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
		},
		Message: &aichatmodel.Message{
			ID:       uuid.New(),
			Metadata: map[string]interface{}{},
		},
		LLMRequest: &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "写一个 brief"}},
		},
		Scope: Scope{
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
		},
		parts: &chatRequestParts{
			SkillMode: skillModeAuto,
			SkillIDs:  []string{"brief-writer"},
		},
	}

	answer, _, err := svc.runPreparedSkillStream(context.Background(), context.Background(), prepared, nil, nil)
	if err != nil {
		t.Fatalf("runPreparedSkillStream() error = %v", err)
	}
	if answer != "参数已修正，结果完成。" {
		t.Fatalf("answer = %q, want final answer after recovery", answer)
	}
	if fakeLLM.appChatCalls != 2 {
		t.Fatalf("AppChat calls = %d, want 2", fakeLLM.appChatCalls)
	}
}

func TestRunPreparedSkillStreamCountsOnlySuccessfulSkillToolCalls(t *testing.T) {
	catalogDir := t.TempDir()
	writeTestSkill(t, catalogDir, "limited-calculator", `---
name: limited-calculator
description: Calculate with a low per-turn success limit.
when_to_use: Use when testing tool call limits.
provider_type: builtin
provider_id: calculator
runtime_type: tool
tools:
  - evaluate_expression
max_calls_per_turn: 5
---

# Limited Calculator

Use the calculator tool.
`)
	toolCalls := []adapter.ToolCall{
		testSkillToolCall("call_bad", "limited-calculator", "evaluate_expression", map[string]interface{}{
			"expression": "1/",
		}),
	}
	for i := 1; i <= 5; i++ {
		toolCalls = append(toolCalls, testSkillToolCall(
			fmt.Sprintf("call_ok_%d", i),
			"limited-calculator",
			"evaluate_expression",
			map[string]interface{}{"expression": fmt.Sprintf("%d+1", i)},
		))
	}
	fakeLLM := &fakeAgenticLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{{
							ID:   "call_load",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolLoadSkill,
								Arguments: `{"skill_id":"limited-calculator"}`,
							},
						}},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role:      "assistant",
						ToolCalls: toolCalls,
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "done"},
				}},
			},
		},
	}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(calculator.NewProvider()); err != nil {
		t.Fatalf("register calculator provider: %v", err)
	}
	svc := &service{
		repos: &repository.Repositories{
			Message:     &recordingMessageRepository{},
			CustomSkill: &fakeCustomSkillRepository{items: map[string]*aichatmodel.CustomSkill{}},
		},
		llmClient:    fakeLLM,
		events:       newStreamEventStore(nil),
		skillRuntime: skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir),
	}
	prepared := &PreparedChat{
		Conversation: &aichatmodel.Conversation{
			ID:             uuid.New(),
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
		},
		Message: &aichatmodel.Message{
			ID:       uuid.New(),
			Metadata: map[string]interface{}{},
		},
		LLMRequest: &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "calculate several expressions"}},
		},
		Scope: Scope{
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
		},
		parts: &chatRequestParts{
			SkillMode: skillModeAuto,
			SkillIDs:  []string{"limited-calculator"},
		},
	}

	answer, _, err := svc.runPreparedSkillStream(context.Background(), context.Background(), prepared, nil, nil)
	if err != nil {
		t.Fatalf("runPreparedSkillStream() error = %v, want failed tool attempt not to consume success limit", err)
	}
	if answer != "done" {
		t.Fatalf("answer = %q, want done", answer)
	}
	if fakeLLM.appChatCalls != 3 {
		t.Fatalf("AppChat calls = %d, want 3", fakeLLM.appChatCalls)
	}
}

func TestRunPreparedSkillStreamStopsAfterConsecutiveFailedSkillToolCalls(t *testing.T) {
	catalogDir := t.TempDir()
	writeTestSkill(t, catalogDir, "limited-calculator", `---
name: limited-calculator
description: Calculate with a low per-turn success limit.
when_to_use: Use when testing tool call limits.
provider_type: builtin
provider_id: calculator
runtime_type: tool
tools:
  - evaluate_expression
max_calls_per_turn: 20
---

# Limited Calculator

Use the calculator tool.
`)
	toolCalls := make([]adapter.ToolCall, 0, defaultMaxConsecutiveRecoverableFailures+1)
	for i := 0; i <= defaultMaxConsecutiveRecoverableFailures; i++ {
		toolCalls = append(toolCalls, testSkillToolCall(
			fmt.Sprintf("call_bad_%d", i),
			"limited-calculator",
			"evaluate_expression",
			map[string]interface{}{"expression": "1/"},
		))
	}
	fakeLLM := &fakeAgenticLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{{
							ID:   "call_load",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolLoadSkill,
								Arguments: `{"skill_id":"limited-calculator"}`,
							},
						}},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role:      "assistant",
						ToolCalls: toolCalls,
					},
				}},
			},
		},
	}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(calculator.NewProvider()); err != nil {
		t.Fatalf("register calculator provider: %v", err)
	}
	svc := &service{
		repos: &repository.Repositories{
			Message:     &recordingMessageRepository{},
			CustomSkill: &fakeCustomSkillRepository{items: map[string]*aichatmodel.CustomSkill{}},
		},
		llmClient:    fakeLLM,
		events:       newStreamEventStore(nil),
		skillRuntime: skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir),
	}
	prepared := &PreparedChat{
		Conversation: &aichatmodel.Conversation{
			ID:             uuid.New(),
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
		},
		Message: &aichatmodel.Message{
			ID:       uuid.New(),
			Metadata: map[string]interface{}{},
		},
		LLMRequest: &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "calculate several expressions"}},
		},
		Scope: Scope{
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
		},
		parts: &chatRequestParts{
			SkillMode: skillModeAuto,
			SkillIDs:  []string{"limited-calculator"},
		},
	}

	_, _, err := svc.runPreparedSkillStream(context.Background(), context.Background(), prepared, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "too many failed skill calls") {
		t.Fatalf("runPreparedSkillStream() error = %v, want failed skill call limit", err)
	}
	if fakeLLM.appChatCalls != 2 {
		t.Fatalf("AppChat calls = %d, want 2", fakeLLM.appChatCalls)
	}
}

func TestRunPreparedSkillStreamRequiredModeDoesNotEmitAnswerBeforeSkillUseError(t *testing.T) {
	catalogDir := t.TempDir()
	writeTestSkill(t, catalogDir, "brief-writer", `---
name: brief-writer
description: Help draft short writing briefs.
when_to_use: Use when writing a concise brief.
---

# Brief Writer

Write concise briefs.
`)
	svc := &service{
		repos: &repository.Repositories{
			Message:     &recordingMessageRepository{},
			CustomSkill: &fakeCustomSkillRepository{items: map[string]*aichatmodel.CustomSkill{}},
		},
		llmClient: &fakeAgenticLLMClient{
			appChatResponses: []*adapter.ChatResponse{
				{
					Choices: []adapter.Choice{{
						Message: adapter.Message{Role: "assistant", Content: "I can answer directly."},
					}},
				},
			},
		},
		events:       newStreamEventStore(nil),
		skillRuntime: skills.NewRuntimeWithCatalog(tools.NewToolEngine(tools.NewToolManager(nil)), tools.NewToolManager(nil), catalogDir),
	}
	prepared := &PreparedChat{
		Conversation: &aichatmodel.Conversation{
			ID:             uuid.New(),
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
		},
		Message: &aichatmodel.Message{
			ID:       uuid.New(),
			Metadata: map[string]interface{}{},
		},
		LLMRequest: &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "write a brief"}},
		},
		Scope: Scope{
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
		},
		parts: &chatRequestParts{
			SkillMode: skillModeRequired,
			SkillIDs:  []string{"brief-writer"},
		},
	}
	chunks := make([]string, 0)

	answer, _, err := svc.runPreparedSkillStream(context.Background(), context.Background(), prepared, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "required skill was not used") {
		t.Fatalf("runPreparedSkillStream() error = %v, want required skill error", err)
	}
	if answer != "" {
		t.Fatalf("answer = %q, want empty answer", answer)
	}
	if len(chunks) != 0 {
		t.Fatalf("chunks = %#v, want no streamed answer chunks", chunks)
	}
}

func TestRunPreparedSkillStreamEmitsAgentProgressForContentWithToolCalls(t *testing.T) {
	catalogDir := t.TempDir()
	writeTestSkill(t, catalogDir, "brief-writer", `---
name: brief-writer
description: Help draft short writing briefs.
when_to_use: Use when writing a concise brief.
---

# Brief Writer

Write concise briefs.
`)
	fakeLLM := &fakeAgenticLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role:    "assistant",
						Content: "我会先加载 brief writer。",
						ToolCalls: []adapter.ToolCall{{
							ID:   "call_1",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolLoadSkill,
								Arguments: "{",
							},
						}},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "最终结果。"},
				}},
			},
		},
	}
	svc := &service{
		repos: &repository.Repositories{
			Message:     &recordingMessageRepository{},
			CustomSkill: &fakeCustomSkillRepository{items: map[string]*aichatmodel.CustomSkill{}},
		},
		llmClient:    fakeLLM,
		events:       newStreamEventStore(nil),
		skillRuntime: skills.NewRuntimeWithCatalog(tools.NewToolEngine(tools.NewToolManager(nil)), tools.NewToolManager(nil), catalogDir),
	}
	prepared := &PreparedChat{
		Conversation: &aichatmodel.Conversation{
			ID:             uuid.New(),
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
		},
		Message: &aichatmodel.Message{
			ID:       uuid.New(),
			Metadata: map[string]interface{}{},
		},
		LLMRequest: &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "写一个 brief"}},
		},
		Scope: Scope{
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
		},
		parts: &chatRequestParts{
			SkillMode: skillModeAuto,
			SkillIDs:  []string{"brief-writer"},
		},
	}
	events := make([]StreamEvent, 0)

	answer, _, err := svc.runPreparedSkillStream(context.Background(), context.Background(), prepared, nil, func(event StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("runPreparedSkillStream() error = %v", err)
	}
	if answer != "最终结果。" {
		t.Fatalf("answer = %q, want final answer without progress text", answer)
	}
	var progressContent string
	for _, event := range events {
		if event.EventType == streamEventAgentProgress {
			progressContent, _ = event.Payload["content"].(string)
			break
		}
	}
	if progressContent != "我会先加载 brief writer。" {
		t.Fatalf("agent progress content = %q, want progress text", progressContent)
	}
}

func TestRunPreparedSkillStreamRecordsIntermediateAnswer(t *testing.T) {
	catalogDir := t.TempDir()
	writeTestSkill(t, catalogDir, "brief-writer", `---
name: brief-writer
description: Help draft short writing briefs.
when_to_use: Use when writing a concise brief.
---

# Brief Writer

Write concise briefs.
`)
	fakeLLM := &fakeAgenticLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{{
							ID:   "call_1",
							Type: "function",
							Function: adapter.FunctionCall{
								Name: skills.MetaToolIntermediateAnswer,
								Arguments: `{
									"title":"Outline",
									"content":"## Outline\n\nA useful draft."
								}`,
							},
						}},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "Final answer."},
				}},
			},
		},
	}
	svc := &service{
		repos: &repository.Repositories{
			Message:     &recordingMessageRepository{},
			CustomSkill: &fakeCustomSkillRepository{items: map[string]*aichatmodel.CustomSkill{}},
		},
		llmClient:    fakeLLM,
		events:       newStreamEventStore(nil),
		skillRuntime: skills.NewRuntimeWithCatalog(tools.NewToolEngine(tools.NewToolManager(nil)), tools.NewToolManager(nil), catalogDir),
	}
	prepared := &PreparedChat{
		Conversation: &aichatmodel.Conversation{
			ID:             uuid.New(),
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
		},
		Message: &aichatmodel.Message{
			ID:       uuid.New(),
			Metadata: map[string]interface{}{},
		},
		LLMRequest: &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "write an outline then continue"}},
		},
		Scope: Scope{
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
		},
		parts: &chatRequestParts{
			SkillMode: skillModeAuto,
			SkillIDs:  []string{"brief-writer"},
		},
	}
	events := make([]StreamEvent, 0)

	answer, _, err := svc.runPreparedSkillStream(context.Background(), context.Background(), prepared, nil, func(event StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("runPreparedSkillStream() error = %v", err)
	}
	if answer != "Final answer." {
		t.Fatalf("answer = %q, want final answer only", answer)
	}
	var intermediateContent string
	for _, event := range events {
		if event.EventType == streamEventIntermediateAnswer {
			intermediateContent, _ = event.Payload["content"].(string)
			break
		}
	}
	if intermediateContent != "## Outline\n\nA useful draft." {
		t.Fatalf("intermediate content = %q, want submitted draft", intermediateContent)
	}
	invocations, _ := prepared.Message.Metadata["skill_invocations"].([]interface{})
	if len(invocations) == 0 {
		t.Fatalf("skill_invocations is empty, want intermediate answer trace")
	}
	last, _ := invocations[len(invocations)-1].(map[string]interface{})
	if last["kind"] != "intermediate_answer" || last["message"] != "## Outline\n\nA useful draft." {
		t.Fatalf("last invocation = %#v, want intermediate answer trace", last)
	}
}

func TestRunPreparedSkillStreamStagesSkillToolSchema(t *testing.T) {
	catalogDir := t.TempDir()
	writeTestSkill(t, catalogDir, "clock", `---
name: clock
description: Look up current time.
when_to_use: Use when time is needed.
provider_type: builtin
provider_id: time
runtime_type: tool
tools:
  - current_time
---

# Clock

Use the current_time tool after loading this skill.
`)
	fakeLLM := &fakeAgenticLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{{
							ID:   "call_1",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolLoadSkill,
								Arguments: `{"skill_id":"clock"}`,
							},
						}},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "Clock skill is ready."},
				}},
			},
		},
	}
	svc := &service{
		repos: &repository.Repositories{
			Message:     &recordingMessageRepository{},
			CustomSkill: &fakeCustomSkillRepository{items: map[string]*aichatmodel.CustomSkill{}},
		},
		llmClient:    fakeLLM,
		events:       newStreamEventStore(nil),
		skillRuntime: skills.NewRuntimeWithCatalog(tools.NewToolEngine(tools.NewToolManager(nil)), tools.NewToolManager(nil), catalogDir),
	}
	prepared := &PreparedChat{
		Conversation: &aichatmodel.Conversation{
			ID:             uuid.New(),
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
		},
		Message: &aichatmodel.Message{
			ID:       uuid.New(),
			Metadata: map[string]interface{}{},
		},
		LLMRequest: &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "what time is it"}},
		},
		Scope: Scope{
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
		},
		parts: &chatRequestParts{
			SkillMode: skillModeAuto,
			SkillIDs:  []string{"clock"},
		},
	}

	if _, _, err := svc.runPreparedSkillStream(context.Background(), context.Background(), prepared, nil, nil); err != nil {
		t.Fatalf("runPreparedSkillStream() error = %v", err)
	}
	if len(fakeLLM.appChatRequests) != 2 {
		t.Fatalf("recorded AppChat requests = %d, want 2", len(fakeLLM.appChatRequests))
	}
	firstTools := toolNamesFromRequest(fakeLLM.appChatRequests[0])
	if containsString(firstTools, skills.MetaToolCallSkillTool) {
		t.Fatalf("first planning tools = %#v, want no call_skill_tool before load_skill", firstTools)
	}
	secondTools := toolNamesFromRequest(fakeLLM.appChatRequests[1])
	if !containsString(secondTools, skills.MetaToolCallSkillTool) {
		t.Fatalf("second planning tools = %#v, want call_skill_tool after load_skill", secondTools)
	}
	callTool := findToolByName(fakeLLM.appChatRequests[1].Tools, skills.MetaToolCallSkillTool)
	properties := toolParameterProperties(callTool)
	skillEnum, _ := properties["skill_id"].(map[string]interface{})["enum"].([]string)
	toolEnum, _ := properties["tool_name"].(map[string]interface{})["enum"].([]string)
	if !containsString(skillEnum, "clock") || !containsString(toolEnum, "current_time") {
		t.Fatalf("call_skill_tool enum skill=%#v tool=%#v, want clock/current_time", skillEnum, toolEnum)
	}
}

func TestRunPreparedSkillStreamPreservesReasoningContentForToolRounds(t *testing.T) {
	catalogDir := t.TempDir()
	writeTestSkill(t, catalogDir, "clock", `---
name: clock
description: Look up current time.
when_to_use: Use when time is needed.
provider_type: builtin
provider_id: time
runtime_type: tool
tools:
  - current_time
---

# Clock

Use the current_time tool after loading this skill.
`)
	fakeLLM := &fakeAgenticLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role:             "assistant",
						ReasoningContent: "I should load the clock skill first.",
						ToolCalls: []adapter.ToolCall{{
							ID:   "call_1",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolLoadSkill,
								Arguments: `{"skill_id":"clock"}`,
							},
						}},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "Clock skill is ready."},
				}},
			},
		},
	}
	svc := &service{
		repos: &repository.Repositories{
			Message:     &recordingMessageRepository{},
			CustomSkill: &fakeCustomSkillRepository{items: map[string]*aichatmodel.CustomSkill{}},
		},
		llmClient:    fakeLLM,
		events:       newStreamEventStore(nil),
		skillRuntime: skills.NewRuntimeWithCatalog(tools.NewToolEngine(tools.NewToolManager(nil)), tools.NewToolManager(nil), catalogDir),
	}
	prepared := &PreparedChat{
		Conversation: &aichatmodel.Conversation{
			ID:             uuid.New(),
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
		},
		Message: &aichatmodel.Message{
			ID:       uuid.New(),
			Metadata: map[string]interface{}{},
		},
		LLMRequest: &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "what time is it"}},
		},
		Scope: Scope{
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
		},
		parts: &chatRequestParts{
			SkillMode: skillModeAuto,
			SkillIDs:  []string{"clock"},
		},
	}

	if _, _, err := svc.runPreparedSkillStream(context.Background(), context.Background(), prepared, nil, nil); err != nil {
		t.Fatalf("runPreparedSkillStream() error = %v", err)
	}
	if len(fakeLLM.appChatRequests) != 2 {
		t.Fatalf("recorded AppChat requests = %d, want 2", len(fakeLLM.appChatRequests))
	}
	var found bool
	for _, message := range fakeLLM.appChatRequests[1].Messages {
		if message.Role == "assistant" && message.ReasoningContent == "I should load the clock skill first." {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("second planning request did not preserve assistant reasoning_content: %#v", fakeLLM.appChatRequests[1].Messages)
	}
}

func TestAgenticSkillLoopSystemMessageRequiresIntermediateAnswers(t *testing.T) {
	message := agenticSkillLoopSystemMessage()
	content, _ := message.Content.(string)

	requiredPhrases := []string{
		"MUST call submit_intermediate_answer",
		"multiple ordered phases",
		"that reply MUST include the deliverable in full",
		"not a compressed summary",
		"Do not label the user-facing reply",
	}
	for _, phrase := range requiredPhrases {
		if !strings.Contains(content, phrase) {
			t.Fatalf("agentic skill loop system message missing %q: %s", phrase, content)
		}
	}
}

type recordingConversationRepository struct {
	updateAfterConversationID  uuid.UUID
	updateAfterLeafID          uuid.UUID
	completeRootConversationID uuid.UUID
	completeRootMessageID      uuid.UUID
	finishActiveCalls          int
}

func (r *recordingConversationRepository) Create(ctx context.Context, conversation *aichatmodel.Conversation) error {
	return nil
}
func (r *recordingConversationRepository) GetScoped(ctx context.Context, id, organizationID, accountID uuid.UUID) (*aichatmodel.Conversation, error) {
	return nil, nil
}
func (r *recordingConversationRepository) GetBySourceConversation(ctx context.Context, sourceConversationID uuid.UUID) (*aichatmodel.Conversation, error) {
	return nil, nil
}
func (r *recordingConversationRepository) ListScoped(ctx context.Context, organizationID, accountID uuid.UUID, limit, offset int) ([]*aichatmodel.Conversation, int64, error) {
	return nil, 0, nil
}
func (r *recordingConversationRepository) UpdateScoped(ctx context.Context, id, organizationID, accountID uuid.UUID, updates map[string]interface{}) error {
	return nil
}
func (r *recordingConversationRepository) UpdateMetadata(ctx context.Context, id uuid.UUID, metadata map[string]interface{}) error {
	return nil
}
func (r *recordingConversationRepository) DeleteScoped(ctx context.Context, id, organizationID, accountID uuid.UUID) error {
	return nil
}
func (r *recordingConversationRepository) StartStreaming(ctx context.Context, id, organizationID, accountID, messageID uuid.UUID) error {
	return nil
}
func (r *recordingConversationRepository) ClearActiveMessage(ctx context.Context, id, messageID uuid.UUID) error {
	return nil
}
func (r *recordingConversationRepository) FinishActiveMessage(ctx context.Context, id, messageID uuid.UUID) error {
	r.finishActiveCalls++
	return nil
}
func (r *recordingConversationRepository) ClearActiveMessages(ctx context.Context, messageIDs []uuid.UUID) error {
	return nil
}
func (r *recordingConversationRepository) CompleteRootReplacement(ctx context.Context, id, messageID uuid.UUID) error {
	r.completeRootConversationID = id
	r.completeRootMessageID = messageID
	return nil
}
func (r *recordingConversationRepository) UpdateAfterMessage(ctx context.Context, id uuid.UUID, leafMessageID uuid.UUID) error {
	r.updateAfterConversationID = id
	r.updateAfterLeafID = leafMessageID
	return nil
}
func (r *recordingConversationRepository) RefreshAfterMessageDelete(ctx context.Context, id uuid.UUID) error {
	return nil
}

type recordingMessageRepository struct {
	updateErrorID      uuid.UUID
	updateErrorMessage string
}

func (r *recordingMessageRepository) Create(ctx context.Context, message *aichatmodel.Message) error {
	return nil
}
func (r *recordingMessageRepository) GetScoped(ctx context.Context, id, organizationID, accountID uuid.UUID) (*aichatmodel.Message, error) {
	return nil, nil
}
func (r *recordingMessageRepository) GetBySourceMessage(ctx context.Context, sourceMessageID uuid.UUID) (*aichatmodel.Message, error) {
	return nil, nil
}
func (r *recordingMessageRepository) ListByConversationScoped(ctx context.Context, conversationID, organizationID, accountID uuid.UUID, limit, offset int) ([]*aichatmodel.Message, int64, error) {
	return nil, 0, nil
}
func (r *recordingMessageRepository) ListBranch(ctx context.Context, leafID uuid.UUID, maxDepth int) ([]*aichatmodel.Message, error) {
	return nil, nil
}
func (r *recordingMessageRepository) CountByConversation(ctx context.Context, conversationID uuid.UUID) (int64, error) {
	return 0, nil
}
func (r *recordingMessageRepository) ReplaceRootForStreaming(ctx context.Context, message *aichatmodel.Message) error {
	return nil
}
func (r *recordingMessageRepository) UpdateCompleted(ctx context.Context, id uuid.UUID, answer string, metadata map[string]interface{}) error {
	return nil
}
func (r *recordingMessageRepository) UpdateMetadata(ctx context.Context, id uuid.UUID, metadata map[string]interface{}) error {
	return nil
}
func (r *recordingMessageRepository) UpdateError(ctx context.Context, id uuid.UUID, message string) error {
	r.updateErrorID = id
	r.updateErrorMessage = message
	return nil
}
func (r *recordingMessageRepository) MarkStopped(ctx context.Context, id uuid.UUID) error {
	return nil
}
func (r *recordingMessageRepository) UpdateStoppedAnswer(ctx context.Context, id uuid.UUID, answer string, metadata map[string]interface{}) error {
	return nil
}
func (r *recordingMessageRepository) DeleteSubtreeScoped(ctx context.Context, id, organizationID, accountID uuid.UUID) (*repository.MessageDeleteResult, error) {
	return nil, nil
}
func (r *recordingMessageRepository) ListStaleActiveIDs(ctx context.Context, cutoff time.Time) ([]uuid.UUID, error) {
	return nil, nil
}
func (r *recordingMessageRepository) MarkStaleActiveAsError(ctx context.Context, cutoff time.Time, message string) (int64, error) {
	return 0, nil
}

func writeTestSkill(t *testing.T, catalogDir string, skillID string, content string) {
	t.Helper()

	root := filepath.Join(catalogDir, skillID)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir skill root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

func testSkillToolCall(callID string, skillID string, toolName string, arguments map[string]interface{}) adapter.ToolCall {
	payload, _ := json.Marshal(map[string]interface{}{
		"skill_id":  skillID,
		"tool_name": toolName,
		"arguments": arguments,
	})
	return adapter.ToolCall{
		ID:   callID,
		Type: "function",
		Function: adapter.FunctionCall{
			Name:      skills.MetaToolCallSkillTool,
			Arguments: string(payload),
		},
	}
}

func toolNamesFromRequest(req *adapter.ChatRequest) []string {
	if req == nil {
		return nil
	}
	out := make([]string, 0, len(req.Tools))
	for _, tool := range req.Tools {
		out = append(out, tool.Function.Name)
	}
	return out
}

func findToolByName(tools []adapter.Tool, name string) adapter.Tool {
	for _, tool := range tools {
		if tool.Function.Name == name {
			return tool
		}
	}
	return adapter.Tool{}
}

func toolParameterProperties(tool adapter.Tool) map[string]interface{} {
	parameters, _ := tool.Function.Parameters.(map[string]interface{})
	properties, _ := parameters["properties"].(map[string]interface{})
	return properties
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

type fakeAgenticLLMClient struct {
	appChatResponses []*adapter.ChatResponse
	appChatRequests  []*adapter.ChatRequest
	appChatCalls     int
}

func (f *fakeAgenticLLMClient) Chat(ctx context.Context, organizationID string, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAgenticLLMClient) ChatStream(ctx context.Context, organizationID string, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAgenticLLMClient) CreateResponse(ctx context.Context, organizationID string, req *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAgenticLLMClient) Embed(ctx context.Context, organizationID string, req *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAgenticLLMClient) CreateImage(ctx context.Context, organizationID string, req *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAgenticLLMClient) Rerank(ctx context.Context, organizationID string, req *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAgenticLLMClient) AppChat(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	if f.appChatCalls >= len(f.appChatResponses) {
		return nil, errors.New("unexpected AppChat call")
	}
	cloned := cloneChatRequest(req)
	f.appChatRequests = append(f.appChatRequests, cloned)
	resp := f.appChatResponses[f.appChatCalls]
	f.appChatCalls++
	return resp, nil
}

func (f *fakeAgenticLLMClient) AppChatStream(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAgenticLLMClient) AppCreateResponse(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAgenticLLMClient) AppEmbed(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAgenticLLMClient) AppCreateImage(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAgenticLLMClient) AppRerank(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, errors.New("not implemented")
}
