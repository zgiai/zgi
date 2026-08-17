package contextmgr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestBudgetUsesAgentWorkingWindowAndClampsToPhysicalWindow(t *testing.T) {
	config, err := normalizeConfig(Config{
		ConfiguredAgentWindowK: 256,
		ModelContextWindow:     1_000_000,
		MaxInputTokens:         900_000,
		MaxOutputTokens:        32_000,
		SummaryOutputTokens:    20_000,
		EmergencyBufferTokens:  13_000,
		HysteresisTokens:       8_000,
		TargetRatio:            0.60,
	})
	if err != nil {
		t.Fatal(err)
	}
	budget, err := budgetForRequest(config, 16_000)
	if err != nil {
		t.Fatal(err)
	}
	if budget.AgentContextWindow != 256_000 || budget.PromptBudget != 240_000 || budget.SoftLimit != 223_000 || budget.HardLimit != 240_000 || budget.TargetTokens != 144_000 {
		t.Fatalf("budget = %#v", budget)
	}
	if budget.AgentContextWindowClamped {
		t.Fatal("1M physical window must not clamp configured 256K working window")
	}

	config.ModelContextWindow = 128_000
	budget, err = budgetForRequest(config, 16_000)
	if err != nil {
		t.Fatal(err)
	}
	if budget.AgentContextWindow != 128_000 || !budget.AgentContextWindowClamped {
		t.Fatalf("clamped budget = %#v", budget)
	}
}

