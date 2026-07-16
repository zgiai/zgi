package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestHandleOpenAICompatibleErrorMapsPlatformChannelUnavailable(t *testing.T) {
	err := handleOpenAICompatibleError(
		http.StatusBadGateway,
		[]byte(`{"error":{"message":"Platform model service is temporarily unavailable","type":"server_error","code":"platform_channel_unavailable"}}`),
	)

	if !errors.Is(err, adapter.ErrPlatformChannelUnavailable) {
		t.Fatalf("handleOpenAICompatibleError() error = %v, want ErrPlatformChannelUnavailable", err)
	}
}

func TestOpenAIAdapterChatCompletionStreamParsesPlatformChannelError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = fmt.Fprint(w, `{"error":{"message":"Platform model service is temporarily unavailable","type":"server_error","code":"platform_channel_unavailable"}}`)
	}))
	defer server.Close()

	a, err := NewOpenAIAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAIAdapter() error = %v", err)
	}

	_, err = a.ChatCompletionStream(context.Background(), &adapter.ChatRequest{
		Model: "kimi-k2.6",
		Messages: []adapter.Message{
			{Role: "user", Content: "hello"},
		},
	})
	if !errors.Is(err, adapter.ErrPlatformChannelUnavailable) {
		t.Fatalf("ChatCompletionStream() error = %v, want ErrPlatformChannelUnavailable", err)
	}
}

func TestOpenAIAdapterCreateResponseRaw_UsesResponsesEndpointAndRawBody(t *testing.T) {
	t.Helper()

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
		fmt.Fprint(w, `{
			"id":"resp_1",
			"object":"response",
			"created_at":1732083164,
			"status":"completed",
			"model":"gpt-4.1-mini",
			"output":[],
			"usage":{"input_tokens":4,"output_tokens":6,"total_tokens":10}
		}`)
	}))
	defer server.Close()

	a, err := NewOpenAIAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAIAdapter() error = %v", err)
	}

	resp, err := a.CreateResponseRaw(context.Background(), &adapter.RawResponseRequest{
		Model: "gpt-4.1-mini",
		Body:  json.RawMessage(`{"model":"gpt-4.1-mini","input":"hello","tools":[{"type":"web_search_preview"}]}`),
	})
	if err != nil {
		t.Fatalf("CreateResponseRaw() error = %v", err)
	}

	if gotPath != "/v1/responses" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/responses")
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want Bearer test-key", gotAuth)
	}
	if gotPayload["model"] != "gpt-4.1-mini" || gotPayload["input"] != "hello" {
		t.Fatalf("payload = %#v, want raw responses body", gotPayload)
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 4 || resp.Usage.CompletionTokens != 6 || resp.Usage.TotalTokens != 10 {
		t.Fatalf("usage = %+v, want prompt=4 completion=6 total=10", resp.Usage)
	}
}

func TestOpenAIAdapterCreateResponseStream_EmitsNativeResponsesEvents(t *testing.T) {
	t.Helper()

	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v1/responses")
		}
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer does not support flushing")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.created\n")
		fmt.Fprint(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"model\":\"gpt-4.1-mini\"}}\n\n")
		fmt.Fprint(w, "event: response.output_text.delta\n")
		fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n")
		fmt.Fprint(w, "event: response.completed\n")
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"usage\":{\"input_tokens\":3,\"output_tokens\":2,\"total_tokens\":5}}}\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	a, err := NewOpenAIAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAIAdapter() error = %v", err)
	}

	stream, err := a.CreateResponseStream(context.Background(), &adapter.RawResponseRequest{
		Model: "gpt-4.1-mini",
		Body:  json.RawMessage(`{"model":"gpt-4.1-mini","input":"hello"}`),
	})
	if err != nil {
		t.Fatalf("CreateResponseStream() error = %v", err)
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
	if len(events) != 3 || events[0] != "response.created" || events[1] != "response.output_text.delta" || events[2] != "response.completed" {
		t.Fatalf("events = %#v, want native responses events", events)
	}
	if usage == nil || usage.PromptTokens != 3 || usage.CompletionTokens != 2 || usage.TotalTokens != 5 {
		t.Fatalf("usage = %+v, want prompt=3 completion=2 total=5", usage)
	}
}

func TestShouldTreatOpenAIListModelsAsCapabilityUnsupported(t *testing.T) {
	t.Helper()

	cases := []struct {
		name       string
		statusCode int
		body       string
		want       bool
	}{
		{
			name:       "404",
			statusCode: 404,
			body:       `{"error":{"message":"models endpoint is not implemented","code":"not_found"}}`,
			want:       true,
		},
		{
			name:       "405",
			statusCode: 405,
			body:       `{"error":{"message":"method not allowed","code":"method_not_allowed"}}`,
			want:       true,
		},
		{
			name:       "501",
			statusCode: 501,
			body:       `{"error":{"message":"not implemented","code":"not_implemented"}}`,
			want:       true,
		},
		{
			name:       "401",
			statusCode: 401,
			body:       `{"error":{"message":"bad key","code":"invalid_api_key"}}`,
			want:       false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldTreatOpenAIListModelsAsCapabilityUnsupported(tc.statusCode, []byte(tc.body))
			if got != tc.want {
				t.Fatalf("shouldTreatOpenAIListModelsAsCapabilityUnsupported(%d, %q) = %v, want %v", tc.statusCode, tc.body, got, tc.want)
			}
		})
	}
}

func TestNewAdapter_OpenAICompatibleKeyResolvesToOpenAIAdapter(t *testing.T) {
	t.Helper()

	instance, err := adapter.NewAdapter(&adapter.AdapterConfig{
		ProviderName: "openai-compatible",
		APIKey:       "test-key",
		BaseURL:      "https://proxy.example.com/v1",
	})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	if _, ok := instance.(*OpenAIAdapter); !ok {
		t.Fatalf("instance type = %T, want *OpenAIAdapter", instance)
	}
}
