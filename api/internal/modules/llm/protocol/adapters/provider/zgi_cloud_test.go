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

func TestZGICloudAdapterChatCompletion_ForwardsToConsoleInternal(t *testing.T) {
	t.Helper()

	var (
		gotPath string
		gotSig  string
		gotAuth string
		gotOrg  string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotSig = r.Header.Get("X-Test-Signature")
		gotAuth = r.Header.Get("Authorization")
		gotOrg = r.Header.Get("OpenAI-Organization")

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set(headerSettlementID, "deduction-1")
		w.Header().Set(headerOfficialPoints, "7")
		w.Header().Set(headerRemainingBalance, "93")
		w.Header().Set(headerSettlementStatus, "settled")
		fmt.Fprint(w, `{
			"id":"chatcmpl-zgi-cloud-1",
			"object":"chat.completion",
			"created":1732083164,
			"model":"gpt-4o-mini",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}
		}`)
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL:      server.URL + "/v1/internal",
		Organization: "should-not-forward",
		AuthHook: func(req *http.Request) {
			req.Header.Set("X-Test-Signature", "signed")
		},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	resp, err := a.ChatCompletion(context.Background(), &adapter.ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []adapter.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	if gotPath != "/v1/internal/chat/completions" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/internal/chat/completions")
	}
	if gotSig != "signed" {
		t.Fatalf("X-Test-Signature = %q, want %q", gotSig, "signed")
	}
	if gotAuth != "" {
		t.Fatalf("Authorization = %q, want empty for HMAC-only official transport", gotAuth)
	}
	if gotOrg != "" {
		t.Fatalf("OpenAI-Organization = %q, want empty for console forward transport", gotOrg)
	}
	if resp.Model != "gpt-4o-mini" {
		t.Fatalf("response model = %q, want %q", resp.Model, "gpt-4o-mini")
	}
	if resp.Settlement == nil || resp.Settlement.SettlementID != "deduction-1" || resp.Settlement.OfficialPoints != 7 {
		t.Fatalf("settlement = %+v, want deduction-1/7", resp.Settlement)
	}
}

func TestZGICloudAdapterChatCompletionStream_ConsumesSettlementEvent(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/internal/chat/completions" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v1/internal/chat/completions")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\n")
		fmt.Fprint(w, "event: zgi.settlement\n")
		fmt.Fprint(w, "data: {\"settlement_id\":\"deduction-stream\",\"official_points\":9,\"remaining_balance\":91,\"status\":\"settled\"}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL: server.URL + "/v1/internal",
		AuthHook: func(req *http.Request) {
			req.Header.Set("X-Test-Signature", "signed")
		},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	stream, err := a.ChatCompletionStream(context.Background(), &adapter.ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []adapter.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletionStream() error = %v", err)
	}

	var (
		chunkCount int
		done       adapter.StreamResponse
	)
	for event := range stream {
		if event.Error != nil {
			t.Fatalf("stream error = %v", event.Error)
		}
		if event.Done {
			done = event
			continue
		}
		chunkCount++
	}

	if chunkCount != 2 {
		t.Fatalf("chunk count = %d, want 2", chunkCount)
	}
	if done.Settlement == nil || done.Settlement.SettlementID != "deduction-stream" || done.Settlement.OfficialPoints != 9 {
		t.Fatalf("done settlement = %+v, want deduction-stream/9", done.Settlement)
	}
	if done.Usage == nil || done.Usage.TotalTokens != 5 {
		t.Fatalf("done usage = %+v, want total 5", done.Usage)
	}
}

func TestZGICloudAdapterChatCompletionStream_SettlementErrorReturnsStreamError(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\n")
		fmt.Fprint(w, "event: zgi.settlement_error\n")
		fmt.Fprint(w, "data: {\"code\":\"billing_settlement_failed\",\"message\":\"official settlement failed\",\"status\":\"failed\"}\n\n")
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL: server.URL + "/v1/internal",
		AuthHook: func(req *http.Request) {
			req.Header.Set("X-Test-Signature", "signed")
		},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	stream, err := a.ChatCompletionStream(context.Background(), &adapter.ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []adapter.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletionStream() error = %v", err)
	}

	var (
		chunkCount int
		done       adapter.StreamResponse
	)
	for event := range stream {
		if event.Done {
			done = event
			continue
		}
		chunkCount++
	}

	if chunkCount != 2 {
		t.Fatalf("chunk count = %d, want 2", chunkCount)
	}
	if done.Error == nil || done.Error.Error() != "console proxy settlement failed: official settlement failed" {
		t.Fatalf("done error = %v, want explicit settlement failure", done.Error)
	}
	if done.Usage == nil || done.Usage.TotalTokens != 5 {
		t.Fatalf("done usage = %+v, want total 5", done.Usage)
	}
}

func TestZGICloudAdapterCreateResponseRaw_ForwardsToConsoleInternalResponses(t *testing.T) {
	t.Helper()

	var (
		gotPath    string
		gotSig     string
		gotAuth    string
		gotPayload map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		gotPath = r.URL.Path
		gotSig = r.Header.Get("X-Test-Signature")
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id":"resp_zgi_cloud_1",
			"object":"response",
			"model":"gpt-4.1-mini",
			"output":[],
			"usage":{"input_tokens":6,"output_tokens":4,"total_tokens":10}
		}`)
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL: server.URL + "/v1/internal",
		AuthHook: func(req *http.Request) {
			req.Header.Set("X-Test-Signature", "signed")
		},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	resp, err := a.CreateResponseRaw(context.Background(), &adapter.RawResponseRequest{
		Model: "gpt-4.1-mini",
		Body:  json.RawMessage(`{"model":"gpt-4.1-mini","input":"hello"}`),
	})
	if err != nil {
		t.Fatalf("CreateResponseRaw() error = %v", err)
	}

	if gotPath != "/v1/internal/responses" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/internal/responses")
	}
	if gotSig != "signed" {
		t.Fatalf("X-Test-Signature = %q, want signed", gotSig)
	}
	if gotAuth != "" {
		t.Fatalf("Authorization = %q, want empty for HMAC-only official transport", gotAuth)
	}
	if gotPayload["model"] != "gpt-4.1-mini" || gotPayload["input"] != "hello" {
		t.Fatalf("payload = %#v, want raw responses body", gotPayload)
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 6 || resp.Usage.CompletionTokens != 4 || resp.Usage.TotalTokens != 10 {
		t.Fatalf("usage = %+v, want prompt=6 completion=4 total=10", resp.Usage)
	}
}

func TestZGICloudAdapterCreateResponseStream_ConsumesSettlementEvent(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/internal/responses" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v1/internal/responses")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.created\n")
		fmt.Fprint(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\"}}\n\n")
		fmt.Fprint(w, "event: response.completed\n")
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"usage\":{\"input_tokens\":4,\"output_tokens\":3,\"total_tokens\":7}}}\n\n")
		fmt.Fprint(w, "event: zgi.settlement\n")
		fmt.Fprint(w, "data: {\"settlement_id\":\"deduction-response\",\"official_points\":11,\"remaining_balance\":89,\"status\":\"settled\"}\n\n")
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL: server.URL + "/v1/internal",
		AuthHook: func(req *http.Request) {
			req.Header.Set("X-Test-Signature", "signed")
		},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	stream, err := a.CreateResponseStream(context.Background(), &adapter.RawResponseRequest{
		Model: "gpt-4.1-mini",
		Body:  json.RawMessage(`{"model":"gpt-4.1-mini","input":"hello","stream":true}`),
	})
	if err != nil {
		t.Fatalf("CreateResponseStream() error = %v", err)
	}

	var (
		events []string
		done   adapter.RawStreamEvent
	)
	for event := range stream {
		if event.Error != nil {
			t.Fatalf("stream error = %v", event.Error)
		}
		if event.Done {
			done = event
			continue
		}
		events = append(events, event.Event)
	}

	if len(events) != 2 || events[0] != "response.created" || events[1] != "response.completed" {
		t.Fatalf("events = %#v, want native response events only", events)
	}
	if done.Settlement == nil || done.Settlement.SettlementID != "deduction-response" || done.Settlement.OfficialPoints != 11 {
		t.Fatalf("done settlement = %+v, want deduction-response/11", done.Settlement)
	}
	if done.Usage == nil || done.Usage.TotalTokens != 7 {
		t.Fatalf("done usage = %+v, want total 7", done.Usage)
	}
}

func TestZGICloudAdapterCreateResponseStream_SettlementErrorReturnsStreamError(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.completed\n")
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"usage\":{\"input_tokens\":4,\"output_tokens\":3,\"total_tokens\":7}}}\n\n")
		fmt.Fprint(w, "event: zgi.settlement_error\n")
		fmt.Fprint(w, "data: {\"code\":\"billing_settlement_failed\",\"message\":\"official settlement failed\",\"status\":\"failed\"}\n\n")
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL: server.URL + "/v1/internal",
		AuthHook: func(req *http.Request) {
			req.Header.Set("X-Test-Signature", "signed")
		},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	stream, err := a.CreateResponseStream(context.Background(), &adapter.RawResponseRequest{
		Model: "gpt-4.1-mini",
		Body:  json.RawMessage(`{"model":"gpt-4.1-mini","input":"hello","stream":true}`),
	})
	if err != nil {
		t.Fatalf("CreateResponseStream() error = %v", err)
	}

	var (
		events []string
		done   adapter.RawStreamEvent
	)
	for event := range stream {
		if event.Done {
			done = event
			continue
		}
		events = append(events, event.Event)
	}

	if len(events) != 1 || events[0] != "response.completed" {
		t.Fatalf("events = %#v, want response.completed only", events)
	}
	if done.Error == nil || done.Error.Error() != "console proxy settlement failed: official settlement failed" {
		t.Fatalf("done error = %v, want explicit settlement failure", done.Error)
	}
	if done.Usage == nil || done.Usage.TotalTokens != 7 {
		t.Fatalf("done usage = %+v, want total 7", done.Usage)
	}
}

func TestZGICloudAdapterCreateAnthropicMessage_ForwardsToConsoleInternalAnthropicMessages(t *testing.T) {
	t.Helper()

	var (
		gotPath    string
		gotSig     string
		gotVersion string
		gotPayload map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		gotPath = r.URL.Path
		gotSig = r.Header.Get("X-Test-Signature")
		gotVersion = r.Header.Get("anthropic-version")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id":"msg_zgi_cloud_1",
			"type":"message",
			"role":"assistant",
			"model":"claude-sonnet-4-0",
			"content":[{"type":"text","text":"ok"}],
			"usage":{"input_tokens":7,"output_tokens":3}
		}`)
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL: server.URL + "/v1/internal",
		AuthHook: func(req *http.Request) {
			req.Header.Set("X-Test-Signature", "signed")
		},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	resp, err := a.CreateAnthropicMessage(context.Background(), &adapter.AnthropicMessageRequest{
		Model: "claude-sonnet-4-0",
		Body:  json.RawMessage(`{"model":"claude-sonnet-4-0","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`),
		Headers: map[string]string{
			"anthropic-version": "2023-06-01",
		},
	})
	if err != nil {
		t.Fatalf("CreateAnthropicMessage() error = %v", err)
	}

	if gotPath != "/v1/internal/anthropic/v1/messages" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/internal/anthropic/v1/messages")
	}
	if gotSig != "signed" {
		t.Fatalf("X-Test-Signature = %q, want signed", gotSig)
	}
	if gotVersion != "2023-06-01" {
		t.Fatalf("anthropic-version = %q, want 2023-06-01", gotVersion)
	}
	if gotPayload["model"] != "claude-sonnet-4-0" || gotPayload["max_tokens"] != float64(64) {
		t.Fatalf("payload = %#v, want raw messages body", gotPayload)
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 7 || resp.Usage.CompletionTokens != 3 || resp.Usage.TotalTokens != 10 {
		t.Fatalf("usage = %+v, want prompt=7 completion=3 total=10", resp.Usage)
	}
}

