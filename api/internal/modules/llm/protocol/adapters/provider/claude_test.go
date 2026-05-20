package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

func TestClaudeAdapterConvertToClaude_UsesBlocksAndToolUseID(t *testing.T) {
	t.Helper()

	a, err := NewClaudeAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: "https://api.anthropic.test/v1",
	})
	if err != nil {
		t.Fatalf("NewClaudeAdapter() error = %v", err)
	}

	req, err := a.convertToClaude(&adapter.ChatRequest{
		Model: "claude-sonnet-4-0",
		Messages: []adapter.Message{
			{Role: "system", Content: "system-a"},
			{Role: "system", Content: "system-b"},
			{
				Role: "user",
				Content: []adapter.MessageContentPart{
					{Type: "text", Text: "look"},
					{Type: "image_url", ImageURL: &adapter.ImageURL{URL: "data:image/png;base64,aGVsbG8="}},
				},
			},
			{
				Role:    "assistant",
				Content: "calling tool",
				ToolCalls: []adapter.ToolCall{
					{
						ID:   "toolu_1",
						Type: "function",
						Function: adapter.FunctionCall{
							Name:      "lookup_weather",
							Arguments: `{"city":"shanghai"}`,
						},
					},
				},
			},
			{
				Role:       "tool",
				ToolCallID: "toolu_1",
				Content:    "sunny",
			},
		},
	})
	if err != nil {
		t.Fatalf("convertToClaude() error = %v", err)
	}

	if req.System != "system-a\n\nsystem-b" {
		t.Fatalf("system = %q, want %q", req.System, "system-a\n\nsystem-b")
	}
	if len(req.Messages) != 3 {
		t.Fatalf("len(messages) = %d, want 3", len(req.Messages))
	}

	userBlocks, ok := req.Messages[0].Content.([]claudeContentBlock)
	if !ok {
		t.Fatalf("user message content type = %T, want []claudeContentBlock", req.Messages[0].Content)
	}
	if req.Messages[0].Role != "user" {
		t.Fatalf("user message role = %q, want %q", req.Messages[0].Role, "user")
	}
	if len(userBlocks) != 2 {
		t.Fatalf("len(userBlocks) = %d, want 2", len(userBlocks))
	}
	if userBlocks[0].Type != "text" || userBlocks[0].Text != "look" {
		t.Fatalf("user text block = %#v, want text=look", userBlocks[0])
	}
	if userBlocks[1].Type != "image" || userBlocks[1].Source == nil {
		t.Fatalf("user image block = %#v, want image source", userBlocks[1])
	}
	if userBlocks[1].Source.Type != "base64" || userBlocks[1].Source.MediaType != "image/png" || userBlocks[1].Source.Data != "aGVsbG8=" {
		t.Fatalf("image source = %#v, want base64 png data", userBlocks[1].Source)
	}

	assistantBlocks, ok := req.Messages[1].Content.([]claudeContentBlock)
	if !ok {
		t.Fatalf("assistant message content type = %T, want []claudeContentBlock", req.Messages[1].Content)
	}
	if req.Messages[1].Role != "assistant" {
		t.Fatalf("assistant message role = %q, want %q", req.Messages[1].Role, "assistant")
	}
	if len(assistantBlocks) != 2 {
		t.Fatalf("len(assistantBlocks) = %d, want 2", len(assistantBlocks))
	}
	if assistantBlocks[0].Type != "text" || assistantBlocks[0].Text != "calling tool" {
		t.Fatalf("assistant text block = %#v, want text block", assistantBlocks[0])
	}
	if assistantBlocks[1].Type != "tool_use" || assistantBlocks[1].ID != "toolu_1" || assistantBlocks[1].Name != "lookup_weather" {
		t.Fatalf("assistant tool_use block = %#v, want id/name", assistantBlocks[1])
	}

	toolResultBlocks, ok := req.Messages[2].Content.([]claudeContentBlock)
	if !ok {
		t.Fatalf("tool result content type = %T, want []claudeContentBlock", req.Messages[2].Content)
	}
	if req.Messages[2].Role != "user" {
		t.Fatalf("tool result role = %q, want %q", req.Messages[2].Role, "user")
	}
	if len(toolResultBlocks) != 1 {
		t.Fatalf("len(toolResultBlocks) = %d, want 1", len(toolResultBlocks))
	}
	if toolResultBlocks[0].Type != "tool_result" || toolResultBlocks[0].ToolUseID != "toolu_1" {
		t.Fatalf("tool result block = %#v, want tool_use_id=toolu_1", toolResultBlocks[0])
	}
	if content, ok := toolResultBlocks[0].Content.(string); !ok || content != "sunny" {
		t.Fatalf("tool result content = %#v, want %q", toolResultBlocks[0].Content, "sunny")
	}
}

