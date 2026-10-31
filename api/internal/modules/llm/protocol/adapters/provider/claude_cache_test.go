package provider

import (
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// A system message after the conversation starts stays in place as a
// reminder; hoisting it into the system prompt would change the cached prefix
// on every turn (ADR 0032).
func TestClaudeKeepsLaterSystemMessagesInPlace(t *testing.T) {
	a, err := NewClaudeAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: "https://api.anthropic.test/v1"})
	if err != nil {
		t.Fatal(err)
	}
	req, err := a.convertToClaude(&adapter.ChatRequest{
		Model: "claude-sonnet-4-5",
		Messages: []adapter.Message{
			{Role: "system", Content: "persona"},
			{Role: "user", Content: "earlier question"},
			{Role: "assistant", Content: "earlier answer"},
			{Role: "system", Content: "Current time: 15:00"},
			{Role: "user", Content: "new question"},
			{Role: "assistant", Content: "", ToolCalls: []adapter.ToolCall{{ID: "toolu_1", Type: "function", Function: adapter.FunctionCall{Name: "lookup", Arguments: `{}`}}}},
			{Role: "tool", ToolCallID: "toolu_1", Content: "result"},
			{Role: "system", Content: "Skill instructions"},
		},
		Tools: []adapter.Tool{
			{Type: "function", Function: adapter.Function{Name: "lookup", Parameters: map[string]interface{}{"type": "object"}}},
			{Type: "function", Function: adapter.Function{Name: "search", Parameters: map[string]interface{}{"type": "object"}}},
		},
	})
	if err != nil {
		t.Fatalf("convertToClaude() error = %v", err)
	}
	if len(req.System) != 1 || req.System[0].Text != "persona" {
		t.Fatalf("system = %#v, want only the leading persona", req.System)
	}
	roles := make([]string, 0, len(req.Messages))
	for _, message := range req.Messages {
		roles = append(roles, message.Role)
	}
	if strings.Join(roles, ",") != "user,assistant,user,assistant,user" {
		t.Fatalf("roles = %v, want alternating turns", roles)
	}

	turn := claudeBlocks(req.Messages[2].Content)
	if len(turn) != 2 || !strings.Contains(turn[0].Text, "<system-reminder>\nCurrent time: 15:00\n</system-reminder>") || turn[1].Text != "new question" {
		t.Fatalf("current turn = %#v, want the reminder before the question", turn)
	}
	results := claudeBlocks(req.Messages[4].Content)
	if len(results) != 2 || results[0].Type != "tool_result" || !strings.Contains(results[1].Text, "Skill instructions") {
		t.Fatalf("tool turn = %#v, want the tool result first, then the reminder", results)
	}

	// Breakpoints: the last tool, the system prompt, and the last block.
	if req.Tools[0].CacheControl != nil || req.Tools[1].CacheControl == nil {
		t.Fatalf("tool breakpoints = %v, %v", req.Tools[0].CacheControl, req.Tools[1].CacheControl)
	}
	if req.System[0].CacheControl == nil {
		t.Fatal("system prompt has no breakpoint")
	}
	if results[1].CacheControl == nil || results[0].CacheControl != nil {
		t.Fatalf("last message breakpoints = %#v", results)
	}
	marked := 0
	for _, message := range req.Messages {
		for _, block := range claudeBlocks(message.Content) {
			if block.CacheControl != nil {
				marked++
			}
		}
	}
	if marked != 1 {
		t.Fatalf("message breakpoints = %d, want 1 (Claude allows four in all)", marked)
	}
}

func TestClaudeBreakpointOnPlainTextMessage(t *testing.T) {
	a, err := NewClaudeAdapter(&adapter.AdapterConfig{APIKey: "test-key", BaseURL: "https://api.anthropic.test/v1"})
	if err != nil {
		t.Fatal(err)
	}
	req, err := a.convertToClaude(&adapter.ChatRequest{
		Model:    "claude-sonnet-4-5",
		Messages: []adapter.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	blocks := claudeBlocks(req.Messages[0].Content)
	if len(blocks) != 1 || blocks[0].Text != "hello" || blocks[0].CacheControl == nil || req.System != nil {
		t.Fatalf("request = %#v", req)
	}
}
