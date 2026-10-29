package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/tokenestimate"
)

// A channel turn's supplemental history policy applies to the history the
// turn sends (ADR 0038): it was set but never reached.
func TestConversationHistoryAppliesTheSupplementalPolicy(t *testing.T) {
	branch := []*runtimemodel.Message{
		{ID: uuid.New(), Query: "old one", Answer: "old answer one"},
		{ID: uuid.New(), Query: "old two", Answer: "old answer two"},
		{ID: uuid.New(), Query: "duplicated provider text", Answer: "duplicate answer"},
		{ID: uuid.New(), Query: "recent one", Answer: "recent answer one"},
		{ID: uuid.New(), Query: "recent two", Answer: "recent answer two"},
	}
	parts := &chatRequestParts{HistoryPolicy: &runtimedto.ConversationHistoryPolicy{
		Mode: runtimedto.ConversationHistoryModeSupplemental, MaxTurns: 3,
		ExcludedTexts: []string{"duplicated provider text"},
	}}
	messages, err := (&service{}).conversationHistoryMessages(context.Background(), branch, parts)
	if err != nil {
		t.Fatal(err)
	}
	text := ""
	for _, message := range messages {
		text += messageContentText(message.Content) + "\n"
	}
	if strings.Contains(text, "old one") || strings.Contains(text, "duplicated provider text") || !strings.Contains(text, "recent two") {
		t.Fatalf("history = %q, want only the newest three non-duplicate turns", text)
	}
}

// An oversized input is left to the native loop's context manager instead of
// being refused; other loops still refuse it.
func TestOversizedInputIsBudgetedForTheNativeLoop(t *testing.T) {
	svc := &service{tokenEstimator: tokenestimate.NewEstimator()}
	spec := ModelSpec{ContextWindow: 32_000}
	huge := []adapter.Message{{Role: "user", Content: strings.Repeat("a long pasted document ", 20_000)}}
	native := &chatRequestParts{ExecutionMode: executionModeNativeAgentLoop, ModelName: "test-model", Parameters: map[string]interface{}{}}
	if _, err := svc.computeContextBudget(spec, native, huge); err != nil {
		t.Fatalf("native loop: %v, want the input budgeted", err)
	}
	direct := &chatRequestParts{ExecutionMode: executionModeDirectChat, ModelName: "test-model", Parameters: map[string]interface{}{}}
	if _, err := svc.computeContextBudget(spec, direct, huge); err == nil {
		t.Fatal("direct chat accepted an input larger than the window")
	}
}