func TestClaudeAdapterChatCompletionStream_EmitsToolCallsAndUsage(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer does not support flushing")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: message_start\n")
		fmt.Fprint(w, "data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"model\":\"claude-sonnet-4-0\",\"usage\":{\"input_tokens\":28,\"output_tokens\":0}}}\n\n")
		fmt.Fprint(w, "event: content_block_start\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"lookup_weather\",\"input\":{}}}\n\n")
		fmt.Fprint(w, "event: content_block_delta\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"city\\\":\\\"shang\"}}\n\n")
		fmt.Fprint(w, "event: content_block_delta\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"hai\\\"}\"}}\n\n")
		fmt.Fprint(w, "event: message_delta\n")
		fmt.Fprint(w, "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"input_tokens\":28,\"output_tokens\":52}}\n\n")
		fmt.Fprint(w, "event: message_stop\n")
		fmt.Fprint(w, "data: {\"type\":\"message_stop\"}\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	a, err := NewClaudeAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewClaudeAdapter() error = %v", err)
	}

	stream, err := a.ChatCompletionStream(context.Background(), &adapter.ChatRequest{
		Model:    "claude-sonnet-4-0",
		Messages: []adapter.Message{{Role: "user", Content: "weather?"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletionStream() error = %v", err)
	}

	var (
		toolName      string
		toolArgs      []string
		finishReason  string
		finalUsage    *adapter.Usage
		finalDoneSeen bool
	)
	for resp := range stream {
		for _, choice := range resp.Choices {
			if choice.FinishReason != "" {
				finishReason = choice.FinishReason
			}
			for _, tc := range choice.Delta.ToolCalls {
				toolName = tc.Function.Name
				if tc.Function.Arguments != "" {
					toolArgs = append(toolArgs, tc.Function.Arguments)
				}
			}
		}
		if resp.Usage != nil {
			finalUsage = resp.Usage
		}
		if resp.Done {
			finalDoneSeen = true
		}
	}

	if toolName != "lookup_weather" {
		t.Fatalf("tool name = %q, want %q", toolName, "lookup_weather")
	}
	if got := strings.Join(toolArgs, ""); got != "{\"city\":\"shanghai\"}" {
		t.Fatalf("tool arguments = %q, want %q", got, "{\"city\":\"shanghai\"}")
	}
	if finishReason != "tool_calls" {
		t.Fatalf("finishReason = %q, want %q", finishReason, "tool_calls")
	}
	if !finalDoneSeen {
		t.Fatal("expected final done chunk")
	}
	if finalUsage == nil || finalUsage.PromptTokens != 28 || finalUsage.CompletionTokens != 52 || finalUsage.TotalTokens != 80 {
		t.Fatalf("usage = %+v, want prompt=28 completion=52 total=80", finalUsage)
	}
}

func TestClaudeAdapterCreateAnthropicMessage_UsesMessagesEndpointAndRawBody(t *testing.T) {
	t.Helper()

	var (
		gotPath    string
		gotAPIKey  string
		gotVersion string
		gotBeta    string
		gotPayload map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		gotBeta = r.Header.Get("anthropic-beta")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id":"msg_1",
			"type":"message",
			"role":"assistant",
			"model":"claude-sonnet-4-0",
			"content":[{"type":"text","text":"ok"}],
			"stop_reason":"end_turn",
			"stop_sequence":null,
			"usage":{"input_tokens":8,"output_tokens":5}
		}`)
	}))
	defer server.Close()

	a, err := NewClaudeAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewClaudeAdapter() error = %v", err)
	}

	resp, err := a.CreateAnthropicMessage(context.Background(), &adapter.AnthropicMessageRequest{
		Model: "claude-sonnet-4-0",
		Body:  json.RawMessage(`{"model":"claude-sonnet-4-0","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`),
		Headers: map[string]string{
			"anthropic-beta": "tools-2026-01-01",
		},
	})
	if err != nil {
		t.Fatalf("CreateAnthropicMessage() error = %v", err)
	}

	if gotPath != "/v1/messages" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/messages")
	}
	if gotAPIKey != "test-key" {
		t.Fatalf("x-api-key = %q, want test-key", gotAPIKey)
	}
	if gotVersion != "2023-06-01" {
		t.Fatalf("anthropic-version = %q, want 2023-06-01", gotVersion)
	}
	if gotBeta != "tools-2026-01-01" {
		t.Fatalf("anthropic-beta = %q, want tools-2026-01-01", gotBeta)
	}
	if gotPayload["model"] != "claude-sonnet-4-0" || gotPayload["max_tokens"] != float64(64) {
		t.Fatalf("payload = %#v, want raw messages body", gotPayload)
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 8 || resp.Usage.CompletionTokens != 5 || resp.Usage.TotalTokens != 13 {
		t.Fatalf("usage = %+v, want prompt=8 completion=5 total=13", resp.Usage)
	}
}

func TestClaudeAdapterCreateAnthropicMessageStream_EmitsNativeMessagesEvents(t *testing.T) {
	t.Helper()

	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		if r.URL.Path != "/v1/messages" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v1/messages")
		}
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer does not support flushing")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: message_start\n")
		fmt.Fprint(w, "data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"model\":\"claude-sonnet-4-0\",\"usage\":{\"input_tokens\":9,\"output_tokens\":0}}}\n\n")
		fmt.Fprint(w, "event: content_block_start\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		fmt.Fprint(w, "event: content_block_delta\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\n")
		fmt.Fprint(w, "event: content_block_stop\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
		fmt.Fprint(w, "event: message_delta\n")
		fmt.Fprint(w, "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":4}}\n\n")
		fmt.Fprint(w, "event: message_stop\n")
		fmt.Fprint(w, "data: {\"type\":\"message_stop\"}\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	a, err := NewClaudeAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewClaudeAdapter() error = %v", err)
	}

	stream, err := a.CreateAnthropicMessageStream(context.Background(), &adapter.AnthropicMessageRequest{
		Model: "claude-sonnet-4-0",
		Body:  json.RawMessage(`{"model":"claude-sonnet-4-0","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`),
	})
	if err != nil {
		t.Fatalf("CreateAnthropicMessageStream() error = %v", err)
	}

	var (
		events   []string
		usage    *adapter.Usage
		doneSeen bool
	)
	for event := range stream {
		if event.Error != nil {
			t.Fatalf("stream event error = %v", event.Error)
		}
		if event.Done {
			doneSeen = true
			usage = event.Usage
			continue
		}
		events = append(events, event.Event)
		if event.Usage != nil {
			usage = event.Usage
		}
	}

	if gotPayload["stream"] != true {
		t.Fatalf("payload.stream = %#v, want true", gotPayload["stream"])
	}
	if !doneSeen {
		t.Fatal("expected final done marker")
	}
	wantEvents := []string{"message_start", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop"}
	if strings.Join(events, ",") != strings.Join(wantEvents, ",") {
		t.Fatalf("events = %#v, want %#v", events, wantEvents)
	}
	if usage == nil || usage.PromptTokens != 9 || usage.CompletionTokens != 4 || usage.TotalTokens != 13 {
		t.Fatalf("usage = %+v, want prompt=9 completion=4 total=13", usage)
	}
}