func TestPrepareBeforeModelCallDoesNotRequireRuntimeStorage(t *testing.T) {
	storageRoot := filepath.Join(t.TempDir(), "missing")
	manager, err := New(Config{
		AgentRunID:             "run-without-checkpoint-storage",
		ConfiguredAgentWindowK: 64,
		ModelContextWindow:     128_000,
		MaxOutputTokens:        8_000,
	}, nil, NewFileStore(storageRoot), nil)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = manager.PrepareBeforeModelCall(context.Background(), &adapter.ChatRequest{
		Model:    "gpt-5",
		Messages: []adapter.Message{{Role: "user", Content: "ordinary request"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(storageRoot); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("runtime storage was accessed: %v", statErr)
	}
}

func TestPrepareCountsToolResultsAndToolSchemasInFinalRequest(t *testing.T) {
	manager := newTestManager(t, Config{AgentRunID: "run-count", ConfiguredAgentWindowK: 64, ModelContextWindow: 128_000, MaxOutputTokens: 8_000}, nil)
	request := &adapter.ChatRequest{
		Model: "gpt-5",
		Messages: []adapter.Message{
			{Role: "user", Content: "question"},
			{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "graph-1", Function: adapter.FunctionCall{Name: "knowledge_graph_search", Arguments: `{"query":"zgi"}`}}}},
			{Role: "tool", ToolCallID: "graph-1", Content: strings.Repeat("graph-node relation evidence ", 200)},
		},
		Tools: []adapter.Tool{{Type: "function", Function: adapter.Function{Name: "knowledge_graph_search", Description: strings.Repeat("schema ", 100), Parameters: map[string]interface{}{"type": "object"}}}},
	}
	prepared, decision, err := manager.PrepareBeforeModelCall(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if decision.FinalPromptTokens <= 0 || decision.ComponentTokens["tools"] <= 0 || decision.ToolResultOriginalTokens <= 0 {
		t.Fatalf("decision did not count complete request: %#v", decision)
	}
	withoutToolResult := cloneRequest(prepared)
	withoutToolResult.Messages = withoutToolResult.Messages[:2]
	if manager.estimator.EstimateChatRequest(withoutToolResult).Tokens >= decision.FinalPromptTokens {
		t.Fatalf("tool result did not increase final prompt tokens: decision=%#v", decision)
	}
}

func TestOversizedToolResultProjectionIsStableInMemory(t *testing.T) {
	store := NewFileStore(t.TempDir())
	manager, err := New(Config{
		AgentRunID:             "run-tool-projection",
		ConfiguredAgentWindowK: 64,
		ModelContextWindow:     128_000,
		MaxOutputTokens:        8_000,
		MaxToolResultTokens:    100,
		ToolResultPreviewRunes: 200,
	}, nil, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := &adapter.ChatRequest{Model: "gpt-5", Messages: []adapter.Message{
		{Role: "user", Content: "search"},
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "tool-1", Function: adapter.FunctionCall{Name: "search"}}}},
		{Role: "tool", ToolCallID: "tool-1", Content: strings.Repeat("large result row 123456789 ", 1000)},
	}}
	first, firstDecision, err := manager.PrepareBeforeModelCall(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if firstDecision.Action != DecisionToolProjection || firstDecision.ToolProjectionCount != 1 {
		t.Fatalf("decision = %#v", firstDecision)
	}
	projected := contentString(first.Messages[2].Content)
	for _, expected := range []string{`"status":"projected"`, `"artifact_ref":"agent-context://tool-results/`, `"original_tokens"`} {
		if !strings.Contains(projected, expected) {
			t.Fatalf("projection missing %q: %s", expected, projected)
		}
	}
	second, _, err := manager.PrepareBeforeModelCall(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if contentString(second.Messages[2].Content) != projected {
		t.Fatalf("projection changed between requests:\nfirst=%s\nsecond=%s", projected, contentString(second.Messages[2].Content))
	}
	if state := manager.State(); len(state.ContentReplacements) != 1 {
		t.Fatalf("context replacements = %#v", state.ContentReplacements)
	}
}

func TestTurnTranscriptCapturesToolPairsAndTracksProjectedContent(t *testing.T) {
	store := NewMemoryStore()
	manager, err := New(Config{
		AgentRunID:             "run-turn-transcript",
		ConfiguredAgentWindowK: 64,
		ModelContextWindow:     128_000,
		MaxOutputTokens:        8_000,
		MaxToolResultTokens:    40,
		ToolResultPreviewRunes: 120,
	}, nil, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := &adapter.ChatRequest{Model: "gpt-5", Messages: []adapter.Message{{Role: "user", Content: "inspect both sources"}}}
	prepared, _, err := manager.PrepareBeforeModelCall(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	assistant := adapter.Message{
		Role:             "assistant",
		Content:          "I will inspect both sources.",
		ReasoningContent: "private reasoning must not persist",
		ToolCalls: []adapter.ToolCall{
			{ID: "call-a", Function: adapter.FunctionCall{Name: "search", Arguments: `{"query":"a"}`}},
			{ID: "call-b", Function: adapter.FunctionCall{Name: "read_file", Arguments: `{"path":"b"}`}},
		},
	}
	if err := manager.ObserveModelResponse(context.Background(), assistant, nil); err != nil {
		t.Fatal(err)
	}
	projectedAssistant := cloneMessage(assistant)
	projectedAssistant.ToolCalls[0].Function.Arguments = `{"query":"a","content":"[materialized content omitted]"}`
	messages := append(cloneMessages(prepared.Messages), projectedAssistant,
		adapter.Message{Role: "tool", ToolCallID: "call-a", Content: strings.Repeat("large search result ", 500)},
		adapter.Message{Role: "tool", ToolCallID: "call-b", Content: `{"status":"ok"}`},
	)
	if err := manager.ReplaceMessages(context.Background(), messages); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.PrepareBeforeModelCall(context.Background(), &adapter.ChatRequest{Model: "gpt-5", Messages: messages}); err != nil {
		t.Fatal(err)
	}

	transcript := manager.TurnTranscript()
	if len(transcript) != 3 || transcript[0].Role != "assistant" || transcript[1].ToolCallID != "call-a" || transcript[2].ToolCallID != "call-b" {
		t.Fatalf("turn transcript order = %#v", transcript)
	}
	if transcript[0].ReasoningContent != "" {
		t.Fatalf("reasoning leaked into transcript: %#v", transcript[0])
	}
	if transcript[0].ToolCalls[0].Function.Arguments != projectedAssistant.ToolCalls[0].Function.Arguments {
		t.Fatalf("projected assistant arguments were not synchronized: %#v", transcript[0].ToolCalls[0])
	}
	if !strings.Contains(contentString(transcript[1].Content), `"status":"projected"`) || !strings.Contains(contentString(transcript[1].Content), `"artifact_ref"`) {
		t.Fatalf("projected tool content was not synchronized: %v", transcript[1].Content)
	}
}

func TestProjectedToolResultMicrocompactPreservesOriginalArtifact(t *testing.T) {
	store := NewFileStore(t.TempDir())
	manager, err := New(Config{
		AgentRunID:             "run-receipt",
		ConfiguredAgentWindowK: 64,
		ModelContextWindow:     128_000,
		MaxOutputTokens:        8_000,
		MaxToolResultTokens:    10,
		ToolResultPreviewRunes: 100,
	}, nil, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw := adapter.Message{Role: "tool", ToolCallID: "tool-1", Content: strings.Repeat("original knowledge graph result ", 100)}
	projected, err := manager.toolReplacement(context.Background(), raw, false)
	if err != nil {
		t.Fatal(err)
	}
	compacted, err := manager.toolReplacement(context.Background(), adapter.Message{Role: "tool", ToolCallID: raw.ToolCallID, Content: projected.Replacement}, true)
	if err != nil {
		t.Fatal(err)
	}
	if compacted.ContentHash != projected.ContentHash || compacted.ArtifactRef != projected.ArtifactRef || compacted.OriginalTokens != projected.OriginalTokens {
		t.Fatalf("compacted receipt lost original identity: projected=%#v compacted=%#v", projected, compacted)
	}
	if strings.Contains(compacted.Replacement, `"preview"`) || !strings.Contains(compacted.Replacement, `"status":"compacted"`) {
		t.Fatalf("compacted receipt = %s", compacted.Replacement)
	}
	if len(manager.state.ContentReplacements) != 1 {
		t.Fatalf("replacement count = %d, want stable single record", len(manager.state.ContentReplacements))
	}
}

func TestExpandedContextArtifactSkipsProjectionAndReusesOriginalArtifactDuringMicrocompact(t *testing.T) {
	store := NewMemoryStore()
	manager, err := New(Config{
		AgentRunID:             "run-expanded-artifact",
		ConfiguredAgentWindowK: 64,
		ModelContextWindow:     128_000,
		MaxToolResultTokens:    1,
	}, nil, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	original := strings.Repeat("original artifact evidence ", 1_000)
	hashBytes := sha256.Sum256([]byte(original))
	contentHash := hex.EncodeToString(hashBytes[:])
	artifactRef, err := store.Put(context.Background(), "run-expanded-artifact", contentHash, original)
	if err != nil {
		t.Fatal(err)
	}
	expanded, err := FormatContextArtifactToolResult(ContextArtifactResult{
		ArtifactRef:      artifactRef,
		ContentHash:      contentHash,
		SourceToolCallID: "search-1",
		Content:          original,
		ReturnedTokens:   10_000,
		TotalTokens:      10_000,
	})
	if err != nil {
		t.Fatal(err)
	}

	projected, stats, err := manager.projectOversizedToolResults(context.Background(), []adapter.Message{{Role: "tool", ToolCallID: "read-1", Content: expanded}})
	if err != nil {
		t.Fatal(err)
	}
	if stats.count != 0 || contentString(projected[0].Content) != expanded || len(store.results) != 1 {
		t.Fatalf("expanded artifact was recursively projected: stats=%#v results=%d", stats, len(store.results))
	}

	pending := []adapter.Message{
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "read-1", Function: adapter.FunctionCall{Name: "read_context_artifact"}}}},
		{Role: "tool", ToolCallID: "read-1", Content: expanded},
	}
	pendingCompacted, changed := manager.microcompactOldToolResults(context.Background(), pending)
	if changed || contentString(pendingCompacted[1].Content) != expanded {
		t.Fatalf("pending artifact read was compacted before the model consumed it: %s", contentString(pendingCompacted[1].Content))
	}

	messages := append(cloneMessages(pending), adapter.Message{Role: "assistant", Content: "consumed restored evidence"})
	messages = append(messages, adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "new-1"}, {ID: "new-2"}, {ID: "new-3"}, {ID: "new-4"}}})
	for index := 1; index <= 4; index++ {
		messages = append(messages, adapter.Message{Role: "tool", ToolCallID: fmt.Sprintf("new-%d", index), Content: fmt.Sprintf("new result %d", index)})
	}
	compacted, changed := manager.microcompactOldToolResults(context.Background(), messages)
	if !changed {
		t.Fatal("consumed artifact read did not participate in ordinary microcompact")
	}
	receipt, ok := decodeToolResultReceipt(contentString(compacted[1].Content))
	if !ok || fmt.Sprint(receipt["kind"]) != contextArtifactReadKind || fmt.Sprint(receipt["artifact_ref"]) != artifactRef || fmt.Sprint(receipt["source_tool_call_id"]) != "search-1" {
		t.Fatalf("compacted artifact read receipt = %#v", receipt)
	}
	if strings.Contains(contentString(compacted[1].Content), original[:100]) {
		t.Fatalf("compacted artifact read retained expanded content: %s", contentString(compacted[1].Content))
	}
	if len(store.results) != 1 {
		t.Fatalf("microcompact created a recursive artifact: results=%d", len(store.results))
	}
}