func TestZGICloudAdapterCreateAnthropicMessageStream_PreservesNativeEvents(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/internal/anthropic/v1/messages" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v1/internal/anthropic/v1/messages")
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer does not support flushing")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: message_start\n")
		fmt.Fprint(w, "data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"usage\":{\"input_tokens\":5,\"output_tokens\":0}}}\n\n")
		fmt.Fprint(w, "event: content_block_delta\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\n")
		fmt.Fprint(w, "event: message_delta\n")
		fmt.Fprint(w, "data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":2}}\n\n")
		fmt.Fprint(w, "event: message_stop\n")
		fmt.Fprint(w, "data: {\"type\":\"message_stop\"}\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL: server.URL + "/v1/internal",
		AuthHook: func(req *http.Request) {
			req.Header.Set("X-Test-Signature", "signed")
		},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	stream, err := a.CreateAnthropicMessageStream(context.Background(), &adapter.AnthropicMessageRequest{
		Model: "claude-sonnet-4-0",
		Body:  json.RawMessage(`{"model":"claude-sonnet-4-0","stream":true,"max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`),
	})
	if err != nil {
		t.Fatalf("CreateAnthropicMessageStream() error = %v", err)
	}

	var (
		events []string
		usage  *adapter.Usage
	)
	for event := range stream {
		if event.Error != nil {
			t.Fatalf("stream event error = %v", event.Error)
		}
		if event.Done {
			usage = event.Usage
			continue
		}
		events = append(events, event.Event)
		if event.Usage != nil {
			usage = event.Usage
		}
	}

	if len(events) != 4 || events[0] != "message_start" || events[1] != "content_block_delta" || events[2] != "message_delta" || events[3] != "message_stop" {
		t.Fatalf("events = %#v, want native Anthropic events", events)
	}
	if usage == nil || usage.PromptTokens != 5 || usage.CompletionTokens != 2 || usage.TotalTokens != 7 {
		t.Fatalf("usage = %+v, want prompt=5 completion=2 total=7", usage)
	}
}

