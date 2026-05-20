package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

func TestSiliconFlowAdapterResponsesAreUnsupported(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	a, err := NewSiliconFlowAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewSiliconFlowAdapter() error = %v", err)
	}

	if _, err := a.CreateResponse(context.Background(), &adapter.CreateResponseRequest{Model: "deepseek-v3.1"}); !errors.Is(err, adapter.ErrCapabilityUnsupported) {
		t.Fatalf("CreateResponse() error = %v, want ErrCapabilityUnsupported", err)
	}
	if _, err := a.CreateResponseRaw(context.Background(), &adapter.RawResponseRequest{
		Model: "deepseek-v3.1",
		Body:  json.RawMessage(`{"model":"deepseek-v3.1","input":"hello"}`),
	}); !errors.Is(err, adapter.ErrCapabilityUnsupported) {
		t.Fatalf("CreateResponseRaw() error = %v, want ErrCapabilityUnsupported", err)
	}
	if _, err := a.CreateResponseStream(context.Background(), &adapter.RawResponseRequest{
		Model: "deepseek-v3.1",
		Body:  json.RawMessage(`{"model":"deepseek-v3.1","input":"hello","stream":true}`),
	}); !errors.Is(err, adapter.ErrCapabilityUnsupported) {
		t.Fatalf("CreateResponseStream() error = %v, want ErrCapabilityUnsupported", err)
	}
}

func TestSiliconFlowAdapterCreateAnthropicMessage_UsesV1MessagesEndpoint(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		gotPath = r.URL.Path
		if r.Header.Get("x-api-key") != "test-key" {
			t.Fatalf("x-api-key = %q, want %q", r.Header.Get("x-api-key"), "test-key")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":3,"output_tokens":1}}`)
	}))
	defer server.Close()

	a, err := NewSiliconFlowAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewSiliconFlowAdapter() error = %v", err)
	}

	resp, err := a.CreateAnthropicMessage(context.Background(), &adapter.AnthropicMessageRequest{
		Model: "deepseek-v3.1",
		Body:  json.RawMessage(`{"model":"deepseek-v3.1","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`),
	})
	if err != nil {
		t.Fatalf("CreateAnthropicMessage() error = %v", err)
	}
	if gotPath != "/v1/messages" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/messages")
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 3 || resp.Usage.CompletionTokens != 1 {
		t.Fatalf("usage = %+v, want prompt=3 completion=1", resp.Usage)
	}
}