func TestMicrocompactProtectsEntireLatestParallelToolBatch(t *testing.T) {
	store := NewMemoryStore()
	manager, err := New(Config{
		AgentRunID:             "run-parallel-protection",
		ConfiguredAgentWindowK: 64,
		ModelContextWindow:     128_000,
		MaxOutputTokens:        8_000,
	}, nil, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	messages := []adapter.Message{
		{Role: "user", Content: "run tools"},
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "old-1"}, {ID: "old-2"}}},
		{Role: "tool", ToolCallID: "old-1", Content: "old result one"},
		{Role: "tool", ToolCallID: "old-2", Content: "old result two"},
		{Role: "assistant", Content: "continue with a larger parallel batch", ToolCalls: []adapter.ToolCall{
			{ID: "new-1"}, {ID: "new-2"}, {ID: "new-3"}, {ID: "new-4"}, {ID: "new-5"}, {ID: "new-6"},
		}},
	}
	for index := 1; index <= 6; index++ {
		messages = append(messages, adapter.Message{Role: "tool", ToolCallID: fmt.Sprintf("new-%d", index), Content: fmt.Sprintf("new result %d", index)})
	}

	compacted, changed := manager.microcompactOldToolResults(context.Background(), messages)
	if !changed {
		t.Fatal("microcompact changed = false, want old results compacted")
	}
	for index, message := range compacted {
		content := contentString(message.Content)
		switch strings.TrimSpace(message.ToolCallID) {
		case "old-1", "old-2":
			if !strings.Contains(content, `"status":"compacted"`) {
				t.Fatalf("old result at %d was not compacted: %s", index, content)
			}
		case "new-1", "new-2", "new-3", "new-4", "new-5", "new-6":
			if strings.Contains(content, `"status":"compacted"`) {
				t.Fatalf("latest parallel result %s was compacted: %s", message.ToolCallID, content)
			}
		}
	}
}