func TestClaudeAdapterConvertStopReason_MapsCurrentAnthropicValues(t *testing.T) {
	a, err := NewClaudeAdapter(&adapter.AdapterConfig{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("NewClaudeAdapter() error = %v", err)
	}

	cases := map[string]string{
		"end_turn":                      "stop",
		"max_tokens":                    "length",
		"stop_sequence":                 "stop",
		"tool_use":                      "tool_calls",
		"pause_turn":                    "stop",
		"refusal":                       "content_filter",
		"model_context_window_exceeded": "length",
	}

	for input, want := range cases {
		if got := a.convertStopReason(input); got != want {
			t.Fatalf("convertStopReason(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestClaudeAdapterListModels_PaginatesAndUsesDecisionSafeSemantics(t *testing.T) {
	t.Helper()

	type requestMeta struct {
		AfterID       string
		Limit         string
		APIKey        string
		AnthropicBeta string
	}

	requests := make([]requestMeta, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, requestMeta{
			AfterID:       r.URL.Query().Get("after_id"),
			Limit:         r.URL.Query().Get("limit"),
			APIKey:        r.Header.Get("x-api-key"),
			AnthropicBeta: r.Header.Get("anthropic-beta"),
		})

		if r.URL.Path != "/models" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/models")
		}

		page := len(requests)
		switch page {
		case 1:
			fmt.Fprint(w, `{"data":[{"id":"claude-sonnet-4-0","display_name":"Claude Sonnet 4","created_at":"2026-03-01T00:00:00Z","type":"model"}],"has_more":true,"last_id":"claude-sonnet-4-0"}`)
		case 2:
			if got := r.URL.Query().Get("after_id"); got != "claude-sonnet-4-0" {
				t.Fatalf("after_id = %q, want %q", got, "claude-sonnet-4-0")
			}
			fmt.Fprint(w, `{"data":[{"id":"claude-haiku-4-0","display_name":"Claude Haiku 4","created_at":"2026-03-02T00:00:00Z","type":"model"}],"has_more":false,"last_id":"claude-haiku-4-0"}`)
		default:
			t.Fatalf("unexpected page request #%d: %s", page, r.URL.String())
		}
	}))
	defer server.Close()

	a, err := NewClaudeAdapter(&adapter.AdapterConfig{
		APIKey:  "config-key",
		BaseURL: server.URL,
		Headers: map[string]string{
			"anthropic-beta": "messages-2026-03-01",
		},
	})
	if err != nil {
		t.Fatalf("NewClaudeAdapter() error = %v", err)
	}

	models, err := a.ListModels(context.Background(), "runtime-key")
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}

	if len(requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(requests))
	}
	for i, req := range requests {
		if req.Limit == "" {
			t.Fatalf("request[%d].limit is empty, want explicit pagination limit", i)
		}
		if req.APIKey != "runtime-key" {
			t.Fatalf("request[%d].x-api-key = %q, want %q", i, req.APIKey, "runtime-key")
		}
		if req.AnthropicBeta != "messages-2026-03-01" {
			t.Fatalf("request[%d].anthropic-beta = %q, want %q", i, req.AnthropicBeta, "messages-2026-03-01")
		}
	}
	if requests[0].AfterID != "" {
		t.Fatalf("first request after_id = %q, want empty", requests[0].AfterID)
	}
	if requests[1].AfterID != "claude-sonnet-4-0" {
		t.Fatalf("second request after_id = %q, want %q", requests[1].AfterID, "claude-sonnet-4-0")
	}

	if len(models) != 2 {
		t.Fatalf("len(models) = %d, want 2", len(models))
	}
	if models[0].ID != "claude-sonnet-4-0" || models[1].ID != "claude-haiku-4-0" {
		t.Fatalf("models ids = [%q, %q], want [claude-sonnet-4-0, claude-haiku-4-0]", models[0].ID, models[1].ID)
	}
	for i, model := range models {
		if model.Type == "model" {
			t.Fatalf("models[%d].Type = %q, should not reuse upstream object type", i, model.Type)
		}
		if model.Type != "chat" {
			t.Fatalf("models[%d].Type = %q, want %q", i, model.Type, "chat")
		}
		if !containsStringValue(model.Capabilities, "chat") {
			t.Fatalf("models[%d].Capabilities = %#v, want to contain %q", i, model.Capabilities, "chat")
		}
		if !containsStringValue(model.Capabilities, "stream") {
			t.Fatalf("models[%d].Capabilities = %#v, want to contain %q", i, model.Capabilities, "stream")
		}
	}
}

func TestClaudeAdapterListModels_PreservesExplicitBaseQuery(t *testing.T) {
	t.Helper()

	var gotQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		fmt.Fprint(w, `{"data":[],"has_more":false,"last_id":""}`)
	}))
	defer server.Close()

	a, err := NewClaudeAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "?foo=bar",
	})
	if err != nil {
		t.Fatalf("NewClaudeAdapter() error = %v", err)
	}

	if _, err := a.ListModels(context.Background(), "test-key"); err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}

	if got := gotQuery.Get("foo"); got != "bar" {
		t.Fatalf("foo query = %q, want %q", got, "bar")
	}
	if got := gotQuery.Get("limit"); got == "" {
		t.Fatal("limit query is empty, want explicit pagination limit")
	}
}