func TestZGICloudAdapterCreateImage_ForwardsToConsoleInternal(t *testing.T) {
	t.Helper()

	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"created":1732083164,
			"data":[{"url":"https://cdn.example.com/generated.png"}]
		}`)
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL: server.URL + "/v1/internal",
		AuthHook: func(req *http.Request) {
			req.Header.Set("X-Test-Signature", "signed")
		},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	resp, err := a.CreateImage(context.Background(), &adapter.ImageRequest{
		Model:  "qwen-image-2.0",
		Prompt: "a flower",
	})
	if err != nil {
		t.Fatalf("CreateImage() error = %v", err)
	}

	if gotPath != "/v1/internal/images/generations" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/internal/images/generations")
	}
	if len(resp.Data) != 1 || resp.Data[0].URL != "https://cdn.example.com/generated.png" {
		t.Fatalf("response data = %#v, want generated image url", resp.Data)
	}
}

func TestZGICloudAdapterCreateEmbeddings_ForwardsToConsoleInternal(t *testing.T) {
	t.Helper()

	var (
		gotPath    string
		gotSig     string
		gotPayload map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		gotPath = r.URL.Path
		gotSig = r.Header.Get("X-Test-Signature")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"object":"list",
			"model":"text-embedding-3-small",
			"data":[{"object":"embedding","index":0,"embedding":[0.1,0.2,0.3]}],
			"usage":{"prompt_tokens":7,"total_tokens":7}
		}`)
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL: server.URL + "/v1/internal",
		AuthHook: func(req *http.Request) {
			req.Header.Set("X-Test-Signature", "signed")
		},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	resp, err := a.CreateEmbeddings(context.Background(), &adapter.EmbeddingsRequest{
		Model:      "text-embedding-3-small",
		Input:      "hello",
		Dimensions: 1024,
	})
	if err != nil {
		t.Fatalf("CreateEmbeddings() error = %v", err)
	}

	if gotPath != "/v1/internal/embeddings" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/internal/embeddings")
	}
	if gotSig != "signed" {
		t.Fatalf("X-Test-Signature = %q, want %q", gotSig, "signed")
	}
	if got := gotPayload["dimensions"]; got != float64(1024) {
		t.Fatalf("payload.dimensions = %#v, want %d", got, 1024)
	}
	if resp.Model != "text-embedding-3-small" {
		t.Fatalf("response model = %q, want %q", resp.Model, "text-embedding-3-small")
	}
}