func TestMicrocompactProtectsAtLeastFourResultsWhenLatestBatchIsSmaller(t *testing.T) {
	store := NewMemoryStore()
	manager, err := New(Config{AgentRunID: "run-minimum-protection", ConfiguredAgentWindowK: 64, ModelContextWindow: 128_000}, nil, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	messages := []adapter.Message{
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "old-1"}, {ID: "old-2"}, {ID: "old-3"}}},
		{Role: "tool", ToolCallID: "old-1", Content: "old result one"},
		{Role: "tool", ToolCallID: "old-2", Content: "old result two"},
		{Role: "tool", ToolCallID: "old-3", Content: "old result three"},
		{Role: "assistant", Content: "consumed old batch"},
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "new-1"}, {ID: "new-2"}}},
		{Role: "tool", ToolCallID: "new-1", Content: "new result one"},
		{Role: "tool", ToolCallID: "new-2", Content: "new result two"},
	}

	compacted, changed := manager.microcompactOldToolResults(context.Background(), messages)
	if !changed {
		t.Fatal("microcompact changed = false, want oldest result compacted")
	}
	byID := map[string]string{}
	for _, message := range compacted {
		if message.ToolCallID != "" {
			byID[message.ToolCallID] = contentString(message.Content)
		}
	}
	if !strings.Contains(byID["old-1"], `"status":"compacted"`) {
		t.Fatalf("oldest result was not compacted: %s", byID["old-1"])
	}
	for _, id := range []string{"old-2", "old-3", "new-1", "new-2"} {
		if strings.Contains(byID[id], `"status":"compacted"`) {
			t.Fatalf("protected result %s was compacted: %s", id, byID[id])
		}
	}
}

