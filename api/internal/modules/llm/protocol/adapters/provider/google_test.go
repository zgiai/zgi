package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestGoogleAdapterChatCompletion_UsesGeminiGenerateContent(t *testing.T) {
	t.Helper()

	var (
		mu         sync.Mutex
		gotAPIKey  string
		gotPath    string
		gotPayload map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		mu.Lock()
		gotAPIKey = r.Header.Get("x-goog-api-key")
		gotPath = r.URL.Path
		gotPayload = payload
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"responseId":"resp-1",
			"modelVersion":"gemini-2.5-flash",
			"candidates":[
				{
					"index":0,
					"content":{"role":"model","parts":[{"text":"世界"}]},
					"finishReason":"STOP"
				}
			],
			"usageMetadata":{
				"promptTokenCount":11,
				"candidatesTokenCount":7,
				"totalTokenCount":18
			}
		}`)
	}))
	defer server.Close()

	a, err := NewGoogleAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1beta",
	})
	if err != nil {
		t.Fatalf("NewGoogleAdapter() error = %v", err)
	}

	resp, err := a.ChatCompletion(context.Background(), &adapter.ChatRequest{
		Model: "gemini-2.5-flash",
		Messages: []adapter.Message{
			{Role: "system", Content: "你是助手"},
			{Role: "user", Content: "你好"},
			{Role: "assistant", Content: "你好，我在"},
			{Role: "user", Content: "继续"},
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if gotAPIKey != "test-key" {
		t.Fatalf("x-goog-api-key = %q, want %q", gotAPIKey, "test-key")
	}
	if gotPath != "/v1beta/models/gemini-2.5-flash:generateContent" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1beta/models/gemini-2.5-flash:generateContent")
	}

	sys, ok := gotPayload["systemInstruction"].(map[string]any)
	if !ok {
		t.Fatalf("systemInstruction missing from payload: %#v", gotPayload)
	}
	sysParts, ok := sys["parts"].([]any)
	if !ok || len(sysParts) != 1 {
		t.Fatalf("systemInstruction.parts = %#v, want one text part", sys["parts"])
	}
	if got := sysParts[0].(map[string]any)["text"]; got != "你是助手" {
		t.Fatalf("systemInstruction.parts[0].text = %#v, want %q", got, "你是助手")
	}

	contents, ok := gotPayload["contents"].([]any)
	if !ok || len(contents) != 3 {
		t.Fatalf("contents = %#v, want 3 conversation items", gotPayload["contents"])
	}
	if got := contents[0].(map[string]any)["role"]; got != "user" {
		t.Fatalf("contents[0].role = %#v, want %q", got, "user")
	}
	if got := contents[1].(map[string]any)["role"]; got != "model" {
		t.Fatalf("contents[1].role = %#v, want %q", got, "model")
	}
	if got := contents[2].(map[string]any)["role"]; got != "user" {
		t.Fatalf("contents[2].role = %#v, want %q", got, "user")
	}

	if resp.ID != "resp-1" {
		t.Fatalf("response ID = %q, want %q", resp.ID, "resp-1")
	}
	if resp.Model != "gemini-2.5-flash" {
		t.Fatalf("response model = %q, want %q", resp.Model, "gemini-2.5-flash")
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("choices len = %d, want 1", len(resp.Choices))
	}
	if got, _ := resp.Choices[0].Message.Content.(string); got != "世界" {
		t.Fatalf("message content = %q, want %q", got, "世界")
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Fatalf("finish_reason = %q, want %q", resp.Choices[0].FinishReason, "stop")
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 11 || resp.Usage.CompletionTokens != 7 || resp.Usage.TotalTokens != 18 {
		t.Fatalf("usage = %+v, want prompt=11 completion=7 total=18", resp.Usage)
	}
}

func TestGoogleAdapterChatCompletionStream_UsesGeminiStreamGenerateContent(t *testing.T) {
	t.Helper()

	var (
		mu        sync.Mutex
		gotAPIKey string
		gotAccept string
		gotPath   string
		gotQuery  string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotAPIKey = r.Header.Get("x-goog-api-key")
		gotAccept = r.Header.Get("Accept")
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		mu.Unlock()

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer does not support flushing")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"candidates\":[{\"index\":0,\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"你\"}]}}]}\n\n")
		fmt.Fprint(w, "data: {\"candidates\":[{\"index\":0,\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"好\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":28,\"candidatesTokenCount\":52,\"totalTokenCount\":80}}\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	a, err := NewGoogleAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1beta",
	})
	if err != nil {
		t.Fatalf("NewGoogleAdapter() error = %v", err)
	}

	stream, err := a.ChatCompletionStream(context.Background(), &adapter.ChatRequest{
		Model:    "gemini-2.5-flash",
		Messages: []adapter.Message{{Role: "user", Content: "你好"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletionStream() error = %v", err)
	}

	var (
		parts      []string
		finish     string
		finalUsage *adapter.Usage
		doneSeen   bool
	)
	for resp := range stream {
		for _, choice := range resp.Choices {
			if text, ok := choice.Delta.Content.(string); ok && text != "" {
				parts = append(parts, text)
			}
			if choice.FinishReason != "" {
				finish = choice.FinishReason
			}
		}
		if resp.Usage != nil {
			finalUsage = resp.Usage
		}
		if resp.Done {
			doneSeen = true
		}
	}

	mu.Lock()
	defer mu.Unlock()

	if gotAPIKey != "test-key" {
		t.Fatalf("x-goog-api-key = %q, want %q", gotAPIKey, "test-key")
	}
	if !strings.Contains(gotAccept, "text/event-stream") {
		t.Fatalf("Accept = %q, want to contain %q", gotAccept, "text/event-stream")
	}
	if gotPath != "/v1beta/models/gemini-2.5-flash:streamGenerateContent" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1beta/models/gemini-2.5-flash:streamGenerateContent")
	}
	if gotQuery != "alt=sse" {
		t.Fatalf("query = %q, want %q", gotQuery, "alt=sse")
	}
	if got := strings.Join(parts, ""); got != "你好" {
		t.Fatalf("streamed text = %q, want %q", got, "你好")
	}
	if finish != "stop" {
		t.Fatalf("finish_reason = %q, want %q", finish, "stop")
	}
	if !doneSeen {
		t.Fatal("expected final done chunk")
	}
	if finalUsage == nil || finalUsage.PromptTokens != 28 || finalUsage.CompletionTokens != 52 || finalUsage.TotalTokens != 80 {
		t.Fatalf("usage = %+v, want prompt=28 completion=52 total=80", finalUsage)
	}
}

func TestGoogleAdapterListModels_UsesGeminiModelsEndpoint(t *testing.T) {
	t.Helper()

	var (
		mu        sync.Mutex
		gotAPIKey string
		gotPath   string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotAPIKey = r.Header.Get("x-goog-api-key")
		gotPath = r.URL.Path
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"models":[
				{
					"name":"models/gemini-2.5-flash",
					"displayName":"Gemini 2.5 Flash",
					"description":"Fast Gemini model",
					"inputTokenLimit":1048576,
					"outputTokenLimit":8192,
					"supportedGenerationMethods":["generateContent","embedContent"]
				}
			]
		}`)
	}))
	defer server.Close()

	a, err := NewGoogleAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1beta",
	})
	if err != nil {
		t.Fatalf("NewGoogleAdapter() error = %v", err)
	}

	models, err := a.ListModels(context.Background(), "ignored")
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if gotAPIKey != "test-key" {
		t.Fatalf("x-goog-api-key = %q, want %q", gotAPIKey, "test-key")
	}
	if gotPath != "/v1beta/models" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1beta/models")
	}
	if len(models) != 1 {
		t.Fatalf("len(models) = %d, want 1", len(models))
	}
	if models[0].ID != "gemini-2.5-flash" {
		t.Fatalf("models[0].ID = %q, want %q", models[0].ID, "gemini-2.5-flash")
	}
	if models[0].Name != "Gemini 2.5 Flash" {
		t.Fatalf("models[0].Name = %q, want %q", models[0].Name, "Gemini 2.5 Flash")
	}
	if models[0].ContextLength != 1048576 {
		t.Fatalf("models[0].ContextLength = %d, want %d", models[0].ContextLength, 1048576)
	}
	if !containsStringValue(models[0].Capabilities, "chat") {
		t.Fatalf("models[0].Capabilities = %#v, want to contain %q", models[0].Capabilities, "chat")
	}
}

func containsStringValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