func TestZGICloudAdapterRerank_ForwardsToConsoleInternal(t *testing.T) {
	t.Helper()

	var (
		gotPath    string
		gotSig     string
		gotPayload map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		gotPath = r.URL.Path
		gotSig = r.Header.Get("X-Test-Signature")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id":"rerank-zgi-cloud-1",
			"results":[
				{"index":1,"relevance_score":0.97,"document":{"text":"second"}},
				{"index":0,"relevance_score":0.66,"document":{"text":"first"}}
			]
		}`)
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL: server.URL + "/v1/internal",
		AuthHook: func(req *http.Request) {
			req.Header.Set("X-Test-Signature", "signed")
		},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	returnDocuments := true
	resp, err := a.Rerank(context.Background(), &adapter.RerankRequest{
		Model:           "rerank-v1",
		Query:           "hello",
		Documents:       []string{"first", "second"},
		TopN:            intPtrZGICloudTest(1),
		ReturnDocuments: &returnDocuments,
	})
	if err != nil {
		t.Fatalf("Rerank() error = %v", err)
	}

	if gotPath != "/v1/internal/rerank" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/internal/rerank")
	}
	if gotSig != "signed" {
		t.Fatalf("X-Test-Signature = %q, want %q", gotSig, "signed")
	}
	if got := gotPayload["top_n"]; got != float64(1) {
		t.Fatalf("payload.top_n = %#v, want %d", got, 1)
	}
	if got := gotPayload["return_documents"]; got != true {
		t.Fatalf("payload.return_documents = %#v, want true", got)
	}
	if resp.ID != "rerank-zgi-cloud-1" {
		t.Fatalf("response ID = %q, want %q", resp.ID, "rerank-zgi-cloud-1")
	}
	if len(resp.Results) != 2 {
		t.Fatalf("results len = %d, want 2", len(resp.Results))
	}
}

func TestZGICloudAdapterRejectsStillUnsupportedCapabilities(t *testing.T) {
	t.Helper()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL: "http://console.internal/v1/internal",
		AuthHook: func(req *http.Request) {
			req.Header.Set("X-Test-Signature", "signed")
		},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	_, err = a.CreateResponse(context.Background(), &adapter.CreateResponseRequest{
		Model: "gpt-4.1-mini",
		Input: "hello",
	})
	if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
		t.Fatalf("CreateResponse() error = %v, want %v", err, adapter.ErrCapabilityUnsupported)
	}
}

func intPtrZGICloudTest(v int) *int {
	return &v
}