func TestMicrocompactDoesNotProtectAnAlreadyConsumedParallelBatch(t *testing.T) {
	store := NewMemoryStore()
	manager, err := New(Config{AgentRunID: "run-consumed-batch", ConfiguredAgentWindowK: 64, ModelContextWindow: 128_000}, nil, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	messages := []adapter.Message{
		{Role: "assistant", ToolCalls: []adapter.ToolCall{
			{ID: "tool-1"}, {ID: "tool-2"}, {ID: "tool-3"}, {ID: "tool-4"}, {ID: "tool-5"}, {ID: "tool-6"},
		}},
	}
	for index := 1; index <= 6; index++ {
		messages = append(messages, adapter.Message{Role: "tool", ToolCallID: fmt.Sprintf("tool-%d", index), Content: fmt.Sprintf("result %d", index)})
	}
	messages = append(messages,
		adapter.Message{Role: "assistant", Content: "I consumed all six results."},
		adapter.Message{Role: "user", Content: "start the next task"},
	)

	compacted, changed := manager.microcompactOldToolResults(context.Background(), messages)
	if !changed {
		t.Fatal("microcompact changed = false, want older consumed results compacted")
	}
	for _, message := range compacted {
		if message.ToolCallID == "tool-1" || message.ToolCallID == "tool-2" {
			if !strings.Contains(contentString(message.Content), `"status":"compacted"`) {
				t.Fatalf("consumed result %s remained protected: %s", message.ToolCallID, contentString(message.Content))
			}
		}
	}
}

func TestHardLimitFairlyShrinksEveryPendingParallelToolPreview(t *testing.T) {
	store := NewMemoryStore()
	manager, err := New(Config{
		AgentRunID:             "run-fair-pending-batch",
		ConfiguredAgentWindowK: 20,
		ModelContextWindow:     128_000,
		MaxOutputTokens:        4_000,
		SummaryOutputTokens:    4_000,
		EmergencyBufferTokens:  3_000,
		HysteresisTokens:       1_000,
		MaxToolResultTokens:    100_000,
	}, nil, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	calls := make([]adapter.ToolCall, 0, 6)
	messages := []adapter.Message{{Role: "user", Content: "inspect every parallel result"}}
	for index := 1; index <= 6; index++ {
		calls = append(calls, adapter.ToolCall{ID: fmt.Sprintf("parallel-%d", index), Function: adapter.FunctionCall{Name: "search"}})
	}
	messages = append(messages, adapter.Message{Role: "assistant", ToolCalls: calls})
	for index := 1; index <= 6; index++ {
		messages = append(messages, adapter.Message{
			Role:       "tool",
			ToolCallID: fmt.Sprintf("parallel-%d", index),
			Content:    strings.Repeat(fmt.Sprintf("result-%d evidence ", index), 2_000),
		})
	}

	prepared, decision, err := manager.PrepareBeforeModelCall(context.Background(), &adapter.ChatRequest{Model: "gpt-5", Messages: messages})
	if err != nil {
		t.Fatal(err)
	}
	if decision.FinalPromptTokens > decision.Budget.HardLimit {
		t.Fatalf("final prompt = %d, hard limit = %d", decision.FinalPromptTokens, decision.Budget.HardLimit)
	}
	if decision.ToolProjectionCount != 6 {
		t.Fatalf("projection count = %d, want 6", decision.ToolProjectionCount)
	}
	seen := 0
	for _, message := range prepared.Messages {
		if !strings.HasPrefix(message.ToolCallID, "parallel-") {
			continue
		}
		receipt, ok := decodeToolResultReceipt(contentString(message.Content))
		if !ok || !strings.EqualFold(fmt.Sprint(receipt["status"]), "projected") {
			t.Fatalf("parallel result %s was not projected: %s", message.ToolCallID, contentString(message.Content))
		}
		if preview := strings.TrimSpace(fmt.Sprint(receipt["preview"])); preview == "" || preview == "<nil>" {
			t.Fatalf("parallel result %s lost its preview: %s", message.ToolCallID, contentString(message.Content))
		}
		seen++
	}
	if seen != 6 {
		t.Fatalf("projected pending results = %d, want 6", seen)
	}
}

func TestToolPairingRequiresEveryParallelCallResult(t *testing.T) {
	messages := []adapter.Message{
		{Role: "user", Content: "run three tools"},
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "a"}, {ID: "b"}, {ID: "c"}}},
		{Role: "tool", ToolCallID: "a", Content: `{"status":"ok"}`},
		{Role: "tool", ToolCallID: "b", Content: `{"status":"ok"}`},
	}
	if err := validateToolPairing(messages); err == nil || !strings.Contains(err.Error(), `"c"`) {
		t.Fatalf("validateToolPairing() error = %v, want missing result for c", err)
	}
	messages = append(messages, adapter.Message{Role: "tool", ToolCallID: "c", Content: `{"status":"ok"}`})
	if err := validateToolPairing(messages); err != nil {
		t.Fatal(err)
	}
	rounds := groupMessagesByAPIRound(messages)
	if len(rounds) != 2 || len(rounds[1].Messages) != 4 {
		t.Fatalf("parallel tool round grouping = %#v", rounds)
	}
}

func TestToolPairingRejectsToolCallWithoutID(t *testing.T) {
	messages := []adapter.Message{
		{Role: "user", Content: "run a tool"},
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{Function: adapter.FunctionCall{Name: "search"}}}},
	}
	if err := validateToolPairing(messages); err == nil || !strings.Contains(err.Error(), "missing id") {
		t.Fatalf("validateToolPairing() error = %v, want missing id", err)
	}
}

func TestAPIRoundAdvancesOnlyAfterModelResponse(t *testing.T) {
	manager := newTestManager(t, Config{AgentRunID: "run-round-sequence", ConfiguredAgentWindowK: 64, ModelContextWindow: 128_000, MaxOutputTokens: 8_000}, nil)
	request := &adapter.ChatRequest{Model: "gpt-5", Messages: []adapter.Message{{Role: "user", Content: "question"}}}

	_, first, err := manager.PrepareBeforeModelCall(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	_, retry, err := manager.PrepareBeforeModelCall(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.APIRound != 1 || retry.APIRound != 1 || manager.State().NextRound != 1 {
		t.Fatalf("preflight advanced round: first=%d retry=%d state=%d", first.APIRound, retry.APIRound, manager.State().NextRound)
	}
	if err := manager.ObserveModelResponse(context.Background(), adapter.Message{Role: "assistant", Content: "answer"}, nil); err != nil {
		t.Fatal(err)
	}
	_, second, err := manager.PrepareBeforeModelCall(context.Background(), &adapter.ChatRequest{Model: "gpt-5", Messages: manager.State().Messages})
	if err != nil {
		t.Fatal(err)
	}
	if second.APIRound != 2 || manager.State().NextRound != 2 {
		t.Fatalf("response did not advance round: decision=%d state=%d", second.APIRound, manager.State().NextRound)
	}
}

func TestAPIRoundGroupingKeepsAbsoluteRunSequences(t *testing.T) {
	messages := []adapter.Message{
		{Role: "user", Content: "bootstrap"},
		{Role: "assistant", Content: "bootstrap answer"},
		{Role: "user", Content: "current task"},
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "a"}}},
		{Role: "tool", ToolCallID: "a", Content: "a result"},
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "b"}}},
		{Role: "tool", ToolCallID: "b", Content: "b result"},
	}
	rounds := groupMessagesByAPIRoundForRun(messages, 3)
	if len(rounds) != 4 {
		t.Fatalf("round count = %d", len(rounds))
	}
	if rounds[1].Sequence != 0 || rounds[2].Sequence != 1 || rounds[3].Sequence != 2 {
		t.Fatalf("absolute sequences = %d, %d, %d", rounds[1].Sequence, rounds[2].Sequence, rounds[3].Sequence)
	}
}

