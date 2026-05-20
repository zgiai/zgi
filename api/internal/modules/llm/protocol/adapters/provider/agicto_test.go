package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestAgictoAdapterCreateResponseRaw_UsesVersionedResponsesEndpoint(t *testing.T) {
	var (
		gotPath    string
		gotAuth    string
		gotPayload map[string]any
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"resp_1","object":"response","status":"completed","usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`)
	}))
	defer server.Close()

	a, err := NewAgictoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewAgictoAdapter() error = %v", err)
	}

	resp, err := a.CreateResponseRaw(context.Background(), &adapter.RawResponseRequest{
		Model: "gpt-4.1-mini",
		Body:  json.RawMessage(`{"model":"gpt-4.1-mini","input":"hello"}`),
	})
	if err != nil {
		t.Fatalf("CreateResponseRaw() error = %v", err)
	}

	if gotPath != "/v1/responses" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/responses")
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotPayload["input"] != "hello" {
		t.Fatalf("payload = %#v, want raw responses body", gotPayload)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 3 {
		t.Fatalf("usage = %+v, want total=3", resp.Usage)
	}
}

func TestAgictoAdapterCreateAnthropicMessage_UsesNativeMessagesEndpoint(t *testing.T) {
	var (
		gotPath    string
		gotAPIKey  string
		gotVersion string
		gotPayload map[string]any
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":4,"output_tokens":3}}`)
	}))
	defer server.Close()

	a, err := NewAgictoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("NewAgictoAdapter() error = %v", err)
	}

	resp, err := a.CreateAnthropicMessage(context.Background(), &adapter.AnthropicMessageRequest{
		Model: "claude-sonnet-4-0",
		Body:  json.RawMessage(`{"model":"claude-sonnet-4-0","max_tokens":64,"messages":[{"role":"user","content":"hello"}],"tools":[{"name":"lookup","input_schema":{"type":"object"}}]}`),
	})
	if err != nil {
		t.Fatalf("CreateAnthropicMessage() error = %v", err)
	}

	if gotPath != "/v1/messages" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/messages")
	}
	if gotAPIKey != "test-key" {
		t.Fatalf("x-api-key = %q, want %q", gotAPIKey, "test-key")
	}
	if gotVersion != "2023-06-01" {
		t.Fatalf("anthropic-version = %q, want %q", gotVersion, "2023-06-01")
	}
	if gotPayload["tools"] == nil {
		t.Fatalf("payload missing tools: %#v", gotPayload)
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 4 || resp.Usage.CompletionTokens != 3 {
		t.Fatalf("usage = %+v, want prompt=4 completion=3", resp.Usage)
	}
}

func TestAgictoAdapterCreateAnthropicMessageStream_PreservesNativeEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v1/messages")
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer does not support flushing")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: message_start\n")
		fmt.Fprint(w, "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":2}}}\n\n")
		fmt.Fprint(w, "event: content_block_delta\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\n")
		fmt.Fprint(w, "event: message_stop\n")
		fmt.Fprint(w, "data: {\"type\":\"message_stop\"}\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	a, err := NewAgictoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewAgictoAdapter() error = %v", err)
	}
	stream, err := a.CreateAnthropicMessageStream(context.Background(), &adapter.AnthropicMessageRequest{
		Model: "claude-sonnet-4-0",
		Body:  json.RawMessage(`{"model":"claude-sonnet-4-0","stream":true,"max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`),
	})
	if err != nil {
		t.Fatalf("CreateAnthropicMessageStream() error = %v", err)
	}

	var events []string
	for event := range stream {
		if event.Error != nil {
			t.Fatalf("stream event error = %v", event.Error)
		}
		if event.Done {
			continue
		}
		events = append(events, event.Event)
		if strings.Contains(string(event.Data), `"choices"`) {
			t.Fatalf("stream data contains chat choices: %s", string(event.Data))
		}
	}

	want := []string{"message_start", "content_block_delta", "message_stop"}
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("events = %v, want %v", events, want)
	}
}