func TestHardLimitReturnsContextExhaustedAndKeepsInMemoryState(t *testing.T) {
	manager, err := New(Config{
		AgentRunID:             "run-hard-limit",
		ConfiguredAgentWindowK: 32,
		ModelContextWindow:     128_000,
		MaxOutputTokens:        4_000,
		SummaryOutputTokens:    4_000,
		EmergencyBufferTokens:  3_000,
		HysteresisTokens:       1_000,
	}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = manager.PrepareBeforeModelCall(context.Background(), &adapter.ChatRequest{
		Model:     "gpt-5",
		MaxTokens: intPointerForTest(2_000),
		Messages:  []adapter.Message{{Role: "user", Content: strings.Repeat("uncompressible current request ", 20_000)}},
	})
	if !errors.Is(err, ErrContextExhausted) {
		t.Fatalf("error = %v, want context exhausted", err)
	}
	if state := manager.State(); len(state.Messages) == 0 {
		t.Fatalf("context state = %#v", state)
	}
}

func TestHardLimitForcesFinalRecoveryAfterProactiveCompactionCircuitBreaker(t *testing.T) {
	compactor := &recordingCompactor{summary: `<summary>Earlier work is preserved in a compact form.</summary>`}
	manager := newTestManager(t, Config{
		AgentRunID:             "run-final-recovery",
		ConfiguredAgentWindowK: 32,
		ModelContextWindow:     128_000,
		MaxOutputTokens:        4_000,
		SummaryOutputTokens:    4_000,
		EmergencyBufferTokens:  3_000,
		HysteresisTokens:       1_000,
		TailMinTextRounds:      2,
	}, compactor)
	manager.state.Compaction.ConsecutiveFailures = maxConsecutiveCompactionFailures
	request := &adapter.ChatRequest{
		Model:     "test-model",
		MaxTokens: intPointerForTest(2_000),
		Messages: []adapter.Message{
			{Role: "user", Content: strings.Repeat("old ", 15_000)},
			{Role: "assistant", Content: strings.Repeat("middle ", 3_000)},
			{Role: "assistant", Content: strings.Repeat("recent ", 3_000)},
		},
	}
	budget, err := budgetForRequest(manager.config, *request.MaxTokens)
	if err != nil {
		t.Fatal(err)
	}
	if before := manager.estimator.EstimateChatRequest(request).Tokens; before <= budget.HardLimit {
		t.Fatalf("test request tokens = %d, want above hard limit %d", before, budget.HardLimit)
	}

	prepared, decision, err := manager.PrepareBeforeModelCall(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != DecisionReactiveCompact || decision.FinalPromptTokens > decision.Budget.HardLimit {
		t.Fatalf("final recovery decision = %#v", decision)
	}
	if len(compactor.calls) != 1 || compactor.calls[0].Type != RequestTypeReactiveCompact {
		t.Fatalf("compact calls = %#v, want one reactive compact", compactor.calls)
	}
	if manager.State().Compaction.ConsecutiveFailures != 0 {
		t.Fatalf("failure count was not reset: %#v", manager.State().Compaction)
	}
	if !strings.Contains(messagesString(prepared.Messages), "Earlier work is preserved") {
		t.Fatalf("recovered request did not contain the compact summary: %#v", prepared.Messages)
	}
}

func TestHardLimitStopsAfterFinalRecoveryFails(t *testing.T) {
	compactor := &recordingCompactor{failures: []error{errors.New("summary provider unavailable")}}
	manager := newTestManager(t, Config{
		AgentRunID:             "run-final-recovery-failed",
		ConfiguredAgentWindowK: 32,
		ModelContextWindow:     128_000,
		MaxOutputTokens:        4_000,
		SummaryOutputTokens:    4_000,
		EmergencyBufferTokens:  3_000,
		HysteresisTokens:       1_000,
		TailMinTextRounds:      2,
	}, compactor)
	manager.state.Compaction.ConsecutiveFailures = maxConsecutiveCompactionFailures
	request := &adapter.ChatRequest{
		Model:     "test-model",
		MaxTokens: intPointerForTest(2_000),
		Messages: []adapter.Message{
			{Role: "user", Content: strings.Repeat("old ", 15_000)},
			{Role: "assistant", Content: strings.Repeat("middle ", 3_000)},
			{Role: "assistant", Content: strings.Repeat("recent ", 3_000)},
		},
	}

	_, decision, err := manager.PrepareBeforeModelCall(context.Background(), request)
	if !errors.Is(err, ErrContextExhausted) || !strings.Contains(err.Error(), "final recovery compaction failed") {
		t.Fatalf("error = %v, want failed final recovery context exhaustion", err)
	}
	if decision.CompactionFailure != "summary provider unavailable" {
		t.Fatalf("decision = %#v", decision)
	}
	if len(compactor.calls) != 1 || compactor.calls[0].Type != RequestTypeReactiveCompact {
		t.Fatalf("compact calls = %#v, want one reactive compact", compactor.calls)
	}
}

func TestLongAgentRunSurvivesOneHundredToolRoundsAndRepeatedCompaction(t *testing.T) {
	compactor := &recordingCompactor{summary: `<summary>The earlier API rounds completed successfully. Preserve the current task constraints and continue from the verbatim recent rounds.</summary>`}
	manager := newTestManager(t, Config{
		AgentRunID:             "run-one-hundred-rounds",
		ConfiguredAgentWindowK: 32,
		ModelContextWindow:     128_000,
		MaxInputTokens:         128_000,
		MaxOutputTokens:        4_000,
		SummaryOutputTokens:    4_000,
		EmergencyBufferTokens:  3_000,
		HysteresisTokens:       1_000,
		TailMinTextRounds:      2,
	}, compactor)
	messages := []adapter.Message{{Role: "user", Content: "complete a long Agent task"}}
	for round := 1; round <= 100; round++ {
		prepared, decision, err := manager.PrepareBeforeModelCall(context.Background(), &adapter.ChatRequest{
			Model:     "gpt-5",
			MaxTokens: intPointerForTest(2_000),
			Messages:  messages,
		})
		if err != nil {
			t.Fatalf("round %d preflight: %v", round, err)
		}
		if decision.APIRound != round {
			t.Fatalf("round %d decision sequence = %d", round, decision.APIRound)
		}
		messages = cloneMessages(prepared.Messages)
		callID := fmt.Sprintf("tool-%03d", round)
		assistant := adapter.Message{
			Role:    "assistant",
			Content: strings.Repeat("model-visible plan and accumulated implementation judgment ", 500),
			ToolCalls: []adapter.ToolCall{{
				ID:       callID,
				Function: adapter.FunctionCall{Name: "work_step", Arguments: fmt.Sprintf(`{"round":%d}`, round)},
			}},
		}
		if err := manager.ObserveModelResponse(context.Background(), assistant, &adapter.Usage{PromptTokens: decision.FinalPromptTokens}); err != nil {
			t.Fatalf("round %d response state update: %v", round, err)
		}
		messages = append(messages, assistant, adapter.Message{Role: "tool", ToolCallID: callID, Content: fmt.Sprintf(`{"status":"ok","round":%d}`, round)})
		if err := manager.ReplaceMessages(context.Background(), messages); err != nil {
			t.Fatalf("round %d tool state update: %v", round, err)
		}
	}
	if manager.State().NextRound != 101 {
		t.Fatalf("next round = %d, want 101", manager.State().NextRound)
	}
	if len(compactor.requests) < 2 {
		t.Fatalf("semantic compactions = %d, want at least 2", len(compactor.requests))
	}
	if err := validateToolPairing(manager.State().Messages); err != nil {
		t.Fatalf("final transcript tool pairing: %v", err)
	}
}

func TestSemanticCompactionUsesClaudeUpToPromptAndPreservesRecentToolRounds(t *testing.T) {
	compactor := &recordingCompactor{summary: `<analysis>draft only</analysis><summary>1. Primary Request and Intent:
continue the long task

9. Context for Continuing Work:
the recent tool rounds remain authoritative</summary>`}
	manager := newTestManager(t, Config{
		AgentRunID:             "run-compact",
		ConfiguredAgentWindowK: 32,
		ModelContextWindow:     128_000,
		MaxInputTokens:         128_000,
		MaxOutputTokens:        4_000,
		SummaryOutputTokens:    4_000,
		EmergencyBufferTokens:  3_000,
		HysteresisTokens:       1_000,
		TargetRatio:            0.60,
		TailMinTextRounds:      2,
	}, compactor)
	longHistory := strings.Repeat("historical requirement number 12345 with implementation detail. ", 2600)
	request := &adapter.ChatRequest{Model: "gpt-5", MaxTokens: intPointerForTest(2_000), Messages: []adapter.Message{
		{Role: "system", Content: "system rules"},
		{Role: "user", Content: longHistory},
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "call-1", Function: adapter.FunctionCall{Name: "read_file"}}}},
		{Role: "tool", ToolCallID: "call-1", Content: "first recent tool result"},
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "call-2", Function: adapter.FunctionCall{Name: "search"}}}},
		{Role: "tool", ToolCallID: "call-2", Content: "latest recent tool result"},
	}}
	prepared, decision, err := manager.PrepareBeforeModelCall(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != DecisionSemanticCompact || len(compactor.requests) != 1 {
		t.Fatalf("decision=%#v compact requests=%d", decision, len(compactor.requests))
	}
	compactRequest := compactor.requests[0]
	if len(compactRequest.Tools) != 0 || compactRequest.ToolChoice != nil {
		t.Fatalf("compact request carried tools: %#v", compactRequest)
	}
	compactPrompt := contentString(compactRequest.Messages[len(compactRequest.Messages)-1].Content)
	for _, expected := range []string{"CRITICAL: Respond with TEXT ONLY", "newer messages that build on this context will follow", "REMINDER: Do NOT call any tools"} {
		if !strings.Contains(compactPrompt, expected) {
			t.Fatalf("compact prompt missing %q", expected)
		}
	}
	encoded := contentString(prepared.Messages[1].Content)
	if strings.Contains(encoded, "draft only") || !strings.Contains(encoded, "Recent messages are preserved verbatim") {
		t.Fatalf("formatted summary = %q", encoded)
	}
	if err := validateToolPairing(prepared.Messages); err != nil {
		t.Fatal(err)
	}
	all := messagesString(prepared.Messages)
	for _, expected := range []string{"call-1", "first recent tool result", "call-2", "latest recent tool result"} {
		if !strings.Contains(all, expected) {
			t.Fatalf("compacted messages lost %q: %s", expected, all)
		}
	}
}

func TestSemanticCompactionRetriesPromptTooLongByDroppingOldestAPIRound(t *testing.T) {
	compactor := &recordingCompactor{
		summary:  `<summary>older work is summarized and the recent rounds remain authoritative</summary>`,
		failures: []error{errors.New("context_length_exceeded")},
	}
	manager := newTestManager(t, Config{
		AgentRunID:             "run-compact-retry",
		ConfiguredAgentWindowK: 32,
		ModelContextWindow:     128_000,
		MaxInputTokens:         128_000,
		MaxOutputTokens:        4_000,
		SummaryOutputTokens:    4_000,
		EmergencyBufferTokens:  3_000,
		HysteresisTokens:       1_000,
		TargetRatio:            0.60,
		TailMinTextRounds:      1,
	}, compactor)
	request := &adapter.ChatRequest{Model: "gpt-5", MaxTokens: intPointerForTest(2_000), Messages: []adapter.Message{
		{Role: "user", Content: strings.Repeat("old bootstrap requirement ", 6500)},
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "call-1", Function: adapter.FunctionCall{Name: "read_file"}}}},
		{Role: "tool", ToolCallID: "call-1", Content: strings.Repeat("old file result ", 300)},
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "call-2", Function: adapter.FunctionCall{Name: "search"}}}},
		{Role: "tool", ToolCallID: "call-2", Content: strings.Repeat("x ", 5000)},
	}}
	_, decision, err := manager.PrepareBeforeModelCall(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != DecisionSemanticCompact || len(compactor.requests) != 2 {
		t.Fatalf("decision=%#v compact requests=%d", decision, len(compactor.requests))
	}
	if len(compactor.requests[1].Messages) >= len(compactor.requests[0].Messages) {
		t.Fatalf("retry did not drop the oldest API round: first=%d second=%d", len(compactor.requests[0].Messages), len(compactor.requests[1].Messages))
	}
}

type recordingCompactor struct {
	summary  string
	requests []*adapter.ChatRequest
	calls    []CompactCall
	failures []error
}

func (c *recordingCompactor) Compact(_ context.Context, request *adapter.ChatRequest, call CompactCall) (string, *adapter.Usage, error) {
	c.requests = append(c.requests, cloneRequest(request))
	c.calls = append(c.calls, call)
	if len(c.failures) > 0 {
		err := c.failures[0]
		c.failures = c.failures[1:]
		return "", nil, err
	}
	return c.summary, &adapter.Usage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120}, nil
}

func newTestManager(t *testing.T, config Config, compactor Compactor) *Manager {
	t.Helper()
	manager, err := New(config, compactor, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func intPointerForTest(value int) *int { return &value }

func messagesString(messages []adapter.Message) string {
	var builder strings.Builder
	for _, message := range messages {
		builder.WriteString(contentString(message.Content))
		for _, call := range message.ToolCalls {
			builder.WriteString(call.ID)
		}
	}
	return builder.String()
}
